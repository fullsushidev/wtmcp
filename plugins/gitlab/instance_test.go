package main

import (
	"os"
	"strings"
	"testing"
)

func TestDiscoverInstancesLegacy(t *testing.T) {
	t.Setenv("GITLAB_TOKEN", "test-token")
	t.Setenv("GITLAB_URL", "https://gitlab.example.com")

	// Clear any multi-instance vars
	for _, env := range os.Environ() {
		key, _, _ := strings.Cut(env, "=")
		if key != "GITLAB_TOKEN" && key != "GITLAB_URL" &&
			len(key) > 7 && key[:7] == "GITLAB_" && key[len(key)-6:] == "_TOKEN" {
			t.Setenv(key, "")
		}
	}

	if err := discoverInstances(); err != nil {
		t.Fatalf("discoverInstances: %v", err)
	}

	if len(instances) != 1 {
		t.Fatalf("expected 1 instance, got %d", len(instances))
	}
	inst, ok := instances["default"]
	if !ok {
		t.Fatal("expected 'default' instance")
	}
	if inst.URL != "https://gitlab.example.com" {
		t.Errorf("URL = %q, want https://gitlab.example.com", inst.URL)
	}
	if defaultInstance != "default" {
		t.Errorf("defaultInstance = %q, want 'default'", defaultInstance)
	}
}

func TestDiscoverInstancesMulti(t *testing.T) {
	// Clear legacy
	t.Setenv("GITLAB_TOKEN", "")

	t.Setenv("GITLAB_PUBLIC_TOKEN", "pub-token")
	t.Setenv("GITLAB_PUBLIC_URL", "https://gitlab.com")
	t.Setenv("GITLAB_INTERNAL_TOKEN", "int-token")
	t.Setenv("GITLAB_INTERNAL_URL", "https://gitlab.internal.com")

	if err := discoverInstances(); err != nil {
		t.Fatalf("discoverInstances: %v", err)
	}

	if len(instances) != 2 {
		t.Fatalf("expected 2 instances, got %d", len(instances))
	}
	if _, ok := instances["public"]; !ok {
		t.Error("missing 'public' instance")
	}
	if _, ok := instances["internal"]; !ok {
		t.Error("missing 'internal' instance")
	}
	// With multiple instances, no default
	if defaultInstance != "" {
		t.Errorf("defaultInstance should be empty with multiple instances, got %q", defaultInstance)
	}
}

func TestDiscoverInstancesDefaultURL(t *testing.T) {
	t.Setenv("GITLAB_TOKEN", "")
	t.Setenv("GITLAB_MYINST_TOKEN", "tok")
	// No GITLAB_MYINST_URL set

	if err := discoverInstances(); err != nil {
		t.Fatalf("discoverInstances: %v", err)
	}

	inst := instances["myinst"]
	if inst == nil {
		t.Fatal("missing 'myinst' instance")
	}
	if inst.URL != "https://gitlab.com" {
		t.Errorf("URL = %q, want https://gitlab.com (default)", inst.URL)
	}
}

func TestDiscoverInstancesNoToken(t *testing.T) {
	t.Setenv("GITLAB_TOKEN", "")
	// Clear all GITLAB_*_TOKEN vars
	for _, env := range os.Environ() {
		key, _, _ := strings.Cut(env, "=")
		if len(key) > 7 && key[:7] == "GITLAB_" && key[len(key)-6:] == "_TOKEN" {
			t.Setenv(key, "")
		}
	}

	err := discoverInstances()
	if err == nil {
		t.Fatal("expected error with no tokens configured")
	}
}

func TestResolveInstanceSingle(t *testing.T) {
	t.Setenv("GITLAB_TOKEN", "tok")
	t.Setenv("GITLAB_URL", "https://gitlab.com")

	if err := discoverInstances(); err != nil {
		t.Fatalf("discoverInstances: %v", err)
	}

	// Empty name should resolve to default
	client, err := resolveInstance("")
	if err != nil {
		t.Fatalf("resolveInstance: %v", err)
	}
	if client == nil {
		t.Fatal("client is nil")
	}
}

func TestResolveInstanceUnknown(t *testing.T) {
	t.Setenv("GITLAB_TOKEN", "tok")

	if err := discoverInstances(); err != nil {
		t.Fatalf("discoverInstances: %v", err)
	}

	_, err := resolveInstance("nonexistent")
	if err == nil {
		t.Fatal("expected error for unknown instance")
	}
}

func TestSslVerify(t *testing.T) {
	tests := []struct {
		name     string
		envs     map[string]string
		instance string
		want     bool
	}{
		{"default true", nil, "", true},
		{"global false", map[string]string{"GITLAB_SSL_VERIFY": "false"}, "", false},
		{"instance false", map[string]string{"GITLAB_MYINST_SSL_VERIFY": "false"}, "myinst", false},
		{"instance overrides global", map[string]string{
			"GITLAB_SSL_VERIFY":        "false",
			"GITLAB_MYINST_SSL_VERIFY": "true",
		}, "myinst", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Clear
			t.Setenv("GITLAB_SSL_VERIFY", "")
			t.Setenv("GITLAB_MYINST_SSL_VERIFY", "")
			for k, v := range tt.envs {
				t.Setenv(k, v)
			}
			got := sslVerify(tt.instance)
			if got != tt.want {
				t.Errorf("sslVerify(%q) = %v, want %v", tt.instance, got, tt.want)
			}
		})
	}
}

func TestParseTime(t *testing.T) {
	// RFC3339
	if pt := parseTime("2026-03-10T14:00:00Z"); pt == nil {
		t.Error("parseTime(RFC3339) returned nil")
	}
	// Date only
	if pt := parseTime("2026-03-10"); pt == nil {
		t.Error("parseTime(date) returned nil")
	}
	// Invalid
	if pt := parseTime("not-a-date"); pt != nil {
		t.Error("parseTime(invalid) should return nil")
	}
}
