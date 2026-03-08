package cache

import (
	"context"
	"encoding/json"
	"path/filepath"
	"sync"
	"time"
)

type entry struct {
	value   json.RawMessage
	expires time.Time // zero value = no expiry
}

func (e *entry) expired() bool {
	return !e.expires.IsZero() && time.Now().After(e.expires)
}

// MemoryStore is an in-memory cache backend.
type MemoryStore struct {
	mu      sync.RWMutex
	entries map[string]*entry
}

// NewMemoryStore creates an in-memory cache store.
func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		entries: make(map[string]*entry),
	}
}

// Get retrieves a value from the cache.
func (m *MemoryStore) Get(_ context.Context, namespace, key string) (json.RawMessage, bool, error) {
	sk := storageKey(namespace, key)

	m.mu.RLock()
	e, ok := m.entries[sk]
	m.mu.RUnlock()

	if !ok {
		return nil, false, nil
	}
	if e.expired() {
		m.mu.Lock()
		delete(m.entries, sk)
		m.mu.Unlock()
		return nil, false, nil
	}
	return e.value, true, nil
}

// Set stores a value in the cache with an optional TTL.
// A TTL of 0 means no expiry.
func (m *MemoryStore) Set(_ context.Context, namespace, key string, value json.RawMessage, ttl time.Duration) error {
	sk := storageKey(namespace, key)

	e := &entry{value: value}
	if ttl > 0 {
		e.expires = time.Now().Add(ttl)
	}

	m.mu.Lock()
	m.entries[sk] = e
	m.mu.Unlock()

	return nil
}

// Del removes a value from the cache. Returns true if the key existed.
func (m *MemoryStore) Del(_ context.Context, namespace, key string) (bool, error) {
	sk := storageKey(namespace, key)

	m.mu.Lock()
	_, existed := m.entries[sk]
	delete(m.entries, sk)
	m.mu.Unlock()

	return existed, nil
}

// List returns keys matching a glob pattern within a namespace.
// Results are capped at 1000 keys.
func (m *MemoryStore) List(_ context.Context, namespace, pattern string) ([]string, error) {
	prefix := namespace + "\x00"
	fullPattern := prefix + pattern

	m.mu.RLock()
	defer m.mu.RUnlock()

	var keys []string
	for sk, e := range m.entries {
		if e.expired() {
			continue
		}
		if len(sk) <= len(prefix) {
			continue
		}
		if sk[:len(prefix)] != prefix {
			continue
		}
		userKey := sk[len(prefix):]
		matched, err := filepath.Match(fullPattern, sk)
		if err != nil {
			return nil, err
		}
		if matched {
			keys = append(keys, userKey)
			if len(keys) >= 1000 {
				break
			}
		}
	}

	return keys, nil
}

// Flush removes all entries in a namespace. Returns the count of removed entries.
func (m *MemoryStore) Flush(_ context.Context, namespace string) (int, error) {
	prefix := namespace + "\x00"

	m.mu.Lock()
	defer m.mu.Unlock()

	count := 0
	for sk := range m.entries {
		if len(sk) > len(prefix) && sk[:len(prefix)] == prefix {
			delete(m.entries, sk)
			count++
		}
	}
	return count, nil
}
