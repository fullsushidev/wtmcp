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
	"net/http"
)

// SPNEGORoundTripper wraps an http.RoundTripper to add SPNEGO authentication
// headers to all outgoing requests.
type SPNEGORoundTripper struct {
	spn  string
	next http.RoundTripper
}

// NewSPNEGORoundTripper creates a new round tripper that adds SPNEGO headers
// for the given service principal name to all requests.
func NewSPNEGORoundTripper(spn string, next http.RoundTripper) http.RoundTripper {
	if next == nil {
		next = http.DefaultTransport
	}
	return &SPNEGORoundTripper{
		spn:  spn,
		next: next,
	}
}

// RoundTrip implements http.RoundTripper by adding SPNEGO authentication
// to the request before passing it to the next transport.
func (s *SPNEGORoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	clonedReq := req.Clone(req.Context())

	if err := SetSPNEGOHeader(clonedReq, s.spn); err != nil {
		return nil, fmt.Errorf("failed to set SPNEGO header: %w", err)
	}

	return s.next.RoundTrip(clonedReq)
}
