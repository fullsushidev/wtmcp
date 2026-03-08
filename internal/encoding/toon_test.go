package encoding

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestEncodeTOON(t *testing.T) {
	input := json.RawMessage(`{"users":[{"id":1,"name":"Alice"},{"id":2,"name":"Bob"}]}`)

	result, err := EncodeTOON(input)
	if err != nil {
		t.Fatalf("EncodeTOON: %v", err)
	}

	// TOON output should be shorter than JSON
	if len(result) >= len(input) {
		t.Errorf("TOON (%d bytes) should be shorter than JSON (%d bytes)", len(result), len(input))
	}

	// Should contain the data
	if !strings.Contains(result, "Alice") || !strings.Contains(result, "Bob") {
		t.Errorf("TOON output missing data: %s", result)
	}
}

func TestEncodeTOONSimpleValue(t *testing.T) {
	result, err := EncodeTOON(json.RawMessage(`"hello"`))
	if err != nil {
		t.Fatalf("EncodeTOON: %v", err)
	}
	if result == "" {
		t.Error("expected non-empty output")
	}
}

func TestFormatResultJSON(t *testing.T) {
	data := json.RawMessage(`{"key":"value"}`)
	result := FormatResult(data, "json", true)
	if result != `{"key":"value"}` {
		t.Errorf("JSON format should passthrough: %s", result)
	}
}

func TestFormatResultTOON(t *testing.T) {
	data := json.RawMessage(`{"items":[{"a":1},{"a":2}]}`)
	result := FormatResult(data, "toon", true)
	// Should not be raw JSON (TOON encoding applied)
	if result == string(data) {
		t.Error("TOON format should transform the output")
	}
}

func TestFormatResultTOONFallback(t *testing.T) {
	// Invalid JSON — TOON will fail
	data := json.RawMessage(`not valid json`)
	result := FormatResult(data, "toon", true)
	// With fallback=true, should return raw data
	if result != "not valid json" {
		t.Errorf("fallback should return raw data: %s", result)
	}
}
