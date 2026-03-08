package config

import (
	"os"
	"path/filepath"
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

func TestLoadConfigFile(t *testing.T) {
	dir := t.TempDir()
	cfgFile := filepath.Join(dir, "config.yaml")

	yaml := `
plugin_dirs:
  - /opt/plugins
  - /home/user/plugins
output:
  format: toon
  toon_fallback: true
cache:
  backend: filesystem
  dir: /tmp/cache
`
	if err := os.WriteFile(cfgFile, []byte(yaml), 0o600); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(cfgFile, dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if len(cfg.PluginDirs) != 2 {
		t.Errorf("PluginDirs = %v, want 2 entries", cfg.PluginDirs)
	}
	if cfg.Output.Format != "toon" {
		t.Errorf("Output.Format = %q, want toon", cfg.Output.Format)
	}
	if cfg.Cache.Backend != "filesystem" {
		t.Errorf("Cache.Backend = %q, want filesystem", cfg.Cache.Backend)
	}
	// Defaults should still apply for unset fields
	if cfg.Plugins.ToolCallTimeout == 0 {
		t.Error("ToolCallTimeout should have default value")
	}
}

func TestLoadConfigMissing(t *testing.T) {
	cfg, err := Load("/nonexistent/config.yaml", "/nonexistent")
	if err != nil {
		t.Fatalf("should not error on missing file: %v", err)
	}
	if cfg.Output.Format != "json" {
		t.Errorf("should return defaults, got format=%q", cfg.Output.Format)
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
