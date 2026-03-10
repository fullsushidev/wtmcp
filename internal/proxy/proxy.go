// Package proxy provides an HTTP proxy that makes authenticated
// requests on behalf of plugins. Auth headers, retries, rate limiting,
// and response body limits are handled centrally.
package proxy

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"net/http/cookiejar"
	"net/textproto"
	"net/url"
	"strings"

	"github.com/LeGambiArt/wtmcp/internal/auth"
	"github.com/LeGambiArt/wtmcp/internal/auth/kerberos"
	"github.com/LeGambiArt/wtmcp/internal/protocol"
)

// PluginAuth holds the resolved auth and HTTP config for a plugin.
type PluginAuth struct {
	Provider       auth.Provider
	BaseURL        string
	AllowedDomains []string

	// Client is an optional per-plugin HTTP client. When set (e.g., for
	// Kerberos plugins with cookie jar + SPNEGORoundTripper), it is used
	// instead of the shared proxy client, and header-based auth injection
	// is skipped.
	Client *http.Client
}

// Proxy executes HTTP requests on behalf of plugins, injecting
// authentication headers and enforcing security policies.
type Proxy struct {
	plugins     map[string]*PluginAuth
	client      *http.Client
	maxBodySize int64
}

// New creates a Proxy with the given HTTP client and max response body size.
// When client is nil, a default client with SSRF-safe dialer is used.
func New(client *http.Client, maxBodySize int64) *Proxy {
	if client == nil {
		client = &http.Client{Transport: safeTransport(false)}
	}
	return &Proxy{
		plugins:     make(map[string]*PluginAuth),
		client:      client,
		maxBodySize: maxBodySize,
	}
}

// RegisterPlugin associates auth and HTTP config with a plugin name.
func (p *Proxy) RegisterPlugin(name string, pa *PluginAuth) {
	p.plugins[name] = pa
}

// NewKerberosClient creates an HTTP client with a cookie jar and
// SPNEGORoundTripper for Kerberos-authenticated plugins. If spn is
// empty, the SPN is derived dynamically from each request's hostname.
func NewKerberosClient(spn string) *http.Client {
	jar, _ := cookiejar.New(nil) // cookiejar.New only errors with non-nil options
	return &http.Client{
		Jar:       jar,
		Transport: kerberos.NewSPNEGORoundTripper(spn, safeTransport(false)),
	}
}

// Execute handles an http_request message from a plugin.
func (p *Proxy) Execute(ctx context.Context, pluginName string, req protocol.Message) protocol.Message {
	pa, ok := p.plugins[pluginName]
	if !ok {
		return errResponse(req.ID, "no_config", "no HTTP config registered for plugin "+pluginName)
	}

	fullURL, err := p.resolveURL(pluginName, pa, req)
	if err != nil {
		return errResponse(req.ID, "invalid_url", err.Error())
	}

	httpReq, err := p.buildRequest(ctx, fullURL, req)
	if err != nil {
		return errResponse(req.ID, "build_request", err.Error())
	}

	// Use per-plugin client (Kerberos with cookie jar + round tripper)
	// or fall back to the shared client with header-based auth injection.
	client := p.client
	if pa.Client != nil {
		client = pa.Client
	} else if pa.Provider != nil {
		if err := p.injectAuth(ctx, pa.Provider, httpReq); err != nil {
			return errResponse(req.ID, "auth_failed", err.Error())
		}
	}

	resp, err := client.Do(httpReq)
	if err != nil {
		return protocol.Message{
			ID:     req.ID,
			Type:   protocol.TypeHTTPResponse,
			Status: 0,
			Error:  &protocol.Error{Code: "transport_error", Message: err.Error()},
		}
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			log.Printf("proxy: failed to close response body: %v", err)
		}
	}()

	body, encoding, err := p.readBody(resp)
	if err != nil {
		return errResponse(req.ID, "response_too_large", err.Error())
	}

	return protocol.Message{
		ID:           req.ID,
		Type:         protocol.TypeHTTPResponse,
		Status:       resp.StatusCode,
		Headers:      responseHeaders(resp),
		Body:         body,
		BodyEncoding: encoding,
	}
}

func (p *Proxy) resolveURL(pluginName string, pa *PluginAuth, req protocol.Message) (string, error) {
	if req.URL != "" {
		if !p.isDomainAllowed(pluginName, pa, req.URL) {
			return "", fmt.Errorf("domain not allowed: %s", req.URL)
		}
		return req.URL, nil
	}

	if pa.BaseURL == "" {
		return "", fmt.Errorf("no base_url configured and no full url provided")
	}

	joined, err := url.JoinPath(pa.BaseURL, req.Path)
	if err != nil {
		return "", fmt.Errorf("join path: %w", err)
	}
	return joined, nil
}

func (p *Proxy) buildRequest(ctx context.Context, fullURL string, req protocol.Message) (*http.Request, error) {
	var bodyReader io.Reader
	var contentType string

	if len(req.Multipart) > 0 {
		var err error
		bodyReader, contentType, err = buildMultipart(req.Multipart)
		if err != nil {
			return nil, fmt.Errorf("build multipart: %w", err)
		}
	} else if req.Body != nil {
		if len(req.Body) > 0 && req.Body[0] == '"' {
			var s string
			if err := json.Unmarshal(req.Body, &s); err == nil {
				bodyReader = strings.NewReader(s)
			} else {
				bodyReader = strings.NewReader(string(req.Body))
			}
		} else {
			bodyReader = strings.NewReader(string(req.Body))
		}
	}

	httpReq, err := http.NewRequestWithContext(ctx, req.Method, fullURL, bodyReader)
	if err != nil {
		return nil, err
	}

	// Add query params (supports string and []string values)
	if len(req.Query) > 0 {
		q := httpReq.URL.Query()
		for k, v := range req.Query {
			switch val := v.(type) {
			case string:
				q.Set(k, val)
			case []any:
				for _, item := range val {
					q.Add(k, fmt.Sprint(item))
				}
			default:
				q.Set(k, fmt.Sprint(v))
			}
		}
		httpReq.URL.RawQuery = q.Encode()
	}

	// Add plugin-specified headers, then strip security-sensitive ones
	// that plugins should not control.
	for k, v := range req.Headers {
		httpReq.Header.Set(k, v)
	}
	stripDangerousHeaders(httpReq)

	// Proxy sets Content-Type for multipart (includes boundary).
	// Must come after plugin headers to override any plugin-set Content-Type.
	if contentType != "" {
		httpReq.Header.Set("Content-Type", contentType)
	} else if req.Body != nil && httpReq.Header.Get("Content-Type") == "" {
		// Default to application/json for requests with a body
		httpReq.Header.Set("Content-Type", "application/json")
	}

	return httpReq, nil
}

// buildMultipart assembles a multipart/form-data body from protocol parts.
func buildMultipart(parts []protocol.MultipartPart) (io.Reader, string, error) {
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)

	for _, part := range parts {
		var content []byte
		if part.BodyEncoding == "base64" {
			var err error
			content, err = base64.StdEncoding.DecodeString(part.Body)
			if err != nil {
				return nil, "", fmt.Errorf("base64 decode field %q: %w", part.Field, err)
			}
		} else {
			content = []byte(part.Body)
		}

		if part.Filename != "" {
			ct := part.ContentType
			if ct == "" {
				ct = "application/octet-stream"
			}
			h := make(textproto.MIMEHeader)
			h.Set("Content-Disposition",
				fmt.Sprintf(`form-data; name=%q; filename=%q`, part.Field, part.Filename))
			h.Set("Content-Type", ct)

			pw, err := w.CreatePart(h)
			if err != nil {
				return nil, "", fmt.Errorf("create file part %q: %w", part.Field, err)
			}
			if _, err := pw.Write(content); err != nil {
				return nil, "", fmt.Errorf("write file part %q: %w", part.Field, err)
			}
		} else {
			if err := w.WriteField(part.Field, string(content)); err != nil {
				return nil, "", fmt.Errorf("write field %q: %w", part.Field, err)
			}
		}
	}

	if err := w.Close(); err != nil {
		return nil, "", fmt.Errorf("close multipart writer: %w", err)
	}

	return &buf, w.FormDataContentType(), nil
}

func (p *Proxy) injectAuth(ctx context.Context, provider auth.Provider, httpReq *http.Request) error {
	authHeaders, err := provider.Authenticate(ctx, httpReq)
	if err != nil {
		return err
	}
	// Direct assign preserves multi-value headers and overwrites
	// any plugin-set auth headers.
	for k, vals := range authHeaders {
		httpReq.Header[k] = vals
	}
	return nil
}

func (p *Proxy) readBody(resp *http.Response) (json.RawMessage, string, error) {
	limited := io.LimitReader(resp.Body, p.maxBodySize+1)
	body, err := io.ReadAll(limited)
	if err != nil {
		return nil, "", err
	}
	if int64(len(body)) > p.maxBodySize {
		return nil, "", fmt.Errorf("response body exceeds %d bytes", p.maxBodySize)
	}

	ct := resp.Header.Get("Content-Type")

	// JSON: return raw bytes as-is
	if strings.Contains(ct, "application/json") {
		return json.RawMessage(body), "", nil
	}

	// Text: return as quoted JSON string
	if strings.HasPrefix(ct, "text/") {
		b, err := json.Marshal(string(body))
		return json.RawMessage(b), "", err
	}

	// Binary (or unknown): base64-encode
	encoded := base64.StdEncoding.EncodeToString(body)
	b, err := json.Marshal(encoded)
	return json.RawMessage(b), "base64", err
}

func (p *Proxy) isDomainAllowed(pluginName string, pa *PluginAuth, rawURL string) bool {
	reqURL, err := url.Parse(rawURL)
	if err != nil {
		return false
	}

	// Reject non-HTTPS when auth is configured
	if pa.Provider != nil && reqURL.Scheme != "https" {
		log.Printf("[%s] rejecting non-HTTPS URL with auth: %s", pluginName, rawURL)
		return false
	}

	// Reject URLs with userinfo
	if reqURL.User != nil {
		return false
	}

	reqHost := reqURL.Hostname()
	baseURL, _ := url.Parse(pa.BaseURL)
	baseHost := baseURL.Hostname()

	if strings.EqualFold(reqHost, baseHost) {
		return true
	}
	for _, domain := range pa.AllowedDomains {
		if strings.EqualFold(reqHost, domain) {
			return true
		}
	}
	return false
}

func responseHeaders(resp *http.Response) map[string]string {
	if len(resp.Header) == 0 {
		return nil
	}
	h := make(map[string]string, len(resp.Header))
	for k := range resp.Header {
		h[k] = resp.Header.Get(k)
	}
	return h
}

func errResponse(id, code, message string) protocol.Message {
	return protocol.Message{
		ID:     id,
		Type:   protocol.TypeHTTPResponse,
		Status: 0,
		Error:  &protocol.Error{Code: code, Message: message},
	}
}

// dangerousHeaders are headers that plugins must not control.
// Authorization is stripped here and re-added by injectAuth() when
// auth is configured. For Kerberos plugins, the SPNEGO round-tripper
// re-adds it. When no auth is configured, stripping prevents plugins
// from injecting arbitrary credentials.
//
// Note: Kerberos plugins have a per-plugin cookiejar that re-adds
// cookies from prior responses after stripping — this is intentional
// for SPNEGO auth flows.
var dangerousHeaders = []string{
	"Authorization",
	"Proxy-Authorization",
	"Host",
	"Cookie",
	"Set-Cookie",
	"Connection",
	"Upgrade",
	"Transfer-Encoding",
	"Te",
	"Trailer",
	"Forwarded",
	"X-Forwarded-For",
	"X-Forwarded-Host",
	"X-Forwarded-Proto",
	"X-Real-Ip",
	"X-Original-Url",
	"X-Rewrite-Url",
}

// stripDangerousHeaders removes security-sensitive headers that
// plugins should not set on proxied requests.
func stripDangerousHeaders(req *http.Request) {
	for _, h := range dangerousHeaders {
		req.Header.Del(h)
	}
}
