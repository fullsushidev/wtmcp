package handler

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"sync/atomic"
)

// ToolFunc is a function that handles a tool call.
// It receives the raw JSON params and the plugin config,
// and returns a result to be sent back to the core.
type ToolFunc func(params, config json.RawMessage) (any, error)

// Plugin implements a persistent plugin handler.
// Register tool functions with Handle, then call Run.
type Plugin struct {
	tools  map[string]ToolFunc
	in     *bufio.Scanner
	out    io.Writer
	nextID atomic.Int64
	initFn func(config json.RawMessage) error
	logger *log.Logger
}

// New creates a new Plugin that reads from stdin and writes to stdout.
// Logging is directed to stderr (captured by the core with a plugin prefix).
func New() *Plugin {
	s := bufio.NewScanner(os.Stdin)
	s.Buffer(make([]byte, 0, 1024*1024), 10*1024*1024) // 10MB max line
	return &Plugin{
		tools:  make(map[string]ToolFunc),
		in:     s,
		out:    os.Stdout,
		logger: log.New(os.Stderr, "", 0),
	}
}

// Handle registers a tool function by name.
func (p *Plugin) Handle(name string, fn ToolFunc) {
	p.tools[name] = fn
}

// OnInit registers a function called during plugin initialization.
// The function receives the plugin config from the core.
func (p *Plugin) OnInit(fn func(config json.RawMessage) error) {
	p.initFn = fn
}

// Log writes a message to stderr (captured by the core).
func (p *Plugin) Log(format string, args ...any) {
	p.logger.Printf(format, args...)
}

// Run starts the handler main loop. It processes messages from stdin
// until shutdown or EOF, then returns.
func (p *Plugin) Run() error {
	for {
		msg, err := p.recv()
		if err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			return fmt.Errorf("recv: %w", err)
		}

		switch msg.Type {
		case TypeInit:
			p.handleInit(msg)
		case TypeToolCall:
			p.handleToolCall(msg)
		case TypeShutdown:
			p.send(Message{ID: msg.ID, Type: TypeShutdownOK})
			return nil
		}
	}
}

func (p *Plugin) handleInit(msg Message) {
	if p.initFn != nil {
		if err := p.initFn(msg.Config); err != nil {
			p.send(Message{
				ID:   msg.ID,
				Type: TypeInitError,
				Error: &Error{
					Code:    "init_failed",
					Message: err.Error(),
				},
			})
			return
		}
	}
	p.send(Message{ID: msg.ID, Type: TypeInitOK})
}

func (p *Plugin) handleToolCall(msg Message) {
	fn, ok := p.tools[msg.Tool]
	if !ok {
		p.send(Message{
			ID:   msg.ID,
			Type: TypeToolResult,
			Error: &Error{
				Code:    "unknown_tool",
				Message: fmt.Sprintf("unknown tool: %s", msg.Tool),
			},
		})
		return
	}

	result, err := fn(msg.Params, msg.Config)
	if err != nil {
		e := &Error{Code: "handler_error", Message: err.Error()}
		if pe, ok := err.(*Error); ok { //nolint:errorlint // exact type match intended
			e = pe
		}
		p.send(Message{
			ID:    msg.ID,
			Type:  TypeToolResult,
			Error: e,
		})
		return
	}

	data, err := json.Marshal(result)
	if err != nil {
		p.send(Message{
			ID:   msg.ID,
			Type: TypeToolResult,
			Error: &Error{
				Code:    "marshal_error",
				Message: fmt.Sprintf("marshal result: %v", err),
			},
		})
		return
	}

	p.send(Message{
		ID:     msg.ID,
		Type:   TypeToolResult,
		Result: data,
	})
}

func (p *Plugin) recv() (Message, error) {
	if !p.in.Scan() {
		if err := p.in.Err(); err != nil {
			return Message{}, err
		}
		return Message{}, io.EOF
	}
	var msg Message
	if err := json.Unmarshal(p.in.Bytes(), &msg); err != nil {
		return Message{}, fmt.Errorf("unmarshal: %w", err)
	}
	return msg, nil
}

func (p *Plugin) send(msg Message) {
	data, err := json.Marshal(msg)
	if err != nil {
		p.logger.Printf("marshal send: %v", err)
		return
	}
	data = append(data, '\n')
	_, _ = p.out.Write(data)
}

// nextMsgID generates a unique message ID for service requests.
func (p *Plugin) nextMsgID(prefix string) string {
	return fmt.Sprintf("%s-%d", prefix, p.nextID.Add(1))
}
