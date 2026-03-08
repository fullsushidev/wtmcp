// Package auth provides authentication providers for the HTTP proxy.
//
// Built-in providers: bearer, basic, kerberos/spnego, oauth2.
// Plugins can register additional providers via the auth_request/response
// protocol.
package auth

import (
	"context"
	"fmt"
	"net/http"
	"sync"
)

// Provider resolves auth credentials for HTTP requests.
type Provider interface {
	// Name returns the auth type identifier (e.g., "bearer", "kerberos/spnego").
	Name() string

	// Authenticate returns headers to inject into an HTTP request.
	// Called by the HTTP proxy before each request.
	Authenticate(ctx context.Context, target *http.Request) (http.Header, error)

	// Available reports whether this provider has valid credentials
	// configured (env vars set, ticket present, etc.).
	// Used by the variant auto-detection logic.
	Available() bool
}

// Registry holds all registered auth providers, both built-in and
// plugin-provided.
type Registry struct {
	mu        sync.RWMutex
	providers map[string]Provider
}

// NewRegistry creates an empty auth provider registry.
func NewRegistry() *Registry {
	return &Registry{
		providers: make(map[string]Provider),
	}
}

// Register adds a provider to the registry.
// If a provider with the same name already exists, it is replaced.
func (r *Registry) Register(p Provider) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.providers[p.Name()] = p
}

// Get returns the provider for the given auth type.
func (r *Registry) Get(name string) (Provider, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	p, ok := r.providers[name]
	if !ok {
		return nil, fmt.Errorf("unknown auth provider: %s", name)
	}
	return p, nil
}

// Has reports whether a provider with the given name is registered.
func (r *Registry) Has(name string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	_, ok := r.providers[name]
	return ok
}
