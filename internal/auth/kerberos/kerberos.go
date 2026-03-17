// Package kerberos provides GSSAPI/SPNEGO authentication for HTTP requests.
//
// Platform support:
//   - Linux: dlopen libgssapi_krb5.so.2 (via sassoftware/gssapi, no link-time CGO)
//   - macOS: CGO linking GSS.framework (uses pure Kerberos V5 instead of SPNEGO
//     because GSS.framework/Heimdal does not properly support SPNEGO)
//
// Credentials are acquired fresh on each call from the system's default
// credential cache, so kinit renewals are picked up automatically.
package kerberos

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"
)

const skipHostTTL = 5 * time.Minute

// SPNEGORoundTripper wraps an http.RoundTripper to add SPNEGO
// authentication. Uses proactive-first strategy: sends the Negotiate
// header on the first request. Falls back to reactive 401
// challenge-response if proactive token generation fails.
//
// Proactive auth avoids mod_auth_gssapi CSRF protection issues on
// POST requests (e.g., FreeIPA). Reactive fallback handles SSO
// redirect flows where the initial host has no SPN.
//
// Mutual authentication is OPTIONAL — the server's proof token in
// 200 responses is not verified. TLS provides server authentication.
//
// If spn is empty, the SPN is derived dynamically as "HTTP@<hostname>"
// from each request's URL. If the GSSAPI call fails (e.g., no SPN
// registered in the KDC for that hostname), the request proceeds
// without a Negotiate header — this allows redirect-based flows
// (like OIDC) where the initial host has no SPN but the SSO server
// does.
type SPNEGORoundTripper struct {
	spn       string
	next      http.RoundTripper
	skipHosts sync.Map // hostname -> time.Time (expiry after skipHostTTL)
}

// NewSPNEGORoundTripper creates a new round tripper that adds SPNEGO headers.
// If spn is empty, the SPN is derived from each request's hostname.
func NewSPNEGORoundTripper(spn string, next http.RoundTripper) http.RoundTripper {
	if next == nil {
		next = http.DefaultTransport
	}
	return &SPNEGORoundTripper{
		spn:  spn,
		next: next,
	}
}

// RoundTrip implements http.RoundTripper with proactive-first SPNEGO.
//
// Flow:
//  1. Try to generate a SPNEGO token and send it on the FIRST request
//  2. If token generation fails (no SPN, no ticket, wrong realm),
//     send without auth — hostname is cached to skip future attempts
//  3. If the server returns 401 + WWW-Authenticate: Negotiate,
//     generate a fresh token and retry (reactive fallback)
func (s *SPNEGORoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	spn := s.spn
	if spn == "" {
		spn = "HTTP@" + req.URL.Hostname()
	}

	// Proactive: try to attach Negotiate token on first request.
	// Skip if this host previously failed (cached with TTL).
	authReq := req.Clone(req.Context())
	hostname := req.URL.Hostname()

	skip := false
	if t, ok := s.skipHosts.Load(hostname); ok {
		if time.Since(t.(time.Time)) < skipHostTTL {
			skip = true
		} else {
			s.skipHosts.Delete(hostname)
		}
	}
	if !skip {
		if err := SetSPNEGOHeader(authReq, spn); err != nil {
			// No ticket / wrong realm / GSSAPI error — send without
			// auth. SetSPNEGOHeader does not modify headers on error,
			// so authReq is still clean (no redundant clone needed).
			log.Printf("kerberos: proactive SPNEGO skipped for %s: %v",
				hostname, err)
			s.skipHosts.Store(hostname, time.Now())
		}
	}

	resp, err := s.next.RoundTrip(authReq)
	if err != nil {
		return nil, err
	}

	// If not 401, we're done (auth succeeded or not needed)
	if resp.StatusCode != http.StatusUnauthorized {
		return resp, nil
	}

	// 401 received — check for Negotiate challenge
	authHeader := resp.Header.Get("WWW-Authenticate")
	if !strings.Contains(authHeader, "Negotiate") {
		return resp, nil
	}

	// Reactive fallback: server wants challenge-response.
	// Creates a fresh GSSAPI context (standard for HTTP SPNEGO).
	_, _ = io.Copy(io.Discard, resp.Body)
	_ = resp.Body.Close()

	retryReq := req.Clone(req.Context())
	if err := resetBody(retryReq, req); err != nil {
		return nil, err
	}

	if err := SetSPNEGOHeader(retryReq, spn); err != nil {
		log.Printf("kerberos: SPNEGO failed for %s after 401: %v",
			hostname, err)
		fallbackReq := req.Clone(req.Context())
		_ = resetBody(fallbackReq, req)
		return s.next.RoundTrip(fallbackReq)
	}

	log.Printf("kerberos: reactive auth for %s after 401", hostname)
	return s.next.RoundTrip(retryReq)
}

// resetBody obtains a fresh body reader for a cloned request.
// After the first RoundTrip consumes the body's io.Reader, Clone()
// inherits the exhausted reader. GetBody() provides a fresh copy,
// matching what http.Client does for redirect replays.
func resetBody(cloned, orig *http.Request) error {
	if orig.GetBody == nil {
		return nil // no body or body doesn't support replay
	}
	body, err := orig.GetBody()
	if err != nil {
		return fmt.Errorf("kerberos: reset body for retry: %w", err)
	}
	cloned.Body = body
	return nil
}
