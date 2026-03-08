// Package proxy provides an HTTP proxy that makes authenticated
// requests on behalf of plugins. Auth headers, retries, rate limiting,
// and response body limits are handled centrally.
package proxy

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"

	"gitlab.cee.redhat.com/bragctl/what-the-mcp/internal/auth"
	"gitlab.cee.redhat.com/bragctl/what-the-mcp/internal/plugin"
)

// PluginAuth holds the resolved auth and HTTP config for a plugin.
type PluginAuth struct {
	Provider       auth.Provider
	BaseURL        string
	AllowedDomains []string
}

// Proxy executes HTTP requests on behalf of plugins, injecting
// authentication headers and enforcing security policies.
type Proxy struct {
	plugins     map[string]*PluginAuth
	client      *http.Client
	maxBodySize int64
}

// New creates a Proxy with the given HTTP client and max response body size.
func New(client *http.Client, maxBodySize int64) *Proxy {
	if client == nil {
		client = &http.Client{}
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

// Execute handles an http_request message from a plugin.
func (p *Proxy) Execute(ctx context.Context, pluginName string, req plugin.Message) plugin.Message {
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

	if pa.Provider != nil {
		if err := p.injectAuth(ctx, pa.Provider, httpReq); err != nil {
			return errResponse(req.ID, "auth_failed", err.Error())
		}
	}

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return plugin.Message{
			ID:     req.ID,
			Type:   plugin.TypeHTTPResponse,
			Status: 0,
			Error:  &plugin.Error{Code: "transport_error", Message: err.Error()},
		}
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			log.Printf("proxy: failed to close response body: %v", err)
		}
	}()

	body, err := p.readBody(resp)
	if err != nil {
		return errResponse(req.ID, "response_too_large", err.Error())
	}

	return plugin.Message{
		ID:     req.ID,
		Type:   plugin.TypeHTTPResponse,
		Status: resp.StatusCode,
		Body:   body,
	}
}

func (p *Proxy) resolveURL(pluginName string, pa *PluginAuth, req plugin.Message) (string, error) {
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

func (p *Proxy) buildRequest(ctx context.Context, fullURL string, req plugin.Message) (*http.Request, error) {
	var bodyReader io.Reader
	if req.Body != nil {
		bodyReader = strings.NewReader(string(req.Body))
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

	// Add plugin-specified headers
	for k, v := range req.Headers {
		httpReq.Header.Set(k, v)
	}

	return httpReq, nil
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

func (p *Proxy) readBody(resp *http.Response) (json.RawMessage, error) {
	limited := io.LimitReader(resp.Body, p.maxBodySize+1)
	body, err := io.ReadAll(limited)
	if err != nil {
		return nil, err
	}
	if int64(len(body)) > p.maxBodySize {
		return nil, fmt.Errorf("response body exceeds %d bytes", p.maxBodySize)
	}

	ct := resp.Header.Get("Content-Type")
	if strings.Contains(ct, "application/json") {
		return json.RawMessage(body), nil
	}
	// Non-JSON: return as quoted string
	return json.Marshal(string(body))
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

func errResponse(id, code, message string) plugin.Message {
	return plugin.Message{
		ID:     id,
		Type:   plugin.TypeHTTPResponse,
		Status: 0,
		Error:  &plugin.Error{Code: code, Message: message},
	}
}
