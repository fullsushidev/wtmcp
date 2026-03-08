// Package encoding provides output format encoding for tool results.
// Supports JSON (passthrough) and TOON (~30-40% token savings).
package encoding

import (
	"encoding/json"
	"fmt"

	"github.com/alpkeskin/gotoon"
)

// EncodeTOON converts a JSON value to TOON format.
func EncodeTOON(data json.RawMessage) (string, error) {
	var v any
	if err := json.Unmarshal(data, &v); err != nil {
		return "", fmt.Errorf("toon unmarshal: %w", err)
	}
	return gotoon.Encode(v)
}

// FormatResult applies the configured output format to a tool result.
// Returns the formatted string ready for MCP TextContent.
func FormatResult(data json.RawMessage, format string, fallback bool) string {
	if format == "toon" {
		encoded, err := EncodeTOON(data)
		if err == nil {
			return encoded
		}
		if !fallback {
			return fmt.Sprintf("[toon encoding failed: %v]\n%s", err, string(data))
		}
		// Fall through to JSON
	}
	return string(data)
}
