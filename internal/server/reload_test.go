package server

import (
	"strings"
	"testing"

	"github.com/LeGambiArt/wtmcp/internal/config"
	"github.com/LeGambiArt/wtmcp/internal/plugin"
)

func TestReloadDisabledPlugin_StubToolsRemoved(t *testing.T) {
	mgr := plugin.NewManagerForTest()

	// Register a plugin as disabled (e.g., missing credentials).
	mgr.SetManifest("broken", &plugin.Manifest{
		Name: "broken",
		Tools: []plugin.ToolDef{
			{Name: "broken_search", Description: "Search things", Access: "read"},
			{Name: "broken_create", Description: "Create things", Access: "write"},
		},
	})
	mgr.SetEnvDisabledPlugin("broken", "env.d/broken.env: permissions too broad")

	cfg := config.DefaultConfig()
	index := NewToolIndex(mgr, false)
	srv := New("test", mgr, cfg, index, nil)

	// Verify disabled tools are registered with [DISABLED] descriptions.
	tools := srv.ListTools()
	st, ok := tools["broken_search"]
	if !ok {
		t.Fatal("disabled tool broken_search should be registered as stub")
	}
	if !strings.Contains(st.Tool.Description, "[DISABLED]") {
		t.Error("disabled tool should have [DISABLED] in description")
	}

	// Simulate what ReloadPlugin does: collect old tool names from
	// the disabled plugin, then delete them. Before the fix, this
	// code path only checked mgr.Manifests() and missed disabled
	// plugins entirely.
	var oldToolNames []string
	if manifest, ok := mgr.Manifests()["broken"]; ok {
		for _, tt := range manifest.Tools {
			oldToolNames = append(oldToolNames, tt.Name)
		}
	} else if dp, ok := mgr.EnvDisabledPlugins()["broken"]; ok {
		for _, tt := range dp.Manifest.Tools {
			oldToolNames = append(oldToolNames, tt.Name)
		}
	}

	if len(oldToolNames) != 2 {
		t.Fatalf("expected 2 old tool names from disabled plugin, got %d", len(oldToolNames))
	}

	// Delete old stubs.
	srv.DeleteTools(oldToolNames...)

	// Verify stubs are gone.
	tools = srv.ListTools()
	if _, ok := tools["broken_search"]; ok {
		t.Error("broken_search should have been removed after delete")
	}
	if _, ok := tools["broken_create"]; ok {
		t.Error("broken_create should have been removed after delete")
	}
}

func TestReloadDisabledPlugin_ManifestsPathStillWorks(t *testing.T) {
	mgr := plugin.NewManagerForTest()

	// Register a loaded (non-disabled) plugin.
	mgr.SetManifest("healthy", &plugin.Manifest{
		Name: "healthy",
		Tools: []plugin.ToolDef{
			{Name: "healthy_get", Description: "Get stuff", Access: "read"},
		},
	})
	mgr.SetHandle("healthy")

	cfg := config.DefaultConfig()
	index := NewToolIndex(mgr, false)
	srv := New("test", mgr, cfg, index, nil)

	// Verify the tool is registered normally.
	tools := srv.ListTools()
	st, ok := tools["healthy_get"]
	if !ok {
		t.Fatal("healthy_get should be registered")
	}
	if strings.Contains(st.Tool.Description, "[DISABLED]") {
		t.Error("loaded tool should not have [DISABLED] in description")
	}

	// Collect old tool names via the same logic as ReloadPlugin.
	var oldToolNames []string
	if manifest, ok := mgr.Manifests()["healthy"]; ok {
		for _, tt := range manifest.Tools {
			oldToolNames = append(oldToolNames, tt.Name)
		}
	} else if dp, ok := mgr.EnvDisabledPlugins()["healthy"]; ok {
		for _, tt := range dp.Manifest.Tools {
			oldToolNames = append(oldToolNames, tt.Name)
		}
	}

	if len(oldToolNames) != 1 {
		t.Fatalf("expected 1 old tool name from loaded plugin, got %d", len(oldToolNames))
	}
	if oldToolNames[0] != "healthy_get" {
		t.Errorf("expected healthy_get, got %s", oldToolNames[0])
	}
}
