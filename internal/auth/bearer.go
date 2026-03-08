package auth

import (
	"context"
	"fmt"
	"net/http"
)

// BearerProvider injects a static bearer token into requests.
type BearerProvider struct {
	token  string
	header string
	prefix string
}

// NewBearerProvider creates a bearer token auth provider.
// If header is empty, defaults to "Authorization".
// If prefix is empty, defaults to "Bearer".
func NewBearerProvider(token, header, prefix string) *BearerProvider {
	if header == "" {
		header = "Authorization"
	}
	if prefix == "" {
		prefix = "Bearer"
	}
	return &BearerProvider{token: token, header: header, prefix: prefix}
}

// Name returns "bearer".
func (b *BearerProvider) Name() string { return "bearer" }

// Available reports whether a token is configured.
func (b *BearerProvider) Available() bool { return b.token != "" }

// Authenticate returns the authorization header.
func (b *BearerProvider) Authenticate(_ context.Context, _ *http.Request) (http.Header, error) {
	if b.token == "" {
		return nil, fmt.Errorf("bearer token not configured")
	}
	h := make(http.Header)
	h.Set(b.header, b.prefix+" "+b.token)
	return h, nil
}
