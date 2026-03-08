package plugin

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadManifest(t *testing.T) {
	dir := t.TempDir()

	// Create a handler executable
	handlerPath := filepath.Join(dir, "handler.sh")
	if err := os.WriteFile(handlerPath, []byte("#!/bin/bash\n"), 0o755); err != nil { //nolint:gosec // test needs executable
		t.Fatal(err)
	}

	manifest := `
name: test-plugin
version: "1.0.0"
description: "A test plugin"
execution: persistent
handler: ./handler.sh
depends_on: []

services:
  http:
    base_url: "https://api.example.com/v1"
  cache:
    default_ttl: 300

config:
  api_key: "${API_KEY:-default}"

tools:
  - name: test_tool
    description: "A test tool"
    params:
      query:
        type: string
        required: true
        description: "Search query"
      limit:
        type: integer
        default: 10

context_files:
  - context.md

priority: 50
enabled: true
`

	manifestPath := filepath.Join(dir, "plugin.yaml")
	if err := os.WriteFile(manifestPath, []byte(manifest), 0o644); err != nil { //nolint:gosec // test config file
		t.Fatal(err)
	}

	m, err := LoadManifest(manifestPath)
	if err != nil {
		t.Fatalf("LoadManifest failed: %v", err)
	}

	if m.Name != "test-plugin" {
		t.Errorf("Name = %q", m.Name)
	}
	if m.Version != "1.0.0" {
		t.Errorf("Version = %q", m.Version)
	}
	if m.Execution != "persistent" {
		t.Errorf("Execution = %q", m.Execution)
	}
	if m.Concurrency != 1 {
		t.Errorf("Concurrency = %d, want 1 (default)", m.Concurrency)
	}
	if m.Services.HTTP.BaseURL != "https://api.example.com/v1" {
		t.Errorf("BaseURL = %q", m.Services.HTTP.BaseURL)
	}
	if len(m.Tools) != 1 {
		t.Fatalf("got %d tools, want 1", len(m.Tools))
	}
	if m.Tools[0].Name != "test_tool" {
		t.Errorf("Tool name = %q", m.Tools[0].Name)
	}
	if !m.IsEnabled() {
		t.Error("should be enabled")
	}
	if m.Dir != dir {
		t.Errorf("Dir = %q, want %q", m.Dir, dir)
	}
}

func TestLoadManifestDefaults(t *testing.T) {
	dir := t.TempDir()
	handlerPath := filepath.Join(dir, "handler")
	if err := os.WriteFile(handlerPath, []byte("#!/bin/bash\n"), 0o755); err != nil { //nolint:gosec // test needs executable
		t.Fatal(err)
	}

	manifest := `
name: simple
version: "1.0.0"
description: "Minimal"
handler: ./handler
tools: []
`
	path := filepath.Join(dir, "plugin.yaml")
	if err := os.WriteFile(path, []byte(manifest), 0o644); err != nil { //nolint:gosec // test config file
		t.Fatal(err)
	}

	m, err := LoadManifest(path)
	if err != nil {
		t.Fatalf("LoadManifest failed: %v", err)
	}

	if m.Execution != "persistent" {
		t.Errorf("default Execution = %q, want persistent", m.Execution)
	}
	if m.Concurrency != 1 {
		t.Errorf("default Concurrency = %d, want 1", m.Concurrency)
	}
	if !m.IsEnabled() {
		t.Error("default should be enabled")
	}
	if !m.CacheEnabled() {
		t.Error("default cache should be enabled")
	}
	if m.CacheNamespace() != "simple" {
		t.Errorf("default CacheNamespace = %q, want %q", m.CacheNamespace(), "simple")
	}
}

func TestManifestValidation(t *testing.T) {
	dir := t.TempDir()
	handlerPath := filepath.Join(dir, "handler")
	if err := os.WriteFile(handlerPath, []byte("#!/bin/bash\n"), 0o755); err != nil { //nolint:gosec // test needs executable
		t.Fatal(err)
	}

	tests := []struct {
		name    string
		yaml    string
		wantErr string
	}{
		{
			name:    "invalid name - uppercase",
			yaml:    `name: BadName` + "\nversion: '1.0'\nhandler: ./handler\ntools: []",
			wantErr: "invalid plugin name",
		},
		{
			name:    "invalid name - too short",
			yaml:    `name: x` + "\nversion: '1.0'\nhandler: ./handler\ntools: []",
			wantErr: "invalid plugin name",
		},
		{
			name:    "invalid execution",
			yaml:    `name: ok-name` + "\nversion: '1.0'\nexecution: invalid\nhandler: ./handler\ntools: []",
			wantErr: "execution must be",
		},
		{
			name:    "missing handler",
			yaml:    `name: ok-name` + "\nversion: '1.0'\ntools: []",
			wantErr: "handler is required",
		},
		{
			name:    "handler escapes dir",
			yaml:    `name: ok-name` + "\nversion: '1.0'\nhandler: ../../etc/passwd\ntools: []",
			wantErr: "escapes plugin directory",
		},
		{
			name:    "base_url with query",
			yaml:    `name: ok-name` + "\nversion: '1.0'\nhandler: ./handler\ntools: []\nservices:\n  http:\n    base_url: 'https://api.com?foo=bar'",
			wantErr: "must not contain query",
		},
		{
			name:    "empty tool name",
			yaml:    `name: ok-name` + "\nversion: '1.0'\nhandler: ./handler\ntools:\n  - name: ''\n    description: test",
			wantErr: "tool name is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(dir, "plugin.yaml")
			if err := os.WriteFile(path, []byte(tt.yaml), 0o644); err != nil { //nolint:gosec // test config file
				t.Fatal(err)
			}
			_, err := LoadManifest(path)
			if err == nil {
				t.Fatal("expected error")
			}
			if tt.wantErr != "" {
				if !contains(err.Error(), tt.wantErr) {
					t.Errorf("error = %q, want substring %q", err.Error(), tt.wantErr)
				}
			}
		})
	}
}

func TestParamsSchema(t *testing.T) {
	tool := ToolDef{
		Name: "test",
		Params: map[string]ParamDef{
			"query": {
				Type:        "string",
				Description: "Search query",
				Required:    true,
			},
			"limit": {
				Type:    "integer",
				Default: 10,
			},
			"fields": {
				Type:  "array",
				Items: &ItemsDef{Type: "string"},
			},
		},
	}

	schema := tool.ParamsSchema()

	if schema["type"] != "object" {
		t.Errorf("type = %v", schema["type"])
	}

	props, ok := schema["properties"].(map[string]any)
	if !ok {
		t.Fatal("properties is not a map")
	}
	if len(props) != 3 {
		t.Errorf("got %d properties, want 3", len(props))
	}

	queryProp := props["query"].(map[string]any)
	if queryProp["type"] != "string" {
		t.Errorf("query type = %v", queryProp["type"])
	}
	if queryProp["description"] != "Search query" {
		t.Errorf("query description = %v", queryProp["description"])
	}

	required, ok := schema["required"].([]string)
	if !ok {
		t.Fatal("required is not a string slice")
	}
	if len(required) != 1 || required[0] != "query" {
		t.Errorf("required = %v, want [query]", required)
	}

	fieldsProp := props["fields"].(map[string]any)
	items, ok := fieldsProp["items"].(map[string]any)
	if !ok {
		t.Fatal("fields items is not a map")
	}
	if items["type"] != "string" {
		t.Errorf("fields items type = %v", items["type"])
	}
}

func TestManifestAuthVariantOrder(t *testing.T) {
	dir := t.TempDir()
	handlerPath := filepath.Join(dir, "handler")
	if err := os.WriteFile(handlerPath, []byte("#!/bin/bash\n"), 0o755); err != nil { //nolint:gosec // test needs executable
		t.Fatal(err)
	}

	manifest := `
name: test-auth
version: "1.0.0"
description: "Auth variants test"
handler: ./handler
tools: []
services:
  auth:
    select: auto
    variants:
      cloud:
        type: basic
        username: user
        password: pass
      server-token:
        type: bearer
        token: tok
      server-kerberos:
        type: bearer
        token: tok2
`
	path := filepath.Join(dir, "plugin.yaml")
	if err := os.WriteFile(path, []byte(manifest), 0o644); err != nil { //nolint:gosec // test config file
		t.Fatal(err)
	}

	m, err := LoadManifest(path)
	if err != nil {
		t.Fatalf("LoadManifest failed: %v", err)
	}

	expected := []string{"cloud", "server-token", "server-kerberos"}
	if len(m.Services.Auth.VariantOrder) != len(expected) {
		t.Fatalf("VariantOrder = %v, want %v", m.Services.Auth.VariantOrder, expected)
	}
	for i, name := range expected {
		if m.Services.Auth.VariantOrder[i] != name {
			t.Errorf("VariantOrder[%d] = %q, want %q", i, m.Services.Auth.VariantOrder[i], name)
		}
	}
}

func TestProvidesAuth(t *testing.T) {
	m := &Manifest{}
	if m.ProvidesAuth() {
		t.Error("empty manifest should not provide auth")
	}

	m.Provides.Auth = &ProvidesAuthConfig{Type: "custom-sso/v1"}
	if !m.ProvidesAuth() {
		t.Error("should provide auth")
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && searchSubstring(s, substr)
}

func searchSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
