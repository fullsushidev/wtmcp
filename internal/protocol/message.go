// Package protocol defines the wire protocol message types for
// bidirectional JSON-lines communication between core and plugins.
package protocol

import (
	"encoding/json"
	"fmt"
)

// Message is the wire format for all communication between the core
// and plugin processes. Fields are selectively populated based on
// the message Type.
type Message struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Protocol string `json:"protocol,omitempty"`

	// tool_call fields
	Tool   string          `json:"tool,omitempty"`
	Params json.RawMessage `json:"params,omitempty"`
	Config json.RawMessage `json:"config,omitempty"`

	// tool_result fields
	Result json.RawMessage `json:"result,omitempty"`
	Error  *Error          `json:"error,omitempty"`

	// http_request / http_response fields
	Method       string            `json:"method,omitempty"`
	Path         string            `json:"path,omitempty"`
	URL          string            `json:"url,omitempty"`
	Query        map[string]any    `json:"query,omitempty"`
	Headers      map[string]string `json:"headers,omitempty"`
	Body         json.RawMessage   `json:"body,omitempty"`
	BodyEncoding string            `json:"body_encoding,omitempty"`
	Multipart    []MultipartPart   `json:"multipart,omitempty"`
	Status       int               `json:"status,omitempty"`

	// cache fields
	Key     string          `json:"key,omitempty"`
	Value   json.RawMessage `json:"value,omitempty"`
	TTL     *int            `json:"ttl,omitempty"`
	Hit     *bool           `json:"hit,omitempty"`
	OK      *bool           `json:"ok,omitempty"`
	Deleted *bool           `json:"deleted,omitempty"`
	Keys    []string        `json:"keys,omitempty"`
	Pattern string          `json:"pattern,omitempty"`
	Count   *int            `json:"count,omitempty"`

	// auth_request / auth_response fields
	AuthConfig json.RawMessage `json:"auth_config,omitempty"`
	Target     *AuthTarget     `json:"target,omitempty"`
}

// AuthTarget describes the HTTP request that needs authentication.
type AuthTarget struct {
	Method string `json:"method"`
	URL    string `json:"url"`
}

// MultipartPart describes one part of a multipart/form-data request.
// If Filename is set, the part is a file upload; otherwise it's a text field.
type MultipartPart struct {
	Field        string `json:"field"`
	Filename     string `json:"filename,omitempty"`
	ContentType  string `json:"content_type,omitempty"`
	Body         string `json:"body"`
	BodyEncoding string `json:"body_encoding,omitempty"`
}

// Error is a structured error returned by plugins.
// It preserves the error code through the entire chain from plugin
// to MCP client.
type Error struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// Error implements the error interface.
func (e *Error) Error() string {
	return fmt.Sprintf("[%s] %s", e.Code, e.Message)
}

// Message type constants.
const (
	TypeToolCall     = "tool_call"
	TypeToolResult   = "tool_result"
	TypeInit         = "init"
	TypeInitOK       = "init_ok"
	TypeInitError    = "init_error"
	TypeShutdown     = "shutdown"
	TypeShutdownOK   = "shutdown_ok"
	TypeHTTPRequest  = "http_request"
	TypeHTTPResponse = "http_response"
	TypeCacheGet     = "cache_get"
	TypeCacheSet     = "cache_set"
	TypeCacheDel     = "cache_del"
	TypeCacheList    = "cache_list"
	TypeCacheFlush   = "cache_flush"
	TypeAuthRequest  = "auth_request"
	TypeAuthResponse = "auth_response"
)

// ProtocolVersion is the current wire protocol version sent in init.
const ProtocolVersion = "1.0"
