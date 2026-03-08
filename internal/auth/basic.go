package auth

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/http"
)

// BasicProvider injects HTTP Basic authentication.
type BasicProvider struct {
	username string
	password string
}

// NewBasicProvider creates a basic auth provider.
func NewBasicProvider(username, password string) *BasicProvider {
	return &BasicProvider{username: username, password: password}
}

// Name returns "basic".
func (b *BasicProvider) Name() string { return "basic" }

// Available reports whether credentials are configured.
func (b *BasicProvider) Available() bool { return b.username != "" && b.password != "" }

// Authenticate returns the Basic authorization header.
func (b *BasicProvider) Authenticate(_ context.Context, _ *http.Request) (http.Header, error) {
	if b.username == "" || b.password == "" {
		return nil, fmt.Errorf("basic auth credentials not configured")
	}
	h := make(http.Header)
	encoded := base64.StdEncoding.EncodeToString([]byte(b.username + ":" + b.password))
	h.Set("Authorization", "Basic "+encoded)
	return h, nil
}
