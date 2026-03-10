package plugin

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/LeGambiArt/wtmcp/internal/auth"
	"github.com/LeGambiArt/wtmcp/internal/cache"
	"github.com/LeGambiArt/wtmcp/internal/config"
	"github.com/LeGambiArt/wtmcp/internal/proxy"
)

func setupTestPlugin(t *testing.T, name, script string) string {
	t.Helper()
	dir := t.TempDir()
	pluginDir := filepath.Join(dir, name)
	if err := os.MkdirAll(pluginDir, 0o755); err != nil { //nolint:gosec // test dir
		t.Fatal(err)
	}

	handlerPath := filepath.Join(pluginDir, "handler.sh")
	if err := os.WriteFile(handlerPath, []byte(script), 0o755); err != nil { //nolint:gosec // test needs executable
		t.Fatal(err)
	}

	manifest := `
name: ` + name + `
version: "1.0.0"
description: "Test plugin"
execution: persistent
handler: ./handler.sh
tools:
  - name: ` + name + `_test
    description: "A test tool"
    params: {}
`
	manifestPath := filepath.Join(pluginDir, "plugin.yaml")
	if err := os.WriteFile(manifestPath, []byte(manifest), 0o644); err != nil { //nolint:gosec // test config
		t.Fatal(err)
	}

	return dir
}

func newTestManager(t *testing.T) *Manager {
	t.Helper()
	cfg := config.DefaultConfig()
	authReg := auth.NewRegistry()
	cacheStore := cache.NewMemoryStore()
	p := proxy.New(nil, cfg.Plugins.MaxMessageSize)
	return NewManager(authReg, p, cacheStore, cfg)
}

var echoScript = `#!/bin/bash
while IFS= read -r line; do
  type=$(echo "$line" | jq -r '.type')
  id=$(echo "$line" | jq -r '.id')
  case "$type" in
    init)
      echo "{\"id\":\"$id\",\"type\":\"init_ok\"}"
      ;;
    tool_call)
      tool=$(echo "$line" | jq -r '.tool')
      echo "{\"id\":\"$id\",\"type\":\"tool_result\",\"result\":{\"tool\":\"$tool\"}}"
      ;;
    shutdown)
      echo "{\"id\":\"$id\",\"type\":\"shutdown_ok\"}"
      exit 0
      ;;
  esac
done
`

func TestManagerDiscover(t *testing.T) {
	dir := setupTestPlugin(t, "hello", echoScript)

	m := newTestManager(t)
	if err := m.Discover([]string{dir}); err != nil {
		t.Fatalf("Discover: %v", err)
	}

	manifests := m.Manifests()
	if len(manifests) != 1 {
		t.Fatalf("got %d manifests, want 1", len(manifests))
	}
	if _, ok := manifests["hello"]; !ok {
		t.Error("expected 'hello' manifest")
	}
}

func TestManagerDiscoverSkipsNonexistentDir(t *testing.T) {
	m := newTestManager(t)
	if err := m.Discover([]string{"/nonexistent/path"}); err != nil {
		t.Fatalf("Discover should not error on missing dir: %v", err)
	}
}

func TestManagerLoadAndCallTool(t *testing.T) {
	dir := setupTestPlugin(t, "echo", echoScript)

	m := newTestManager(t)
	if err := m.Discover([]string{dir}); err != nil {
		t.Fatalf("Discover: %v", err)
	}

	ctx := context.Background()
	if err := m.LoadAll(ctx); err != nil {
		t.Fatalf("LoadAll: %v", err)
	}
	defer m.ShutdownAll(ctx)

	// Find tool owner
	owner, handle := m.CallTool(ctx, "echo_test")
	if owner != "echo" {
		t.Errorf("owner = %q, want echo", owner)
	}
	if handle == nil {
		t.Fatal("handle is nil")
	}

	// Call the tool
	result, err := handle.CallTool(ctx, "echo_test", json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}

	var parsed map[string]any
	if err := json.Unmarshal(result, &parsed); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if parsed["tool"] != "echo_test" {
		t.Errorf("tool = %v", parsed["tool"])
	}
}

func TestManagerUnknownTool(t *testing.T) {
	m := newTestManager(t)
	_, handle := m.CallTool(context.Background(), "nonexistent_tool")
	if handle != nil {
		t.Error("expected nil handle for unknown tool")
	}
}

func TestManagerTopologicalSort(t *testing.T) {
	m := newTestManager(t)

	// Create manifests with dependencies
	m.manifests["aa-base"] = &Manifest{Name: "aa-base", Priority: 10}
	m.manifests["bb-derived"] = &Manifest{Name: "bb-derived", DependsOn: []string{"aa-base"}, Priority: 20}
	m.manifests["cc-top"] = &Manifest{Name: "cc-top", DependsOn: []string{"bb-derived"}, Priority: 30}

	sorted, err := m.topologicalSort()
	if err != nil {
		t.Fatalf("topologicalSort: %v", err)
	}

	if len(sorted) != 3 {
		t.Fatalf("got %d, want 3: %v", len(sorted), sorted)
	}

	// base must come before derived, derived before top
	indexOf := func(name string) int {
		for i, n := range sorted {
			if n == name {
				return i
			}
		}
		return -1
	}

	if indexOf("aa-base") >= indexOf("bb-derived") {
		t.Errorf("aa-base should come before bb-derived: %v", sorted)
	}
	if indexOf("bb-derived") >= indexOf("cc-top") {
		t.Errorf("bb-derived should come before cc-top: %v", sorted)
	}
}

func TestManagerCircularDependency(t *testing.T) {
	m := newTestManager(t)
	m.manifests["aa"] = &Manifest{Name: "aa", DependsOn: []string{"bb"}}
	m.manifests["bb"] = &Manifest{Name: "bb", DependsOn: []string{"aa"}}

	_, err := m.topologicalSort()
	if err == nil {
		t.Error("expected circular dependency error")
	}
}

func TestManagerMissingDependency(t *testing.T) {
	m := newTestManager(t)
	m.manifests["aa"] = &Manifest{Name: "aa", DependsOn: []string{"missing"}}

	_, err := m.topologicalSort()
	if err == nil {
		t.Error("expected missing dependency error")
	}
}
