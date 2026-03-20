package plugin

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/LeGambiArt/wtmcp/internal/protocol"
)

// mockServiceHandler implements ServiceHandler for testing.
type mockServiceHandler struct {
	httpHandler  func(ctx context.Context, pluginName string, req protocol.Message) protocol.Message
	cacheHandler func(ctx context.Context, pluginName string, req protocol.Message) protocol.Message
}

func (m *mockServiceHandler) HandleHTTP(ctx context.Context, pluginName string, req protocol.Message) protocol.Message {
	if m.httpHandler != nil {
		return m.httpHandler(ctx, pluginName, req)
	}
	return protocol.Message{ID: req.ID, Type: protocol.TypeHTTPResponse, Status: 200}
}

func (m *mockServiceHandler) HandleCache(ctx context.Context, pluginName string, req protocol.Message) protocol.Message {
	if m.cacheHandler != nil {
		return m.cacheHandler(ctx, pluginName, req)
	}
	hit := true
	return protocol.Message{ID: req.ID, Type: protocol.TypeCacheGet, Hit: &hit}
}

func TestTransportSend(t *testing.T) {
	var buf bytes.Buffer
	tr := NewTransport(&buf, strings.NewReader(""), strings.NewReader(""), 1024*1024)

	msg := protocol.Message{ID: "test-1", Type: protocol.TypeToolCall, Tool: "hello"}
	if err := tr.Send(msg); err != nil {
		t.Fatalf("Send failed: %v", err)
	}

	// Verify output is a single JSON line
	line := buf.String()
	if !strings.HasSuffix(line, "\n") {
		t.Error("message should end with newline")
	}
	line = strings.TrimSpace(line)

	var decoded protocol.Message
	if err := json.Unmarshal([]byte(line), &decoded); err != nil {
		t.Fatalf("output is not valid JSON: %v", err)
	}
	if decoded.ID != "test-1" {
		t.Errorf("ID = %q, want %q", decoded.ID, "test-1")
	}
	if decoded.Type != protocol.TypeToolCall {
		t.Errorf("Type = %q, want %q", decoded.Type, protocol.TypeToolCall)
	}
}

func TestTransportGenerateID(t *testing.T) {
	tr := NewTransport(io.Discard, strings.NewReader(""), strings.NewReader(""), 1024)

	id1 := tr.GenerateID("http")
	id2 := tr.GenerateID("http")
	id3 := tr.GenerateID("cache")

	if id1 == id2 {
		t.Error("IDs should be unique")
	}
	if !strings.HasPrefix(id3, "cache-") {
		t.Errorf("ID %q should have prefix 'cache-'", id3)
	}
}

func TestReadLoopRoutesToolResult(t *testing.T) {
	// Simulate plugin sending a tool_result
	toolResult := protocol.Message{ID: "req-1", Type: protocol.TypeToolResult, Result: json.RawMessage(`{"ok":true}`)}
	data, _ := json.Marshal(toolResult)

	pluginStdout := strings.NewReader(string(data) + "\n")
	var pluginStdin bytes.Buffer

	tr := NewTransport(&pluginStdin, pluginStdout, strings.NewReader(""), 1024*1024)

	handler := &mockServiceHandler{}
	go tr.ReadLoop("test-plugin", 1, handler)

	// Register pending before ReadLoop processes the message
	ch := make(chan protocol.Message, 1)
	tr.pending.Store("req-1", ch)

	select {
	case resp := <-ch:
		if resp.Type != protocol.TypeToolResult {
			t.Errorf("Type = %q, want %q", resp.Type, protocol.TypeToolResult)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for tool_result")
	}
}

func TestReadLoopHandlesHTTPSync(t *testing.T) {
	// Plugin sends an http_request, then a tool_result
	httpReq := protocol.Message{ID: "http-1", Type: protocol.TypeHTTPRequest, Method: "GET", Path: "/test"}
	toolResult := protocol.Message{ID: "req-1", Type: protocol.TypeToolResult, Result: json.RawMessage(`{}`)}

	var lines []string
	for _, msg := range []protocol.Message{httpReq, toolResult} {
		data, _ := json.Marshal(msg)
		lines = append(lines, string(data))
	}
	pluginStdout := strings.NewReader(strings.Join(lines, "\n") + "\n")

	var pluginStdin bytes.Buffer
	tr := NewTransport(&pluginStdin, pluginStdout, strings.NewReader(""), 1024*1024)

	httpCalled := false
	handler := &mockServiceHandler{
		httpHandler: func(_ context.Context, _ string, req protocol.Message) protocol.Message {
			httpCalled = true
			return protocol.Message{ID: req.ID, Type: protocol.TypeHTTPResponse, Status: 200}
		},
	}

	// Register pending for tool_result
	ch := make(chan protocol.Message, 1)
	tr.pending.Store("req-1", ch)

	go tr.ReadLoop("test-plugin", 1, handler)

	select {
	case <-ch:
		// got tool_result
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for tool_result")
	}

	if !httpCalled {
		t.Error("HTTP handler should have been called")
	}

	// Verify http_response was written back
	output := pluginStdin.String()
	if !strings.Contains(output, `"http_response"`) {
		t.Error("http_response should have been written to plugin stdin")
	}
}

func TestReadLoopDrainsPendingOnExit(t *testing.T) {
	// Empty stdout — ReadLoop will exit immediately
	tr := NewTransport(io.Discard, strings.NewReader(""), strings.NewReader(""), 1024)

	ch := make(chan protocol.Message, 1)
	tr.pending.Store("req-1", ch)

	handler := &mockServiceHandler{}
	go tr.ReadLoop("test-plugin", 1, handler)

	// Channel should be closed when ReadLoop exits
	select {
	case _, ok := <-ch:
		if ok {
			t.Error("channel should be closed, not receive a message")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout — pending channel was not closed")
	}
}

func TestError(t *testing.T) {
	err := &protocol.Error{Code: "api_error", Message: "not found"}
	expected := "[api_error] not found"
	if err.Error() != expected {
		t.Errorf("Error() = %q, want %q", err.Error(), expected)
	}
}
