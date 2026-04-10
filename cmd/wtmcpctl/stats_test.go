package main

import (
	"math"
	"testing"
)

func TestFmtInt(t *testing.T) {
	tests := []struct {
		input int
		want  string
	}{
		{0, "0"},
		{1, "1"},
		{12, "12"},
		{999, "999"},
		{1000, "1,000"},
		{1234, "1,234"},
		{12345, "12,345"},
		{123456, "123,456"},
		{1234567, "1,234,567"},
		{1000000, "1,000,000"},
		{-1, "-1"},
		{-1000, "-1,000"},
		{-1234567, "-1,234,567"},
		{math.MaxInt32, "2,147,483,647"},
	}

	for _, tt := range tests {
		got := fmtInt(tt.input)
		if got != tt.want {
			t.Errorf("fmtInt(%d) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestFmtInt64(t *testing.T) {
	tests := []struct {
		input int64
		want  string
	}{
		{0, "0"},
		{999, "999"},
		{1000, "1,000"},
		{1234567890, "1,234,567,890"},
		{-1000, "-1,000"},
		{math.MaxInt32, "2,147,483,647"},
	}

	for _, tt := range tests {
		got := fmtInt64(tt.input)
		if got != tt.want {
			t.Errorf("fmtInt64(%d) = %q, want %q", tt.input, got, tt.want)
		}
	}
}
