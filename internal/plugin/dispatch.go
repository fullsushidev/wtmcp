package plugin

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"github.com/LeGambiArt/wtmcp/internal/protocol"
	"log"
	"os/exec"
	"sync"
	"time"
)

// Handle wraps a plugin process and serializes tool calls based on
// the plugin's concurrency setting.
type Handle struct {
	process     *Process
	manifest    *Manifest
	handler     ServiceHandler
	processCfg  ProcessConfig
	mu          sync.Mutex // serialize tool calls for concurrency:1
	toolTimeout time.Duration
}

// NewHandle creates a Handle for dispatching tool calls to a plugin.
func NewHandle(manifest *Manifest, handler ServiceHandler, cfg ProcessConfig, toolTimeout time.Duration) *Handle {
	return &Handle{
		manifest:    manifest,
		handler:     handler,
		processCfg:  cfg,
		toolTimeout: toolTimeout,
	}
}

// Start launches the plugin process.
func (h *Handle) Start(ctx context.Context) error {
	h.process = NewProcess(h.manifest, h.handler, h.processCfg)
	return h.process.Start(ctx)
}

// Stop gracefully shuts down the plugin process.
func (h *Handle) Stop(ctx context.Context) error {
	if h.process == nil {
		return nil
	}
	return h.process.Stop(ctx)
}

// CallTool dispatches a tool call to the plugin.
// For persistent plugins, sends via the transport.
// For oneshot plugins, spawns a fresh process per call.
func (h *Handle) CallTool(ctx context.Context, toolName string, params json.RawMessage) (json.RawMessage, error) {
	if h.manifest.Concurrency <= 1 {
		h.mu.Lock()
		defer h.mu.Unlock()
	}

	// Auto-restart crashed persistent plugins
	if h.manifest.Execution == "persistent" && h.process != nil && h.process.State() == StateFailed {
		log.Printf("[%s] auto-restarting crashed plugin", h.manifest.Name)
		if err := h.Start(ctx); err != nil {
			return nil, &protocol.Error{
				Code:    "restart_failed",
				Message: fmt.Sprintf("failed to restart %s: %v", h.manifest.Name, err),
			}
		}
	}

	if h.manifest.Execution == "oneshot" {
		return h.callOneshot(ctx, toolName, params)
	}
	return h.callPersistent(ctx, toolName, params)
}

func (h *Handle) callPersistent(ctx context.Context, toolName string, params json.RawMessage) (json.RawMessage, error) {
	ctx, cancel := context.WithTimeout(ctx, h.toolTimeout)
	defer cancel()

	transport := h.process.Transport
	id := transport.GenerateID("req")

	ch := make(chan protocol.Message, 1)
	transport.pending.Store(id, ch)
	defer transport.pending.Delete(id)

	if err := transport.Send(protocol.Message{
		ID:     id,
		Type:   protocol.TypeToolCall,
		Tool:   toolName,
		Params: params,
		Config: h.manifest.resolvedConfig,
	}); err != nil {
		return nil, &protocol.Error{Code: "send_failed", Message: err.Error()}
	}

	select {
	case resp, ok := <-ch:
		if !ok {
			return nil, &protocol.Error{
				Code:    "plugin_exited",
				Message: fmt.Sprintf("plugin exited while handling %s", toolName),
			}
		}
		if resp.Error != nil {
			return nil, resp.Error
		}
		return resp.Result, nil
	case <-ctx.Done():
		return nil, &protocol.Error{
			Code:    "timeout",
			Message: fmt.Sprintf("tool call %s timed out after %s", toolName, h.toolTimeout),
		}
	}
}

func (h *Handle) callOneshot(ctx context.Context, toolName string, params json.RawMessage) (json.RawMessage, error) {
	ctx, cancel := context.WithTimeout(ctx, h.toolTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, h.manifest.HandlerPath()) //nolint:gosec // handler path validated by Manifest.Validate()
	cmd.Dir = h.manifest.Dir
	cmd.Env = buildPluginEnv(h.manifest)

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("stdin pipe: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("stdout pipe: %w", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, fmt.Errorf("stderr pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start oneshot handler: %w", err)
	}
	defer func() {
		stdin.Close() //nolint:errcheck,gosec // best effort
		cmd.Wait()    //nolint:errcheck,gosec // reap child
	}()

	go forwardStderr(stderr, h.manifest.Name)

	// Send tool_call
	id := fmt.Sprintf("oneshot-%d", time.Now().UnixNano())
	enc := json.NewEncoder(stdin)
	if err := enc.Encode(protocol.Message{
		ID:     id,
		Type:   protocol.TypeToolCall,
		Tool:   toolName,
		Params: params,
		Config: h.manifest.resolvedConfig,
	}); err != nil {
		return nil, fmt.Errorf("send tool_call: %w", err)
	}

	// Read messages until we get tool_result
	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 0), h.processCfg.MaxMessageSize)

	for scanner.Scan() {
		var msg protocol.Message
		if err := json.Unmarshal(scanner.Bytes(), &msg); err != nil {
			log.Printf("[%s] malformed oneshot message: %v", h.manifest.Name, err)
			continue
		}

		switch msg.Type {
		case protocol.TypeHTTPRequest:
			resp := h.handler.HandleHTTP(h.manifest.Name, msg)
			if err := enc.Encode(resp); err != nil {
				return nil, fmt.Errorf("send http_response: %w", err)
			}
		case protocol.TypeCacheGet, protocol.TypeCacheSet, protocol.TypeCacheDel, protocol.TypeCacheList, protocol.TypeCacheFlush:
			resp := h.handler.HandleCache(h.manifest.Name, msg)
			if err := enc.Encode(resp); err != nil {
				return nil, fmt.Errorf("send cache response: %w", err)
			}
		case protocol.TypeToolResult:
			if msg.Error != nil {
				return nil, msg.Error
			}
			return msg.Result, nil
		default:
			log.Printf("[%s] unexpected oneshot message type: %q", h.manifest.Name, msg.Type)
		}
	}

	return nil, &protocol.Error{Code: "no_response", Message: "oneshot handler exited without tool_result"}
}

func forwardStderr(r interface{ Read([]byte) (int, error) }, pluginName string) {
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		log.Printf("[%s] %s", pluginName, scanner.Text())
	}
}
