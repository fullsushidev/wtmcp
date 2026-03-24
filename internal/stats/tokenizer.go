// Package stats provides tool usage tracking and token estimation
// for wtmcp. It records per-call metrics, schema overhead, and
// resource reads to help estimate LLM context consumption.
package stats

import (
	"unicode/utf8"
)

// Tokenizer estimates token counts from text.
type Tokenizer interface {
	Count(text string) int
	CountBytes(data []byte) int
	Name() string
}

// CharsTokenizer estimates tokens using Anthropic's heuristic:
// 1 token ≈ 3.5 characters (rune count, not byte count).
type CharsTokenizer struct{}

const charsPerToken = 3.5

// Count estimates tokens from a string using rune count / 3.5.
func (CharsTokenizer) Count(text string) int {
	n := utf8.RuneCountInString(text)
	if n == 0 {
		return 0
	}
	tokens := int(float64(n) / charsPerToken)
	if tokens < 1 {
		return 1
	}
	return tokens
}

// CountBytes estimates tokens from a byte slice using rune count / 3.5.
func (CharsTokenizer) CountBytes(data []byte) int {
	n := utf8.RuneCount(data)
	if n == 0 {
		return 0
	}
	tokens := int(float64(n) / charsPerToken)
	if tokens < 1 {
		return 1
	}
	return tokens
}

// Name returns "chars".
func (CharsTokenizer) Name() string { return "chars" }
