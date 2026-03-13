package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExtractFileID(t *testing.T) {
	tests := []struct {
		name string
		url  string
		want string
	}{
		{
			name: "google doc URL",
			url:  "https://docs.google.com/document/d/1abc123xyz/edit",
			want: "1abc123xyz",
		},
		{
			name: "google sheet URL",
			url:  "https://docs.google.com/spreadsheets/d/1abc123xyz/edit#gid=0",
			want: "1abc123xyz",
		},
		{
			name: "google slides URL",
			url:  "https://docs.google.com/presentation/d/1abc123xyz/edit",
			want: "1abc123xyz",
		},
		{
			name: "drive file URL",
			url:  "https://drive.google.com/file/d/1abc123xyz/view",
			want: "1abc123xyz",
		},
		{
			name: "drive open URL with id param",
			url:  "https://drive.google.com/open?id=1abc123xyz",
			want: "1abc123xyz",
		},
		{
			name: "file ID with hyphens and underscores",
			url:  "https://docs.google.com/document/d/1a-b_c123XYZ/edit",
			want: "1a-b_c123XYZ",
		},
		{
			name: "not a google URL",
			url:  "https://example.com/page",
			want: "",
		},
		{
			name: "empty string",
			url:  "",
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractFileID(tt.url)
			if got != tt.want {
				t.Errorf("extractFileID(%q) = %q, want %q", tt.url, got, tt.want)
			}
		})
	}
}

func TestCleanGoogleDocsCSS(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "no CSS",
			input: "# Hello\n\nSome text",
			want:  "# Hello\n\nSome text",
		},
		{
			name:  "strips @import lines",
			input: "# Title\n@import url('https://fonts.googleapis.com');\nReal content",
			want:  "# Title\nReal content",
		},
		{
			name:  "strips list-style-type blocks",
			input: "# Title\n.lst-kix_abc { list-style-type: disc; }\nReal content",
			want:  "# Title\nReal content",
		},
		{
			name:  "empty input",
			input: "",
			want:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := cleanGoogleDocsCSS(tt.input)
			if got != tt.want {
				t.Errorf("cleanGoogleDocsCSS() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestBuildSearchQuery(t *testing.T) {
	tests := []struct {
		name           string
		text           string
		inNameOnly     bool
		mimeTypes      []string
		owners         []string
		includeTrashed bool
		wantContains   []string
		wantNotContain []string
	}{
		{
			name:         "text only full-text",
			text:         "quarterly report",
			wantContains: []string{"fullText contains 'quarterly report'", "name contains 'quarterly report'", "trashed = false"},
		},
		{
			name:           "name only",
			text:           "budget",
			inNameOnly:     true,
			wantContains:   []string{"name contains 'budget'"},
			wantNotContain: []string{"fullText"},
		},
		{
			name:         "single mime type",
			text:         "doc",
			mimeTypes:    []string{"application/vnd.google-apps.document"},
			wantContains: []string{"mimeType = 'application/vnd.google-apps.document'"},
		},
		{
			name:         "multiple mime types ORed",
			text:         "doc",
			mimeTypes:    []string{"application/vnd.google-apps.document", "application/pdf"},
			wantContains: []string{"(mimeType = 'application/vnd.google-apps.document' or mimeType = 'application/pdf')"},
		},
		{
			name:         "single owner",
			text:         "design",
			owners:       []string{"me"},
			wantContains: []string{"'me' in owners"},
		},
		{
			name:         "multiple owners ORed",
			text:         "design",
			owners:       []string{"alice@example.com", "bob@example.com"},
			wantContains: []string{"('alice@example.com' in owners or 'bob@example.com' in owners)"},
		},
		{
			name:           "include trashed",
			text:           "old",
			includeTrashed: true,
			wantNotContain: []string{"trashed"},
		},
		{
			name:         "combined filters",
			text:         "meeting",
			mimeTypes:    []string{"application/vnd.google-apps.document"},
			owners:       []string{"me"},
			wantContains: []string{"fullText contains", "mimeType =", "'me' in owners", "trashed = false"},
		},
		{
			name:         "escapes quotes in text",
			text:         "it's a test",
			wantContains: []string{"it\\'s a test"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildSearchQuery(tt.text, tt.inNameOnly, tt.mimeTypes, tt.owners, tt.includeTrashed)
			for _, want := range tt.wantContains {
				if !strings.Contains(got, want) {
					t.Errorf("query %q should contain %q", got, want)
				}
			}
			for _, notWant := range tt.wantNotContain {
				if strings.Contains(got, notWant) {
					t.Errorf("query %q should not contain %q", got, notWant)
				}
			}
		})
	}
}

func TestSaveExportFile(t *testing.T) {
	dir := t.TempDir()

	t.Run("saves with explicit path", func(t *testing.T) {
		outPath := filepath.Join(dir, "test.md")
		got, err := saveExportFile("", outPath, "hello world")
		if err != nil {
			t.Fatalf("saveExportFile: %v", err)
		}
		if got != outPath {
			t.Errorf("path = %q, want %q", got, outPath)
		}

		data, err := os.ReadFile(outPath) //nolint:gosec // test file path
		if err != nil {
			t.Fatalf("read: %v", err)
		}
		if string(data) != "hello world" {
			t.Errorf("content = %q, want %q", string(data), "hello world")
		}
	})

	t.Run("creates parent directories", func(t *testing.T) {
		outPath := filepath.Join(dir, "sub", "dir", "test.md")
		_, err := saveExportFile("", outPath, "nested")
		if err != nil {
			t.Fatalf("saveExportFile: %v", err)
		}

		data, err := os.ReadFile(outPath) //nolint:gosec // test file path
		if err != nil {
			t.Fatalf("read: %v", err)
		}
		if string(data) != "nested" {
			t.Errorf("content = %q, want %q", string(data), "nested")
		}
	})

	t.Run("file permissions are 0600", func(t *testing.T) {
		outPath := filepath.Join(dir, "perms.md")
		_, err := saveExportFile("", outPath, "secret")
		if err != nil {
			t.Fatalf("saveExportFile: %v", err)
		}

		info, err := os.Stat(outPath)
		if err != nil {
			t.Fatalf("stat: %v", err)
		}
		perm := info.Mode().Perm()
		if perm != 0o600 {
			t.Errorf("permissions = %o, want 0600", perm)
		}
	})
}
