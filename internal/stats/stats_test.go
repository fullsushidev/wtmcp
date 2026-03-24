package stats

import (
	"testing"
	"time"
)

func TestCollector_Record(t *testing.T) {
	c := NewCollector(CharsTokenizer{}, false)

	c.Record("get_issues", "jira", time.Now(), []byte(`{"project":"TEST"}`), `{"issues":[]}`, false)

	summary := c.Summary()
	if len(summary) != 1 {
		t.Fatalf("expected 1 tool summary, got %d", len(summary))
	}
	s := summary[0]
	if s.ToolName != "get_issues" {
		t.Errorf("ToolName = %q, want %q", s.ToolName, "get_issues")
	}
	if s.PluginName != "jira" {
		t.Errorf("PluginName = %q, want %q", s.PluginName, "jira")
	}
	if s.CallCount != 1 {
		t.Errorf("CallCount = %d, want 1", s.CallCount)
	}
	if s.ErrorCount != 0 {
		t.Errorf("ErrorCount = %d, want 0", s.ErrorCount)
	}
	if s.TotalInputTokens == 0 {
		t.Error("TotalInputTokens should be > 0")
	}
	if s.TotalOutputTokens == 0 {
		t.Error("TotalOutputTokens should be > 0")
	}
}

func TestCollector_Record_Error(t *testing.T) {
	c := NewCollector(CharsTokenizer{}, false)

	c.Record("create_issue", "jira", time.Now(), []byte(`{}`), "permission denied", true)

	summary := c.Summary()
	if len(summary) != 1 {
		t.Fatalf("expected 1 summary, got %d", len(summary))
	}
	if summary[0].ErrorCount != 1 {
		t.Errorf("ErrorCount = %d, want 1", summary[0].ErrorCount)
	}
}

func TestCollector_Record_RingBuffer(t *testing.T) {
	c := NewCollector(CharsTokenizer{}, false)

	// Fill beyond maxEntries to test ring buffer wrap.
	for i := 0; i < maxEntries+100; i++ {
		c.Record("tool", "plugin", time.Now(), nil, "ok", false)
	}

	summary := c.Summary()
	if len(summary) != 1 {
		t.Fatalf("expected 1 summary, got %d", len(summary))
	}
	if summary[0].CallCount != maxEntries+100 {
		t.Errorf("CallCount = %d, want %d", summary[0].CallCount, maxEntries+100)
	}
}

func TestCollector_Record_ClosedIsNoop(t *testing.T) {
	c := NewCollector(CharsTokenizer{}, false)

	c.closed.Store(true)
	c.Record("tool", "plugin", time.Now(), nil, "ok", false)

	if len(c.Summary()) != 0 {
		t.Error("Record should be no-op after close")
	}
}

func TestCollector_RecordSchema(t *testing.T) {
	c := NewCollector(CharsTokenizer{}, false)

	c.RecordSchema("get_issues", "jira", "Get Jira issues", []byte(`{"type":"object"}`))

	cost := c.SchemaCost()
	if cost.TotalTools != 1 {
		t.Errorf("TotalTools = %d, want 1", cost.TotalTools)
	}
	if cost.TotalSchemaTokens == 0 {
		t.Error("TotalSchemaTokens should be > 0")
	}
	if len(cost.ByPlugin) != 1 {
		t.Fatalf("expected 1 plugin, got %d", len(cost.ByPlugin))
	}
	if cost.ByPlugin[0].Plugin != "jira" {
		t.Errorf("Plugin = %q, want %q", cost.ByPlugin[0].Plugin, "jira")
	}
}

func TestCollector_RecordSchema_OverwriteOnReload(t *testing.T) {
	c := NewCollector(CharsTokenizer{}, false)

	c.RecordSchema("get_issues", "jira", "v1 description", []byte(`{"v":1}`))
	c.RecordSchema("get_issues", "jira", "v2 description with more text", []byte(`{"v":2,"extra":"field"}`))

	cost := c.SchemaCost()
	if cost.TotalTools != 1 {
		t.Errorf("TotalTools = %d, want 1 (should overwrite)", cost.TotalTools)
	}
}

func TestCollector_RemovePluginSchemas(t *testing.T) {
	c := NewCollector(CharsTokenizer{}, false)

	c.RecordSchema("get_issues", "jira", "desc", []byte(`{}`))
	c.RecordSchema("create_issue", "jira", "desc", []byte(`{}`))
	c.RecordSchema("list_agents", "keylime", "desc", []byte(`{}`))

	c.RemovePluginSchemas("jira")

	cost := c.SchemaCost()
	if cost.TotalTools != 1 {
		t.Errorf("TotalTools = %d, want 1 after removing jira schemas", cost.TotalTools)
	}
	if cost.ByPlugin[0].Plugin != "keylime" {
		t.Errorf("remaining plugin = %q, want keylime", cost.ByPlugin[0].Plugin)
	}
}

func TestCollector_RecordResourceRead(t *testing.T) {
	c := NewCollector(CharsTokenizer{}, false)

	c.RecordResourceRead("wtmcp://jira/context/context.md", "jira", "context", "# Jira Plugin\nUse this to...")

	resources := c.ResourceSummary()
	if len(resources) != 1 {
		t.Fatalf("expected 1 resource, got %d", len(resources))
	}
	r := resources[0]
	if r.URI != "wtmcp://jira/context/context.md" {
		t.Errorf("URI = %q", r.URI)
	}
	if r.ReadCount != 1 {
		t.Errorf("ReadCount = %d, want 1", r.ReadCount)
	}
	if r.ContentTokens == 0 {
		t.Error("ContentTokens should be > 0")
	}
}

func TestCollector_RecordResourceRead_Increments(t *testing.T) {
	c := NewCollector(CharsTokenizer{}, false)

	c.RecordResourceRead("uri://test", "plugin", "context", "content v1")
	c.RecordResourceRead("uri://test", "plugin", "context", "content v2 longer")

	resources := c.ResourceSummary()
	if len(resources) != 1 {
		t.Fatalf("expected 1 resource, got %d", len(resources))
	}
	if resources[0].ReadCount != 2 {
		t.Errorf("ReadCount = %d, want 2", resources[0].ReadCount)
	}
}

func TestCollector_RemovePluginResources(t *testing.T) {
	c := NewCollector(CharsTokenizer{}, false)

	c.RecordResourceRead("uri://jira/1", "jira", "provided", "data")
	c.RecordResourceRead("uri://keylime/1", "keylime", "context", "data")

	c.RemovePluginResources("jira")

	resources := c.ResourceSummary()
	if len(resources) != 1 {
		t.Errorf("expected 1 resource after removal, got %d", len(resources))
	}
	if resources[0].PluginName != "keylime" {
		t.Errorf("remaining resource plugin = %q, want keylime", resources[0].PluginName)
	}
}

func TestCollector_PluginSummaries(t *testing.T) {
	c := NewCollector(CharsTokenizer{}, false)

	c.Record("get_issues", "jira", time.Now(), nil, "ok", false)
	c.Record("create_issue", "jira", time.Now(), nil, "ok", false)
	c.Record("list_agents", "keylime", time.Now(), nil, "ok", false)

	ps := c.PluginSummaries()
	if len(ps) != 2 {
		t.Fatalf("expected 2 plugins, got %d", len(ps))
	}

	// Sorted by name: jira, keylime
	if ps[0].PluginName != "jira" || ps[0].CallCount != 2 || ps[0].ToolCount != 2 {
		t.Errorf("jira: calls=%d tools=%d, want 2,2", ps[0].CallCount, ps[0].ToolCount)
	}
	if ps[1].PluginName != "keylime" || ps[1].CallCount != 1 || ps[1].ToolCount != 1 {
		t.Errorf("keylime: calls=%d tools=%d, want 1,1", ps[1].CallCount, ps[1].ToolCount)
	}
}

func TestCollector_TotalTokens(t *testing.T) {
	c := NewCollector(CharsTokenizer{}, false)

	c.Record("tool1", "p", time.Now(), []byte("input"), "output text here", false)
	c.Record("tool2", "p", time.Now(), []byte("more"), "more output", false)

	input, output := c.TotalTokens()
	if input == 0 {
		t.Error("total input tokens should be > 0")
	}
	if output == 0 {
		t.Error("total output tokens should be > 0")
	}
}

func TestCollector_Nil_Tokenizer_Defaults(t *testing.T) {
	c := NewCollector(nil, false)
	if c.TokenizerName() != "chars" {
		t.Errorf("default tokenizer should be chars, got %q", c.TokenizerName())
	}
}
