package cache

import (
	"context"
	"encoding/json"
	"testing"
	"time"
)

func TestMemoryStoreGetSet(t *testing.T) {
	ctx := context.Background()
	s := NewMemoryStore()

	val := json.RawMessage(`{"key":"value"}`)
	if err := s.Set(ctx, "plugin1", "mykey", val, 0); err != nil {
		t.Fatalf("Set failed: %v", err)
	}

	got, hit, err := s.Get(ctx, "plugin1", "mykey")
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if !hit {
		t.Fatal("expected cache hit")
	}
	if string(got) != string(val) {
		t.Errorf("got %s, want %s", got, val)
	}
}

func TestMemoryStoreNamespaceIsolation(t *testing.T) {
	ctx := context.Background()
	s := NewMemoryStore()

	if err := s.Set(ctx, "plugin1", "key", json.RawMessage(`"one"`), 0); err != nil {
		t.Fatal(err)
	}
	if err := s.Set(ctx, "plugin2", "key", json.RawMessage(`"two"`), 0); err != nil {
		t.Fatal(err)
	}

	v1, _, _ := s.Get(ctx, "plugin1", "key")
	v2, _, _ := s.Get(ctx, "plugin2", "key")

	if string(v1) != `"one"` {
		t.Errorf("plugin1 key = %s, want %q", v1, "one")
	}
	if string(v2) != `"two"` {
		t.Errorf("plugin2 key = %s, want %q", v2, "two")
	}
}

func TestMemoryStoreMiss(t *testing.T) {
	ctx := context.Background()
	s := NewMemoryStore()

	_, hit, err := s.Get(ctx, "ns", "missing")
	if err != nil {
		t.Fatal(err)
	}
	if hit {
		t.Error("expected cache miss")
	}
}

func TestMemoryStoreTTL(t *testing.T) {
	ctx := context.Background()
	s := NewMemoryStore()

	if err := s.Set(ctx, "ns", "expiring", json.RawMessage(`true`), 50*time.Millisecond); err != nil {
		t.Fatal(err)
	}

	// Should be a hit immediately
	_, hit, _ := s.Get(ctx, "ns", "expiring")
	if !hit {
		t.Error("expected hit before TTL")
	}

	// Wait for expiry
	time.Sleep(100 * time.Millisecond)

	_, hit, _ = s.Get(ctx, "ns", "expiring")
	if hit {
		t.Error("expected miss after TTL")
	}
}

func TestMemoryStoreDel(t *testing.T) {
	ctx := context.Background()
	s := NewMemoryStore()

	if err := s.Set(ctx, "ns", "del-me", json.RawMessage(`1`), 0); err != nil {
		t.Fatal(err)
	}

	existed, err := s.Del(ctx, "ns", "del-me")
	if err != nil {
		t.Fatal(err)
	}
	if !existed {
		t.Error("expected existed=true")
	}

	existed, _ = s.Del(ctx, "ns", "del-me")
	if existed {
		t.Error("expected existed=false for second delete")
	}

	_, hit, _ := s.Get(ctx, "ns", "del-me")
	if hit {
		t.Error("expected miss after delete")
	}
}

func TestMemoryStoreFlush(t *testing.T) {
	ctx := context.Background()
	s := NewMemoryStore()

	for i := range 5 {
		key := "key" + string(rune('0'+i))
		if err := s.Set(ctx, "ns", key, json.RawMessage(`1`), 0); err != nil {
			t.Fatal(err)
		}
	}
	// Different namespace — should not be flushed
	if err := s.Set(ctx, "other", "key", json.RawMessage(`1`), 0); err != nil {
		t.Fatal(err)
	}

	count, err := s.Flush(ctx, "ns")
	if err != nil {
		t.Fatal(err)
	}
	if count != 5 {
		t.Errorf("flushed %d, want 5", count)
	}

	// Other namespace intact
	_, hit, _ := s.Get(ctx, "other", "key")
	if !hit {
		t.Error("other namespace should not be affected by flush")
	}
}

func TestMemoryStoreList(t *testing.T) {
	ctx := context.Background()
	s := NewMemoryStore()

	for _, key := range []string{"fields:PROJECT", "fields:TEAM", "issue:FOO-1"} {
		if err := s.Set(ctx, "jira", key, json.RawMessage(`{}`), 0); err != nil {
			t.Fatal(err)
		}
	}

	keys, err := s.List(ctx, "jira", "fields:*")
	if err != nil {
		t.Fatal(err)
	}
	if len(keys) != 2 {
		t.Errorf("got %d keys, want 2: %v", len(keys), keys)
	}
}

func TestValidateKey(t *testing.T) {
	valid := []string{"key", "fields:PROJECT", "issue.FOO-123", "a", "a-b_c.d:e"}
	for _, k := range valid {
		if err := ValidateKey(k); err != nil {
			t.Errorf("ValidateKey(%q) should pass: %v", k, err)
		}
	}

	invalid := []string{"", "../etc/passwd", "key with spaces", "key\x00null", "/absolute", string(make([]byte, 513))}
	for _, k := range invalid {
		if err := ValidateKey(k); err == nil {
			t.Errorf("ValidateKey(%q) should fail", k)
		}
	}
}
