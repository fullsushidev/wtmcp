package config

import (
	"testing"
)

func TestResolveEnvVars(t *testing.T) {
	// Set up test env vars
	t.Setenv("TEST_VAR", "hello")
	t.Setenv("EMPTY_VAR", "")

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "simple var",
			input:    "${TEST_VAR}",
			expected: "hello",
		},
		{
			name:     "unset var",
			input:    "${UNSET_VAR}",
			expected: "",
		},
		{
			name:     "var with default",
			input:    "${UNSET_VAR:-fallback}",
			expected: "fallback",
		},
		{
			name:     "set var ignores default",
			input:    "${TEST_VAR:-fallback}",
			expected: "hello",
		},
		{
			name:     "empty var uses default",
			input:    "${EMPTY_VAR:-fallback}",
			expected: "fallback",
		},
		{
			name:     "literal dollar",
			input:    "$$price",
			expected: "$price",
		},
		{
			name:     "mixed text and vars",
			input:    "https://${TEST_VAR}.example.com/api",
			expected: "https://hello.example.com/api",
		},
		{
			name:     "no vars",
			input:    "plain string",
			expected: "plain string",
		},
		{
			name:     "empty default",
			input:    "${UNSET_VAR:-}",
			expected: "",
		},
		{
			name:     "multiple vars",
			input:    "${TEST_VAR}:${TEST_VAR}",
			expected: "hello:hello",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ResolveEnvVars(tt.input)
			if result != tt.expected {
				t.Errorf("ResolveEnvVars(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestResolveEnvMap(t *testing.T) {
	t.Setenv("TOKEN", "secret123")

	m := map[string]string{
		"url":   "https://api.example.com",
		"token": "${TOKEN}",
		"file":  "${MISSING:-default.json}",
	}

	resolved := ResolveEnvMap(m)

	if resolved["url"] != "https://api.example.com" {
		t.Errorf("url = %q, want %q", resolved["url"], "https://api.example.com")
	}
	if resolved["token"] != "secret123" {
		t.Errorf("token = %q, want %q", resolved["token"], "secret123")
	}
	if resolved["file"] != "default.json" {
		t.Errorf("file = %q, want %q", resolved["file"], "default.json")
	}
}

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.Plugins.MaxMessageSize != 10*1024*1024 {
		t.Errorf("MaxMessageSize = %d, want %d", cfg.Plugins.MaxMessageSize, 10*1024*1024)
	}
	if cfg.Output.Format != "json" {
		t.Errorf("Output.Format = %q, want %q", cfg.Output.Format, "json")
	}
	if cfg.Cache.Backend != "memory" {
		t.Errorf("Cache.Backend = %q, want %q", cfg.Cache.Backend, "memory")
	}
}
