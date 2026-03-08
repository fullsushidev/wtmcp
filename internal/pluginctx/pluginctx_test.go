package pluginctx

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadFile(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "context.md"), []byte("# Test Context\nSome rules."), 0o600); err != nil {
		t.Fatal(err)
	}

	content, err := LoadFile(dir, "context.md")
	if err != nil {
		t.Fatalf("LoadFile: %v", err)
	}
	if content != "# Test Context\nSome rules." {
		t.Errorf("content = %q", content)
	}
}

func TestLoadFileMissing(t *testing.T) {
	_, err := LoadFile(t.TempDir(), "missing.md")
	if err == nil {
		t.Error("expected error for missing file")
	}
}

func TestLoadFileEscapesDir(t *testing.T) {
	_, err := LoadFile(t.TempDir(), "../../etc/passwd")
	if err == nil {
		t.Error("expected error for path traversal")
	}
}

func TestResourceURI(t *testing.T) {
	uri := ResourceURI("calendar", "context.md")
	expected := "wtmcp://plugin/calendar/context/context.md"
	if uri != expected {
		t.Errorf("URI = %q, want %q", uri, expected)
	}
}
