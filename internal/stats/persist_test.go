package stats

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestPersist_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "stats.json")

	c := NewCollector(CharsTokenizer{}, false)
	if err := c.SetPersistPath(path); err != nil {
		t.Fatal(err)
	}

	// Record some data.
	c.Record("get_issues", "jira", time.Now(), []byte(`{"project":"TEST"}`), `{"issues":[]}`, false)
	c.Record("get_issues", "jira", time.Now(), []byte(`{}`), "error", true)
	c.RecordSchema("get_issues", "jira", "Get issues", []byte(`{"type":"object"}`))
	c.RecordResourceRead("uri://ctx", "jira", "context", "# Context")

	// Force save.
	c.Close()

	// Verify file exists.
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("stats.json not created: %v", err)
	}

	// Load into a new collector.
	c2 := NewCollector(CharsTokenizer{}, false)
	if err := c2.SetPersistPath(path); err != nil {
		t.Fatal(err)
	}

	// Verify aggregates restored.
	summary := c2.Summary()
	if len(summary) != 1 {
		t.Fatalf("expected 1 tool summary after load, got %d", len(summary))
	}
	if summary[0].CallCount != 2 {
		t.Errorf("CallCount = %d, want 2", summary[0].CallCount)
	}
	if summary[0].ErrorCount != 1 {
		t.Errorf("ErrorCount = %d, want 1", summary[0].ErrorCount)
	}

	// Verify schemas restored.
	cost := c2.SchemaCost()
	if cost.TotalTools != 1 {
		t.Errorf("TotalTools = %d, want 1", cost.TotalTools)
	}

	// Verify resources restored.
	resources := c2.ResourceSummary()
	if len(resources) != 1 {
		t.Errorf("expected 1 resource after load, got %d", len(resources))
	}
	if resources[0].ReadCount != 1 {
		t.Errorf("ReadCount = %d, want 1", resources[0].ReadCount)
	}

	// Verify StartedAt restored.
	started := c2.StartedAt()
	if started == nil {
		t.Error("StartedAt should be set after save/load")
	}
}

func TestPersist_NoFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nonexistent.json")

	c := NewCollector(CharsTokenizer{}, false)
	if err := c.SetPersistPath(path); err != nil {
		t.Fatal(err)
	}

	// Should have no data — load of nonexistent file is not an error.
	if len(c.Summary()) != 0 {
		t.Error("expected empty summary when no file exists")
	}
}

func TestPersist_StartedAt_BackwardCompat(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "stats.json")

	// Write an old-format snapshot without started_at.
	oldJSON := `{"tokenizer":"chars","aggregates":{"tool":{"tool_name":"tool","plugin_name":"p","call_count":1}},"schemas":{},"resources":{}}`
	if err := os.WriteFile(path, []byte(oldJSON), 0o600); err != nil {
		t.Fatal(err)
	}

	c := NewCollector(CharsTokenizer{}, false)
	if err := c.SetPersistPath(path); err != nil {
		t.Fatal(err)
	}

	// StartedAt should be nil when loading an old snapshot.
	if c.StartedAt() != nil {
		t.Error("StartedAt should be nil for old snapshots without the field")
	}

	// After a save, StartedAt should be set.
	c.Close()

	c2 := NewCollector(CharsTokenizer{}, false)
	if err := c2.SetPersistPath(path); err != nil {
		t.Fatal(err)
	}
	if c2.StartedAt() == nil {
		t.Error("StartedAt should be set after save")
	}
}

func TestPersist_CreatesDirectory(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "nested", "dir")
	path := filepath.Join(dir, "stats.json")

	c := NewCollector(CharsTokenizer{}, false)
	if err := c.SetPersistPath(path); err != nil {
		t.Fatal(err)
	}

	info, err := os.Stat(dir)
	if err != nil {
		t.Fatalf("directory not created: %v", err)
	}
	if info.Mode().Perm() != 0o700 {
		t.Errorf("directory permissions = %o, want 700", info.Mode().Perm())
	}
}

func TestPersist_AtomicWrite(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "stats.json")

	c := NewCollector(CharsTokenizer{}, false)
	if err := c.SetPersistPath(path); err != nil {
		t.Fatal(err)
	}

	c.Record("tool", "plugin", time.Now(), nil, "output", false)
	c.Close()

	// Check file permissions.
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Errorf("file permissions = %o, want 600", info.Mode().Perm())
	}

	// Ensure no temp files left behind.
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if e.Name() != "stats.json" {
			t.Errorf("unexpected file left behind: %s", e.Name())
		}
	}
}

func TestPersist_CloseIsIdempotent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "stats.json")

	c := NewCollector(CharsTokenizer{}, false)
	if err := c.SetPersistPath(path); err != nil {
		t.Fatal(err)
	}

	c.Record("tool", "plugin", time.Now(), nil, "ok", false)
	c.Close()
	c.Close() // Should not panic.
}

func TestPersist_RecordAfterClose(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "stats.json")

	c := NewCollector(CharsTokenizer{}, false)
	if err := c.SetPersistPath(path); err != nil {
		t.Fatal(err)
	}

	c.Close()
	c.Record("tool", "plugin", time.Now(), nil, "ok", false)

	// Should have no data — Record is no-op after Close.
	if len(c.Summary()) != 0 {
		t.Error("Record should be no-op after Close")
	}
}

func TestPersist_NoPersistPath(_ *testing.T) {
	c := NewCollector(CharsTokenizer{}, false)

	c.Record("tool", "plugin", time.Now(), nil, "ok", false)
	c.Close() // Should not panic without persist path.
}

func TestPersist_TotalDurationMs_NoDrift(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "stats.json")

	// Record calls with a known total duration to detect rounding drift.
	c := NewCollector(CharsTokenizer{}, false)
	if err := c.SetPersistPath(path); err != nil {
		t.Fatal(err)
	}

	// 3 calls with durations that produce a non-zero remainder when
	// divided by 3 (total=7ms from integer truncation perspective).
	c.Record("tool", "plugin", time.Now(), nil, "ok", false)
	c.Record("tool", "plugin", time.Now(), nil, "ok", false)
	c.Record("tool", "plugin", time.Now(), nil, "ok", false)
	c.Close()

	// Get the initial TotalDurationMs.
	c2 := NewCollector(CharsTokenizer{}, false)
	if err := c2.SetPersistPath(path); err != nil {
		t.Fatal(err)
	}
	s1 := c2.Summary()
	initialDuration := s1[0].TotalDurationMs

	// Simulate multiple save/load cycles with no new calls.
	// TotalDurationMs should not drift.
	for i := range 5 {
		c2.Close()
		c3 := NewCollector(CharsTokenizer{}, false)
		if err := c3.SetPersistPath(path); err != nil {
			t.Fatal(err)
		}
		s := c3.Summary()
		if s[0].TotalDurationMs != initialDuration {
			t.Errorf("cycle %d: TotalDurationMs = %d, want %d (drift detected)",
				i, s[0].TotalDurationMs, initialDuration)
		}
		c2 = c3
	}
}

func TestPersist_Accumulates(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "stats.json")

	// Session 1: 3 calls.
	c1 := NewCollector(CharsTokenizer{}, false)
	if err := c1.SetPersistPath(path); err != nil {
		t.Fatal(err)
	}
	for range 3 {
		c1.Record("tool", "plugin", time.Now(), nil, "ok", false)
	}
	c1.Close()

	// Session 2: 2 more calls.
	c2 := NewCollector(CharsTokenizer{}, false)
	if err := c2.SetPersistPath(path); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		c2.Record("tool", "plugin", time.Now(), nil, "ok", false)
	}
	c2.Close()

	// Session 3: verify accumulated.
	c3 := NewCollector(CharsTokenizer{}, false)
	if err := c3.SetPersistPath(path); err != nil {
		t.Fatal(err)
	}

	summary := c3.Summary()
	if len(summary) != 1 {
		t.Fatalf("expected 1 summary, got %d", len(summary))
	}
	if summary[0].CallCount != 5 {
		t.Errorf("CallCount = %d, want 5 (accumulated across sessions)", summary[0].CallCount)
	}
}
