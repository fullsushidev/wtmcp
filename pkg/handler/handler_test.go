package handler

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"
	"log"
	"strings"
	"testing"
)

func newTestPlugin(input string) (*Plugin, *bytes.Buffer) {
	out := &bytes.Buffer{}
	s := bufio.NewScanner(strings.NewReader(input))
	s.Buffer(make([]byte, 0, 1024*1024), 10*1024*1024)
	return &Plugin{
		tools:  make(map[string]ToolFunc),
		in:     s,
		out:    out,
		logger: log.New(io.Discard, "", 0),
	}, out
}

func readMessages(t *testing.T, out *bytes.Buffer) []Message {
	t.Helper()
	var msgs []Message
	scanner := bufio.NewScanner(out)
	for scanner.Scan() {
		var msg Message
		if err := json.Unmarshal(scanner.Bytes(), &msg); err != nil {
			t.Fatalf("unmarshal output: %v", err)
		}
		msgs = append(msgs, msg)
	}
	return msgs
}

func TestInitAndShutdown(t *testing.T) {
	input := `{"id":"init-1","type":"init","config":{}}` + "\n" +
		`{"id":"shutdown-1","type":"shutdown"}` + "\n"

	p, out := newTestPlugin(input)

	var gotConfig json.RawMessage
	p.OnInit(func(config json.RawMessage) error {
		gotConfig = config
		return nil
	})

	if err := p.Run(); err != nil {
		t.Fatalf("Run: %v", err)
	}

	if string(gotConfig) != "{}" {
		t.Errorf("init config = %s, want {}", gotConfig)
	}

	msgs := readMessages(t, out)
	if len(msgs) != 2 {
		t.Fatalf("got %d messages, want 2", len(msgs))
	}
	if msgs[0].Type != TypeInitOK || msgs[0].ID != "init-1" {
		t.Errorf("msg[0] = %s/%s, want init_ok/init-1", msgs[0].Type, msgs[0].ID)
	}
	if msgs[1].Type != TypeShutdownOK || msgs[1].ID != "shutdown-1" {
		t.Errorf("msg[1] = %s/%s, want shutdown_ok/shutdown-1", msgs[1].Type, msgs[1].ID)
	}
}

func TestToolCall(t *testing.T) {
	input := `{"id":"init-1","type":"init","config":{}}` + "\n" +
		`{"id":"req-1","type":"tool_call","tool":"greet","params":{"name":"world"}}` + "\n" +
		`{"id":"shutdown-1","type":"shutdown"}` + "\n"

	p, out := newTestPlugin(input)
	p.Handle("greet", func(params, _ json.RawMessage) (any, error) {
		var p struct {
			Name string `json:"name"`
		}
		if err := json.Unmarshal(params, &p); err != nil {
			return nil, err
		}
		return map[string]string{"greeting": "hello " + p.Name}, nil
	})

	if err := p.Run(); err != nil {
		t.Fatalf("Run: %v", err)
	}

	msgs := readMessages(t, out)
	if len(msgs) != 3 {
		t.Fatalf("got %d messages, want 3", len(msgs))
	}

	result := msgs[1]
	if result.Type != TypeToolResult || result.ID != "req-1" {
		t.Errorf("result = %s/%s, want tool_result/req-1", result.Type, result.ID)
	}
	if result.Error != nil {
		t.Errorf("unexpected error: %v", result.Error)
	}

	var got map[string]string
	if err := json.Unmarshal(result.Result, &got); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if got["greeting"] != "hello world" {
		t.Errorf("greeting = %q, want %q", got["greeting"], "hello world")
	}
}

func TestToolCallError(t *testing.T) {
	input := `{"id":"init-1","type":"init","config":{}}` + "\n" +
		`{"id":"req-1","type":"tool_call","tool":"fail","params":{}}` + "\n" +
		`{"id":"shutdown-1","type":"shutdown"}` + "\n"

	p, out := newTestPlugin(input)
	p.Handle("fail", func(_, _ json.RawMessage) (any, error) {
		return nil, &Error{Code: "test_error", Message: "something broke"}
	})

	if err := p.Run(); err != nil {
		t.Fatalf("Run: %v", err)
	}

	msgs := readMessages(t, out)
	result := msgs[1]
	if result.Error == nil {
		t.Fatal("expected error in result")
	}
	if result.Error.Code != "test_error" {
		t.Errorf("error code = %q, want %q", result.Error.Code, "test_error")
	}
}

func TestUnknownTool(t *testing.T) {
	input := `{"id":"init-1","type":"init","config":{}}` + "\n" +
		`{"id":"req-1","type":"tool_call","tool":"nonexistent","params":{}}` + "\n" +
		`{"id":"shutdown-1","type":"shutdown"}` + "\n"

	p, out := newTestPlugin(input)

	if err := p.Run(); err != nil {
		t.Fatalf("Run: %v", err)
	}

	msgs := readMessages(t, out)
	result := msgs[1]
	if result.Error == nil {
		t.Fatal("expected error for unknown tool")
	}
	if result.Error.Code != "unknown_tool" {
		t.Errorf("error code = %q, want %q", result.Error.Code, "unknown_tool")
	}
}

func TestEOFWithoutShutdown(t *testing.T) {
	// Handler should exit gracefully on EOF (core crashed/killed)
	input := `{"id":"init-1","type":"init","config":{}}` + "\n"

	p, _ := newTestPlugin(input)
	if err := p.Run(); err != nil {
		t.Fatalf("Run should return nil on EOF, got: %v", err)
	}
}
