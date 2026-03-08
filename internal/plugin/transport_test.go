package plugin

import (
	"bytes"
	"encoding/json"
	"io"
	"strings"
	"testing"
	"time"
)

// mockServiceHandler implements ServiceHandler for testing.
type mockServiceHandler struct {
	httpHandler  func(pluginName string, req Message) Message
	cacheHandler func(pluginName string, req Message) Message
}

func (m *mockServiceHandler) HandleHTTP(pluginName string, req Message) Message {
	if m.httpHandler != nil {
		return m.httpHandler(pluginName, req)
	}
	return Message{ID: req.ID, Type: TypeHTTPResponse, Status: 200}
}

func (m *mockServiceHandler) HandleCache(pluginName string, req Message) Message {
	if m.cacheHandler != nil {
		return m.cacheHandler(pluginName, req)
	}
	hit := true
	return Message{ID: req.ID, Type: TypeCacheGet, Hit: &hit}
}

func TestTransportSend(t *testing.T) {
	var buf bytes.Buffer
	tr := NewTransport(&buf, strings.NewReader(""), strings.NewReader(""), 1024*1024)

	msg := Message{ID: "test-1", Type: TypeToolCall, Tool: "hello"}
	if err := tr.Send(msg); err != nil {
		t.Fatalf("Send failed: %v", err)
	}

	// Verify output is a single JSON line
	line := buf.String()
	if !strings.HasSuffix(line, "\n") {
		t.Error("message should end with newline")
	}
	line = strings.TrimSpace(line)

	var decoded Message
	if err := json.Unmarshal([]byte(line), &decoded); err != nil {
		t.Fatalf("output is not valid JSON: %v", err)
	}
	if decoded.ID != "test-1" {
		t.Errorf("ID = %q, want %q", decoded.ID, "test-1")
	}
	if decoded.Type != TypeToolCall {
		t.Errorf("Type = %q, want %q", decoded.Type, TypeToolCall)
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
	toolResult := Message{ID: "req-1", Type: TypeToolResult, Result: json.RawMessage(`{"ok":true}`)}
	data, _ := json.Marshal(toolResult)

	pluginStdout := strings.NewReader(string(data) + "\n")
	var pluginStdin bytes.Buffer

	tr := NewTransport(&pluginStdin, pluginStdout, strings.NewReader(""), 1024*1024)

	handler := &mockServiceHandler{}
	go tr.ReadLoop("test-plugin", 1, handler)

	// Register pending before ReadLoop processes the message
	ch := make(chan Message, 1)
	tr.pending.Store("req-1", ch)

	select {
	case resp := <-ch:
		if resp.Type != TypeToolResult {
			t.Errorf("Type = %q, want %q", resp.Type, TypeToolResult)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for tool_result")
	}
}

func TestReadLoopHandlesHTTPSync(t *testing.T) {
	// Plugin sends an http_request, then a tool_result
	httpReq := Message{ID: "http-1", Type: TypeHTTPRequest, Method: "GET", Path: "/test"}
	toolResult := Message{ID: "req-1", Type: TypeToolResult, Result: json.RawMessage(`{}`)}

	var lines []string
	for _, msg := range []Message{httpReq, toolResult} {
		data, _ := json.Marshal(msg)
		lines = append(lines, string(data))
	}
	pluginStdout := strings.NewReader(strings.Join(lines, "\n") + "\n")

	var pluginStdin bytes.Buffer
	tr := NewTransport(&pluginStdin, pluginStdout, strings.NewReader(""), 1024*1024)

	httpCalled := false
	handler := &mockServiceHandler{
		httpHandler: func(_ string, req Message) Message {
			httpCalled = true
			return Message{ID: req.ID, Type: TypeHTTPResponse, Status: 200}
		},
	}

	// Register pending for tool_result
	ch := make(chan Message, 1)
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

	ch := make(chan Message, 1)
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
	err := &Error{Code: "api_error", Message: "not found"}
	expected := "[api_error] not found"
	if err.Error() != expected {
		t.Errorf("Error() = %q, want %q", err.Error(), expected)
	}
}
