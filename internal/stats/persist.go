package stats

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"
)

const debounceInterval = 5 * time.Second

// Snapshot is the persisted format — aggregates only, no raw entries.
type Snapshot struct {
	Tokenizer  string                   `json:"tokenizer"`
	Aggregates map[string]ToolSummary   `json:"aggregates"`
	Schemas    map[string]SchemaEntry   `json:"schemas"`
	Resources  map[string]ResourceEntry `json:"resources"`
}

// SetPersistPath enables persistence to the given file path.
// If the file exists, loads previous aggregates. Creates the
// parent directory with 0o700 if needed.
func (c *Collector) SetPersistPath(path string) error {
	if path == "" {
		return nil
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create stats dir: %w", err)
	}

	// Harden permissions on existing directory.
	if err := os.Chmod(dir, 0o700); err != nil { //nolint:gosec // directory needs 0700 for owner-only access
		log.Printf("[stats] warning: cannot set permissions on %s: %v", dir, err)
	}

	c.mu.Lock()
	c.persistPath = path
	c.mu.Unlock()

	// Load existing snapshot if available.
	if err := c.load(); err != nil {
		log.Printf("[stats] warning: failed to load %s: %v", path, err)
	}

	return nil
}

// Close stops the debounce timer, flushes persistence, and marks
// the collector as closed. Record() becomes a no-op after Close().
func (c *Collector) Close() {
	c.closed.Store(true)

	c.mu.Lock()
	defer c.mu.Unlock()

	if c.persistTmr != nil {
		if !c.persistTmr.Stop() {
			select {
			case <-c.persistTmr.C:
			default:
			}
		}
		c.persistTmr = nil
	}

	c.saveLocked()
}

// scheduleSave starts or resets the debounce timer. Must be called
// with c.mu held.
func (c *Collector) scheduleSave() {
	if c.persistPath == "" {
		return
	}

	if c.persistTmr != nil {
		c.persistTmr.Stop()
	}
	c.persistTmr = time.AfterFunc(debounceInterval, func() {
		c.mu.Lock()
		defer c.mu.Unlock()
		c.saveLocked()
	})
}

// saveLocked writes the current state to disk atomically.
// Must be called with c.mu held.
func (c *Collector) saveLocked() {
	if c.persistPath == "" {
		return
	}

	snap := Snapshot{
		Tokenizer:  c.tokenizer.Name(),
		Aggregates: make(map[string]ToolSummary, len(c.aggregates)),
		Schemas:    make(map[string]SchemaEntry, len(c.schemas)),
		Resources:  make(map[string]ResourceEntry, len(c.resources)),
	}

	for name, agg := range c.aggregates {
		snap.Aggregates[name] = agg.toSummary(name)
	}
	for name, s := range c.schemas {
		snap.Schemas[name] = s
	}
	for uri, r := range c.resources {
		snap.Resources[uri] = *r
	}

	if err := atomicWrite(c.persistPath, snap); err != nil {
		log.Printf("[stats] failed to save %s: %v", c.persistPath, err)
	}
}

// load reads a snapshot from disk and restores aggregates.
func (c *Collector) load() error {
	data, err := os.ReadFile(c.persistPath) //nolint:gosec // path from server config
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	var snap Snapshot
	if err := json.Unmarshal(data, &snap); err != nil {
		return fmt.Errorf("parse stats: %w", err)
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	// Restore aggregates from persisted summaries.
	for name, ts := range snap.Aggregates {
		c.aggregates[name] = &runningAggregate{
			PluginName:        ts.PluginName,
			CallCount:         ts.CallCount,
			ErrorCount:        ts.ErrorCount,
			TotalInputTokens:  ts.TotalInputTokens,
			TotalOutputTokens: ts.TotalOutputTokens,
			TotalDurationMs:   ts.AvgDurationMs * int64(ts.CallCount),
			MaxOutputTokens:   ts.MaxOutputTokens,
		}
	}

	for name, s := range snap.Schemas {
		c.schemas[name] = s
	}

	for uri, r := range snap.Resources {
		entry := r
		c.resources[uri] = &entry
	}

	return nil
}

// atomicWrite writes data to path using temp+fsync+rename.
func atomicWrite(path string, data any) error {
	dir := filepath.Dir(path)
	f, err := os.CreateTemp(dir, "stats-*.tmp")
	if err != nil {
		return fmt.Errorf("create temp: %w", err)
	}
	defer os.Remove(f.Name()) //nolint:errcheck // cleanup on failure; harmless after rename

	if err := f.Chmod(0o600); err != nil {
		f.Close() //nolint:errcheck,gosec // best effort
		return fmt.Errorf("chmod temp: %w", err)
	}

	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	if err := enc.Encode(data); err != nil {
		f.Close() //nolint:errcheck,gosec // best effort
		return fmt.Errorf("encode: %w", err)
	}

	if err := f.Sync(); err != nil {
		f.Close() //nolint:errcheck,gosec // best effort
		return fmt.Errorf("fsync: %w", err)
	}

	if err := f.Close(); err != nil {
		return fmt.Errorf("close: %w", err)
	}

	if err := os.Rename(f.Name(), path); err != nil {
		return fmt.Errorf("rename: %w", err)
	}

	return nil
}
