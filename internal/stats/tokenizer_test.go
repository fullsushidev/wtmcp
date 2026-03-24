package stats

import (
	"strings"
	"testing"
)

func TestCharsTokenizer_Count(t *testing.T) {
	tok := CharsTokenizer{}

	tests := []struct {
		name  string
		input string
		want  int
	}{
		{"empty", "", 0},
		{"single char", "a", 1},
		{"short word", "hello", 1},
		{"sentence", "hello world foo bar", 5},
		{"json blob", `{"key":"value","num":42}`, 6},
		{"multibyte utf8", "日本語テスト", 1},
		{"mixed ascii and utf8", "hello 日本語", 2},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tok.Count(tt.input)
			if got != tt.want {
				t.Errorf("Count(%q) = %d, want %d", tt.input, got, tt.want)
			}
		})
	}
}

func TestCharsTokenizer_CountBytes(t *testing.T) {
	tok := CharsTokenizer{}

	tests := []struct {
		name  string
		input []byte
		want  int
	}{
		{"empty", nil, 0},
		{"empty slice", []byte{}, 0},
		{"single char", []byte("a"), 1},
		{"sentence", []byte("hello world foo bar"), 5},
		{"multibyte utf8", []byte("日本語テスト"), 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tok.CountBytes(tt.input)
			if got != tt.want {
				t.Errorf("CountBytes(%q) = %d, want %d", tt.input, got, tt.want)
			}
		})
	}
}

func TestCharsTokenizer_CountAndCountBytes_Consistent(t *testing.T) {
	tok := CharsTokenizer{}

	inputs := []string{
		"",
		"hello",
		"日本語テスト",
		`{"issues":[{"key":"PROJ-123","summary":"test"}]}`,
		strings.Repeat("a", 1000),
	}

	for _, s := range inputs {
		countStr := tok.Count(s)
		countBytes := tok.CountBytes([]byte(s))
		if countStr != countBytes {
			t.Errorf("Count(%q)=%d != CountBytes=%d", s, countStr, countBytes)
		}
	}
}

func TestCharsTokenizer_Name(t *testing.T) {
	tok := CharsTokenizer{}
	if tok.Name() != "chars" {
		t.Errorf("Name() = %q, want %q", tok.Name(), "chars")
	}
}
