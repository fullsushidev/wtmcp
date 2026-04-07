package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExtractDocumentID(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"full URL with edit", "https://docs.google.com/document/d/abc123_-XY/edit", "abc123_-XY"},
		{"full URL without edit", "https://docs.google.com/document/d/abc123/", "abc123"},
		{"query parameter", "https://example.com/view?id=doc456", "doc456"},
		{"query parameter with ampersand", "https://example.com/view?foo=bar&id=doc789", "doc789"},
		{"raw document ID", "abc123_-XY", "abc123_-XY"},
		{"empty string", "", ""},
		{"invalid characters", "not a valid id!", ""},
		{"URL without doc pattern", "https://example.com/other/path", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractDocumentID(tt.input)
			if got != tt.want {
				t.Errorf("extractDocumentID(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestIndentDepth(t *testing.T) {
	tests := []struct {
		name   string
		indent string
		want   int
	}{
		{"no indent", "", 0},
		{"one tab", "\t", 1},
		{"two tabs", "\t\t", 2},
		{"four spaces", "    ", 1},
		{"eight spaces", "        ", 2},
		{"mixed tab and spaces", "\t    ", 2},
		{"three spaces (partial)", "   ", 0},
		{"five spaces", "     ", 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := indentDepth(tt.indent)
			if got != tt.want {
				t.Errorf("indentDepth(%q) = %d, want %d", tt.indent, got, tt.want)
			}
		})
	}
}

func TestParseMarkdownHeadings(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		wantHeading int
		wantText    string
	}{
		{"h1", "# Title", 1, "Title\n"},
		{"h2", "## Subtitle", 2, "Subtitle\n"},
		{"h3", "### Section", 3, "Section\n"},
		{"h6", "###### Deep", 6, "Deep\n"},
		{"not heading (no space)", "#NoSpace", 0, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			segments := parseMarkdown(tt.input)
			if tt.wantHeading == 0 {
				// Should not produce a heading segment
				for _, seg := range segments {
					if seg.heading > 0 {
						t.Errorf("unexpected heading segment: %+v", seg)
					}
				}
				return
			}
			found := false
			for _, seg := range segments {
				if seg.heading == tt.wantHeading {
					found = true
					if seg.text != tt.wantText {
						t.Errorf("heading text = %q, want %q", seg.text, tt.wantText)
					}
				}
			}
			if !found {
				t.Errorf("no segment with heading=%d found in %+v", tt.wantHeading, segments)
			}
		})
	}
}

func TestParseMarkdownLists(t *testing.T) {
	t.Run("ordered list", func(t *testing.T) {
		segments := parseMarkdown("1. First\n2. Second")
		hasOrdered := false
		for _, seg := range segments {
			if seg.orderedListItem {
				hasOrdered = true
			}
		}
		if !hasOrdered {
			t.Error("expected ordered list items")
		}
	})

	t.Run("unordered list", func(t *testing.T) {
		segments := parseMarkdown("- First\n- Second")
		hasUnordered := false
		for _, seg := range segments {
			if seg.unorderedListItem {
				hasUnordered = true
			}
		}
		if !hasUnordered {
			t.Error("expected unordered list items")
		}
	})

	t.Run("nested list depth", func(t *testing.T) {
		segments := parseMarkdown("- Top\n    - Nested")
		maxDepth := 0
		for _, seg := range segments {
			if seg.listDepth > maxDepth {
				maxDepth = seg.listDepth
			}
		}
		if maxDepth != 1 {
			t.Errorf("max depth = %d, want 1", maxDepth)
		}
	})
}

func TestParseSimpleFormatting(t *testing.T) {
	t.Run("bold", func(t *testing.T) {
		segments := parseSimpleFormatting("**bold**")
		if len(segments) != 1 || !segments[0].bold || segments[0].text != "bold" {
			t.Errorf("got %+v, want bold segment with text 'bold'", segments)
		}
	})

	t.Run("italic with asterisk", func(t *testing.T) {
		segments := parseSimpleFormatting("*italic*")
		if len(segments) != 1 || !segments[0].italic || segments[0].text != "italic" {
			t.Errorf("got %+v, want italic segment", segments)
		}
	})

	t.Run("italic with underscore", func(t *testing.T) {
		segments := parseSimpleFormatting("_italic_")
		if len(segments) != 1 || !segments[0].italic || segments[0].text != "italic" {
			t.Errorf("got %+v, want italic segment", segments)
		}
	})

	t.Run("underline", func(t *testing.T) {
		segments := parseSimpleFormatting("__underlined__")
		if len(segments) != 1 || !segments[0].underline || segments[0].text != "underlined" {
			t.Errorf("got %+v, want underline segment", segments)
		}
	})

	t.Run("plain text", func(t *testing.T) {
		segments := parseSimpleFormatting("hello world")
		merged := mergeSegments(segments)
		if len(merged) != 1 || merged[0].text != "hello world" {
			t.Errorf("got %+v, want single 'hello world' segment", merged)
		}
	})

	t.Run("unclosed bold", func(t *testing.T) {
		segments := parseSimpleFormatting("**unclosed")
		merged := mergeSegments(segments)
		if len(merged) != 1 || merged[0].text != "**unclosed" {
			t.Errorf("got %+v, want literal '**unclosed'", merged)
		}
	})
}

func TestParseInlineFormatting(t *testing.T) {
	t.Run("link", func(t *testing.T) {
		segments := parseInlineFormatting("[Google](https://google.com)")
		found := false
		for _, seg := range segments {
			if seg.linkURL == "https://google.com" && seg.text == "Google" {
				found = true
			}
		}
		if !found {
			t.Errorf("link not found in %+v", segments)
		}
	})

	t.Run("date today", func(t *testing.T) {
		segments := parseInlineFormatting("@today")
		found := false
		for _, seg := range segments {
			if seg.isDateField && seg.dateValue == "" {
				found = true
			}
		}
		if !found {
			t.Errorf("@today not found in %+v", segments)
		}
	})

	t.Run("date specific", func(t *testing.T) {
		segments := parseInlineFormatting("@date(2026-01-15)")
		found := false
		for _, seg := range segments {
			if seg.isDateField && seg.dateValue == "2026-01-15" {
				found = true
			}
		}
		if !found {
			t.Errorf("@date(2026-01-15) not found in %+v", segments)
		}
	})

	t.Run("person chip", func(t *testing.T) {
		segments := parseInlineFormatting("@(user@example.com)")
		found := false
		for _, seg := range segments {
			if seg.isPersonField && seg.personIdentifier == "user@example.com" {
				found = true
			}
		}
		if !found {
			t.Errorf("person chip not found in %+v", segments)
		}
	})
}

func TestMergeSegments(t *testing.T) {
	t.Run("merge adjacent plain", func(t *testing.T) {
		segments := []markdownSegment{
			{text: "a"},
			{text: "b"},
			{text: "c"},
		}
		merged := mergeSegments(segments)
		if len(merged) != 1 || merged[0].text != "abc" {
			t.Errorf("got %+v, want single 'abc' segment", merged)
		}
	})

	t.Run("no merge different formatting", func(t *testing.T) {
		segments := []markdownSegment{
			{text: "plain"},
			{text: "bold", bold: true},
			{text: "plain2"},
		}
		merged := mergeSegments(segments)
		if len(merged) != 3 {
			t.Errorf("got %d segments, want 3", len(merged))
		}
	})

	t.Run("no merge date fields", func(t *testing.T) {
		segments := []markdownSegment{
			{text: "before"},
			{text: " ", isDateField: true, dateValue: "2026-01-01"},
			{text: "after"},
		}
		merged := mergeSegments(segments)
		if len(merged) != 3 {
			t.Errorf("got %d segments, want 3 (date fields should not merge)", len(merged))
		}
	})

	t.Run("empty input", func(t *testing.T) {
		merged := mergeSegments(nil)
		if len(merged) != 0 {
			t.Errorf("got %+v, want empty", merged)
		}
	})
}

func TestSaveDocumentFile(t *testing.T) {
	origDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	tmpDir := t.TempDir()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(origDir) })

	t.Run("saves with title-derived path", func(t *testing.T) {
		got, err := saveDocumentFile("My Document", "", "content", ".txt")
		if err != nil {
			t.Fatalf("saveDocumentFile: %v", err)
		}
		wantAbs := filepath.Join(tmpDir, "docs", "My Document.txt")
		if got != wantAbs {
			t.Errorf("path = %q, want %q", got, wantAbs)
		}
	})

	t.Run("saves with explicit path inside base", func(t *testing.T) {
		outPath := filepath.Join("docs", "custom.txt")
		got, err := saveDocumentFile("", outPath, "content", ".txt")
		if err != nil {
			t.Fatalf("saveDocumentFile: %v", err)
		}
		wantAbs := filepath.Join(tmpDir, "docs", "custom.txt")
		if got != wantAbs {
			t.Errorf("path = %q, want %q", got, wantAbs)
		}
	})

	t.Run("rejects path traversal", func(t *testing.T) {
		_, err := saveDocumentFile("", "../../etc/evil.txt", "pwned", ".txt")
		if err == nil {
			t.Fatal("expected error for path traversal, got nil")
		}
	})

	t.Run("sanitizes title with traversal characters", func(t *testing.T) {
		got, err := saveDocumentFile("../../etc/evil", "", "content", ".txt")
		if err != nil {
			t.Fatalf("saveDocumentFile: %v", err)
		}
		if !strings.HasPrefix(got, filepath.Join(tmpDir, "docs")+string(os.PathSeparator)) {
			t.Errorf("path %q escapes docs directory", got)
		}
	})

	t.Run("file permissions are 0600", func(t *testing.T) {
		outPath := filepath.Join("docs", "perms.txt")
		got, err := saveDocumentFile("", outPath, "secret", ".txt")
		if err != nil {
			t.Fatalf("saveDocumentFile: %v", err)
		}
		info, err := os.Stat(got)
		if err != nil {
			t.Fatalf("stat: %v", err)
		}
		if perm := info.Mode().Perm(); perm != 0o600 {
			t.Errorf("permissions = %o, want 0600", perm)
		}
	})
}
