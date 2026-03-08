// Package cache provides a namespaced key-value cache for plugins.
//
// Each plugin gets its own namespace. Keys are isolated via a null-byte
// separator in the storage key to prevent collisions.
package cache

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"time"
)

// Store is a namespaced key-value cache.
//
// Implementations use a null-byte separator for namespace isolation:
//
//	storageKey = namespace + "\x00" + key
type Store interface {
	Get(ctx context.Context, namespace, key string) (json.RawMessage, bool, error)
	Set(ctx context.Context, namespace, key string, value json.RawMessage, ttl time.Duration) error
	Del(ctx context.Context, namespace, key string) (bool, error)
	List(ctx context.Context, namespace, pattern string) ([]string, error)
	Flush(ctx context.Context, namespace string) (int, error)
}

// validKeyPattern defines allowed cache key characters.
// Keys must be alphanumeric with dots, underscores, colons, and hyphens.
var validKeyPattern = regexp.MustCompile(`^[a-zA-Z0-9_.:-]{1,512}$`)

// ValidateKey checks that a cache key is safe for all backends.
func ValidateKey(key string) error {
	if !validKeyPattern.MatchString(key) {
		return fmt.Errorf("invalid cache key %q: must match [a-zA-Z0-9_.:-]{1,512}", key)
	}
	return nil
}

// storageKey builds the internal key with namespace isolation.
func storageKey(namespace, key string) string {
	return namespace + "\x00" + key
}
