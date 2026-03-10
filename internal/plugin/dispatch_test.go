package plugin

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/LeGambiArt/wtmcp/internal/protocol"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func writeTestHandler(t *testing.T, dir, script string) {
	t.Helper()
	path := filepath.Join(dir, "handler.sh")
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil { //nolint:gosec // test needs executable
		t.Fatal(err)
	}
}

func testProcessConfig() ProcessConfig {
	return ProcessConfig{
		InitTimeout:       5 * time.Second,
		ShutdownTimeout:   5 * time.Second,
		ShutdownKillAfter: 2 * time.Second,
		MaxMessageSize:    1024 * 1024,
	}
}

func TestCallToolPersistent(t *testing.T) {
	dir := t.TempDir()

	// A persistent handler that echoes tool calls as results
	script := `#!/bin/bash
while IFS= read -r line; do
  type=$(echo "$line" | jq -r '.type')
  id=$(echo "$line" | jq -r '.id')
  case "$type" in
    init)
      echo "{\"id\":\"$id\",\"type\":\"init_ok\"}"
      ;;
    tool_call)
      tool=$(echo "$line" | jq -r '.tool')
      echo "{\"id\":\"$id\",\"type\":\"tool_result\",\"result\":{\"tool\":\"$tool\",\"echo\":true}}"
      ;;
    shutdown)
      echo "{\"id\":\"$id\",\"type\":\"shutdown_ok\"}"
      exit 0
      ;;
  esac
done
`
	writeTestHandler(t, dir, script)

	m := &Manifest{
		Name:        "echo-plugin",
		Execution:   "persistent",
		Handler:     "./handler.sh",
		Concurrency: 1,
		Dir:         dir,
	}

	h := NewHandle(m, &mockServiceHandler{}, testProcessConfig(), 5*time.Second, nil)

	ctx := context.Background()
	if err := h.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer h.Stop(ctx) //nolint:errcheck // test cleanup

	result, err := h.CallTool(ctx, "test_tool", json.RawMessage(`{"key":"value"}`))
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}

	var parsed map[string]any
	if err := json.Unmarshal(result, &parsed); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if parsed["tool"] != "test_tool" {
		t.Errorf("tool = %v, want test_tool", parsed["tool"])
	}
	if parsed["echo"] != true {
		t.Errorf("echo = %v, want true", parsed["echo"])
	}
}

func TestCallToolOneshot(t *testing.T) {
	dir := t.TempDir()

	script := `#!/bin/bash
read -r INPUT
ID=$(echo "$INPUT" | jq -r '.id')
TOOL=$(echo "$INPUT" | jq -r '.tool')
echo "{}" | jq -c --arg id "$ID" --arg tool "$TOOL" \
  '{id: $id, type: "tool_result", result: {tool: $tool, oneshot: true}}'
`
	writeTestHandler(t, dir, script)

	m := &Manifest{
		Name:        "oneshot-plugin",
		Execution:   "oneshot",
		Handler:     "./handler.sh",
		Concurrency: 1,
		Dir:         dir,
	}

	h := NewHandle(m, &mockServiceHandler{}, testProcessConfig(), 5*time.Second, nil)

	result, err := h.CallTool(context.Background(), "greet", json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}

	var parsed map[string]any
	if err := json.Unmarshal(result, &parsed); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if parsed["oneshot"] != true {
		t.Errorf("oneshot = %v, want true", parsed["oneshot"])
	}
}

func TestCallToolOneshotWithHTTPProxy(t *testing.T) {
	dir := t.TempDir()

	// Oneshot handler that makes an HTTP request via the proxy
	script := `#!/bin/bash
read -r INPUT
ID=$(echo "$INPUT" | jq -r '.id')

# Send http_request to core
echo '{"id":"http-1","type":"http_request","method":"GET","path":"/test"}'

# Read http_response
read -r HTTP_RESP
STATUS=$(echo "$HTTP_RESP" | jq -r '.status')

# Return tool_result
echo "{}" | jq -c --arg id "$ID" --arg status "$STATUS" \
  '{id: $id, type: "tool_result", result: {status: ($status | tonumber)}}'
`
	writeTestHandler(t, dir, script)

	m := &Manifest{
		Name:        "http-oneshot",
		Execution:   "oneshot",
		Handler:     "./handler.sh",
		Concurrency: 1,
		Dir:         dir,
	}

	handler := &mockServiceHandler{
		httpHandler: func(_ string, req protocol.Message) protocol.Message {
			return protocol.Message{ID: req.ID, Type: protocol.TypeHTTPResponse, Status: 200}
		},
	}

	h := NewHandle(m, handler, testProcessConfig(), 5*time.Second, nil)

	result, err := h.CallTool(context.Background(), "fetch", json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}

	var parsed map[string]any
	if err := json.Unmarshal(result, &parsed); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	// status comes back as float64 from JSON
	if parsed["status"] != float64(200) {
		t.Errorf("status = %v, want 200", parsed["status"])
	}
}

func TestCallToolPersistentError(t *testing.T) {
	dir := t.TempDir()

	script := `#!/bin/bash
while IFS= read -r line; do
  type=$(echo "$line" | jq -r '.type')
  id=$(echo "$line" | jq -r '.id')
  case "$type" in
    init)
      echo "{\"id\":\"$id\",\"type\":\"init_ok\"}"
      ;;
    tool_call)
      echo "{\"id\":\"$id\",\"type\":\"tool_result\",\"error\":{\"code\":\"not_found\",\"message\":\"item missing\"}}"
      ;;
    shutdown)
      echo "{\"id\":\"$id\",\"type\":\"shutdown_ok\"}"
      exit 0
      ;;
  esac
done
`
	writeTestHandler(t, dir, script)

	m := &Manifest{
		Name:        "error-plugin",
		Execution:   "persistent",
		Handler:     "./handler.sh",
		Concurrency: 1,
		Dir:         dir,
	}

	h := NewHandle(m, &mockServiceHandler{}, testProcessConfig(), 5*time.Second, nil)
	ctx := context.Background()
	if err := h.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer h.Stop(ctx) //nolint:errcheck // test cleanup

	_, err := h.CallTool(ctx, "get_item", json.RawMessage(`{}`))
	if err == nil {
		t.Fatal("expected error")
	}

	var pluginErr *protocol.Error
	if !errors.As(err, &pluginErr) {
		t.Fatalf("expected *protocol.Error, got %T", err)
	}
	if pluginErr.Code != "not_found" {
		t.Errorf("code = %q, want not_found", pluginErr.Code)
	}
}

func TestCallToolMultipleCalls(t *testing.T) {
	dir := t.TempDir()

	// Counter-based handler: returns call number
	script := `#!/bin/bash
N=0
while IFS= read -r line; do
  type=$(echo "$line" | jq -r '.type')
  id=$(echo "$line" | jq -r '.id')
  case "$type" in
    init)
      echo "{\"id\":\"$id\",\"type\":\"init_ok\"}"
      ;;
    tool_call)
      N=$((N+1))
      echo "{\"id\":\"$id\",\"type\":\"tool_result\",\"result\":{\"call\":$N}}"
      ;;
    shutdown)
      echo "{\"id\":\"$id\",\"type\":\"shutdown_ok\"}"
      exit 0
      ;;
  esac
done
`
	writeTestHandler(t, dir, script)

	m := &Manifest{
		Name:        "counter-plugin",
		Execution:   "persistent",
		Handler:     "./handler.sh",
		Concurrency: 1,
		Dir:         dir,
	}

	h := NewHandle(m, &mockServiceHandler{}, testProcessConfig(), 5*time.Second, nil)
	ctx := context.Background()
	if err := h.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer h.Stop(ctx) //nolint:errcheck // test cleanup

	for i := 1; i <= 3; i++ {
		result, err := h.CallTool(ctx, "count", json.RawMessage(`{}`))
		if err != nil {
			t.Fatalf("call %d: %v", i, err)
		}
		var parsed map[string]any
		if err := json.Unmarshal(result, &parsed); err != nil {
			t.Fatalf("unmarshal call %d: %v", i, err)
		}
		if int(parsed["call"].(float64)) != i {
			t.Errorf("call %d: got %v", i, parsed["call"])
		}
	}
}
