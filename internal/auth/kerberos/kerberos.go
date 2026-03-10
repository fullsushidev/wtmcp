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
	"io"
	"log"
	"net/http"
	"strings"
)

// SPNEGORoundTripper wraps an http.RoundTripper to add SPNEGO authentication
// headers to all outgoing requests, including redirect hops.
//
// If spn is empty, the SPN is derived dynamically as "HTTP@<hostname>" from
// each request's URL. If the GSSAPI call fails (e.g., no SPN registered in
// the KDC for that hostname), the request proceeds without a Negotiate
// header — this allows redirect-based flows (like OIDC) where the initial
// host has no SPN but the SSO server does.
type SPNEGORoundTripper struct {
	spn  string
	next http.RoundTripper
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

// RoundTrip implements http.RoundTripper with reactive 401 challenge
// handling, matching the behavior of Python's requests-kerberos.
//
// Flow:
//  1. Send the request WITHOUT a Negotiate header
//  2. If the server returns 401 + WWW-Authenticate: Negotiate,
//     generate a SPNEGO token and retry (challenge-response)
//  3. This matches how SSO servers like auth.redhat.com expect
//     SPNEGO authentication to work
func (s *SPNEGORoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	// First attempt: send without Negotiate
	resp, err := s.next.RoundTrip(req.Clone(req.Context()))
	if err != nil {
		return nil, err
	}

	// Check for 401 + WWW-Authenticate: Negotiate challenge
	if resp.StatusCode != http.StatusUnauthorized {
		return resp, nil
	}
	authHeader := resp.Header.Get("WWW-Authenticate")
	if !strings.Contains(authHeader, "Negotiate") {
		return resp, nil
	}

	// Server challenged — drain the 401 body and retry with SPNEGO
	_, _ = io.Copy(io.Discard, resp.Body)
	_ = resp.Body.Close()

	spn := s.spn
	if spn == "" {
		spn = "HTTP@" + req.URL.Hostname()
	}

	retryReq := req.Clone(req.Context())
	if err := SetSPNEGOHeader(retryReq, spn); err != nil {
		log.Printf("kerberos: SPNEGO failed for %s after 401 challenge: %v",
			req.URL.Hostname(), err)
		// Return a fresh unauthenticated response rather than the drained 401
		return s.next.RoundTrip(req.Clone(req.Context()))
	}

	log.Printf("kerberos: responding to 401 challenge from %s with Negotiate", req.URL.Hostname())
	return s.next.RoundTrip(retryReq)
}
