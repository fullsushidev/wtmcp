// Package server wires the MCP server to the plugin manager,
// registering tools from plugin manifests and serving via stdio.
package server

import (
	"context"
	"encoding/json"
	"fmt"
	"log"

	"github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"

	"github.com/LeGambiArt/wtmcp/internal/config"
	"github.com/LeGambiArt/wtmcp/internal/encoding"

	"github.com/LeGambiArt/wtmcp/internal/plugin"
	"github.com/LeGambiArt/wtmcp/internal/pluginctx"
	"github.com/LeGambiArt/wtmcp/internal/protocol"
)

// New creates an MCP server with tools from all loaded plugins.
// The index is used for tool_search and must be rebuilt on plugin
// reload via ReloadPlugin.
func New(version string, manager *plugin.Manager, cfg *config.Config, index *ToolIndex) *mcpserver.MCPServer {
	srv := mcpserver.NewMCPServer(
		"wtmcp",
		version,
		mcpserver.WithToolCapabilities(true),
		mcpserver.WithResourceCapabilities(true, false),
	)

	progressive := cfg.Tools.Discovery == "progressive"

	disabled := manager.DisabledPlugins()

	// Register tools from all plugin manifests. In progressive
	// mode, non-primary tools get the defer_loading flag.
	// Skip disabled plugins — they get separate registration below.
	for name, manifest := range manager.Manifests() {
		if _, isDisabled := disabled[name]; isDisabled {
			continue
		}
		outputFormat := cfg.Output.Format
		if manifest.Output.Format != "" {
			outputFormat = manifest.Output.Format
		}
		registerPluginTools(srv, manager, manifest, outputFormat, cfg.Output.ToonFallback, progressive)
	}

	// Register disabled plugin tools with [DISABLED] descriptions
	registerDisabledPluginTools(srv, disabled, progressive)

	// Register context files as MCP resources
	registerContextResources(srv, manager)

	// Register resources from resource provider plugins
	registerPluginProvidedResources(srv, manager)

	// Built-in management tools
	registerManagementTools(srv, manager, cfg, index)

	// tool_search — useful in both modes
	registerToolSearch(srv, index)

	return srv
}

func registerPluginTools(srv *mcpserver.MCPServer, mgr *plugin.Manager, manifest *plugin.Manifest, outputFormat string, toonFallback bool, progressive bool) {
	for _, toolDef := range manifest.Tools {
		tool := buildMCPTool(toolDef, progressive)
		toolName := toolDef.Name
		format := outputFormat
		fallback := toonFallback

		srv.AddTool(tool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			_, handle := mgr.CallTool(ctx, toolName)
			if handle == nil {
				return mcp.NewToolResultError(fmt.Sprintf("plugin for tool %s not loaded", toolName)), nil
			}

			params, err := json.Marshal(req.GetArguments())
			if err != nil {
				return mcp.NewToolResultError("invalid parameters: " + err.Error()), nil //nolint:nilerr // MCP convention: tool errors returned as result, not Go error
			}

			callResult, err := handle.CallTool(ctx, toolName, params)
			if err != nil {
				var pluginErr *protocol.Error
				if isPluginError(err, &pluginErr) {
					return mcp.NewToolResultError(
						fmt.Sprintf("[%s] %s", pluginErr.Code, pluginErr.Message),
					), nil
				}
				return mcp.NewToolResultError(err.Error()), nil
			}

			// Process post-tool actions in background
			if len(callResult.Actions) > 0 {
				go processToolActions(srv, mgr, manifest.Name, callResult.Actions)
			}

			// Apply output encoding (JSON passthrough or TOON)
			encoded := encoding.FormatResult(callResult.Result, format, fallback)
			return mcp.NewToolResultText(encoded), nil
		})
	}
}

func buildMCPTool(def plugin.ToolDef, progressive bool) mcp.Tool {
	schema := def.ParamsSchema()
	schemaJSON, _ := json.Marshal(schema)
	tool := mcp.NewToolWithRawSchema(def.Name, def.Description, schemaJSON)

	if progressive && !def.IsPrimary() {
		tool.DeferLoading = true
	}

	if def.IsReadOnly() {
		t := true
		tool.Annotations.ReadOnlyHint = &t
	} else {
		t := true
		tool.Annotations.DestructiveHint = &t
	}

	return tool
}

func registerDisabledPluginTools(srv *mcpserver.MCPServer, disabled map[string]plugin.DisabledPlugin, progressive bool) {
	for _, dp := range disabled {
		pluginName := dp.Name
		for _, toolDef := range dp.Manifest.Tools {
			tool := buildMCPTool(toolDef, progressive)
			tool.Description = fmt.Sprintf(
				"[DISABLED] %s — after fixing, run plugin_reload(name=\"%s\") to enable.\n\n---\n\n%s",
				dp.Reason, pluginName, toolDef.Description,
			)

			reason := dp.Reason
			name := pluginName
			srv.AddTool(tool, func(_ context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
				return mcp.NewToolResultError(fmt.Sprintf(
					"[DISABLED] %s\n\nAfter fixing, run: plugin_reload(name=\"%s\")",
					reason, name,
				)), nil
			})
		}
	}
}

func registerManagementTools(srv *mcpserver.MCPServer, mgr *plugin.Manager, cfg *config.Config, index *ToolIndex) {
	// plugin_list: list all plugins and their status
	srv.AddTool(
		mcp.NewTool("plugin_list",
			mcp.WithDescription("List all plugins and their status (loaded, disabled)"),
		),
		func(_ context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			var plugins []map[string]any

			disabled := mgr.DisabledPlugins()
			for name, manifest := range mgr.Manifests() {
				if dp, ok := disabled[name]; ok {
					plugins = append(plugins, map[string]any{
						"name":             name,
						"status":           "disabled",
						"reason":           dp.Reason,
						"credential_group": manifest.CredentialGroup,
						"tools":            len(manifest.Tools),
					})
					continue
				}

				var primaryCount, deferredCount int
				for _, t := range manifest.Tools {
					if t.IsPrimary() {
						primaryCount++
					} else {
						deferredCount++
					}
				}
				plugins = append(plugins, map[string]any{
					"name":        name,
					"version":     manifest.Version,
					"description": manifest.Description,
					"execution":   manifest.Execution,
					"tools":       len(manifest.Tools),
					"primary":     primaryCount,
					"deferred":    deferredCount,
				})
			}
			data, _ := json.Marshal(plugins)
			return mcp.NewToolResultText(string(data)), nil
		},
	)

	// plugin_reload: reload a plugin by name
	srv.AddTool(
		mcp.NewTool("plugin_reload",
			mcp.WithDescription("Reload a plugin by name, re-registering tools and context resources"),
			mcp.WithString("name", mcp.Required(), mcp.Description("Plugin name to reload")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			name, ok := req.GetArguments()["name"].(string)
			if !ok || name == "" {
				return mcp.NewToolResultError("name is required"), nil
			}
			if err := ReloadPlugin(ctx, srv, mgr, cfg, name, index); err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			return mcp.NewToolResultText(fmt.Sprintf("plugin %s reloaded", name)), nil
		},
	)
}

// ReloadPlugin reloads a plugin and re-registers its tools and context
// resources with the MCP server. The mcp-go library automatically sends
// notifications/tools/list_changed and notifications/resources/list_changed
// when tools and resources are added or deleted.
//
// The index is rebuilt to reflect manifest changes, and tool_search is
// re-registered so its CategorySummary stays current.
func ReloadPlugin(ctx context.Context, srv *mcpserver.MCPServer, mgr *plugin.Manager, cfg *config.Config, name string, index *ToolIndex) error {
	progressive := cfg.Tools.Discovery == "progressive"

	// Collect old tool names and context URIs before reload
	var oldToolNames []string
	var oldContextURIs []string
	if manifest, ok := mgr.Manifests()[name]; ok {
		for _, t := range manifest.Tools {
			oldToolNames = append(oldToolNames, t.Name)
		}
		for _, f := range manifest.ContextFiles {
			oldContextURIs = append(oldContextURIs, pluginctx.ResourceURI(name, f))
		}
	}

	// Reload the plugin (stops handler, re-reads manifest, restarts)
	if err := mgr.Reload(ctx, name); err != nil {
		return err
	}

	// Remove old tools and context resources
	if len(oldToolNames) > 0 {
		srv.DeleteTools(oldToolNames...)
	}
	if len(oldContextURIs) > 0 {
		srv.DeleteResources(oldContextURIs...)
	}

	// Re-register from refreshed manifest
	if manifest, ok := mgr.Manifests()[name]; ok {
		outputFormat := cfg.Output.Format
		if manifest.Output.Format != "" {
			outputFormat = manifest.Output.Format
		}
		registerPluginTools(srv, mgr, manifest, outputFormat, cfg.Output.ToonFallback, progressive)
		registerPluginContextResources(srv, manifest)
	}

	// Rebuild tool index and re-register tool_search so the
	// CategorySummary reflects the reloaded manifest.
	index.Rebuild(mgr)
	srv.DeleteTools("tool_search")
	registerToolSearch(srv, index)

	return nil
}

func registerContextResources(srv *mcpserver.MCPServer, mgr *plugin.Manager) {
	for _, manifest := range mgr.Manifests() {
		registerPluginContextResources(srv, manifest)
	}
}

func registerPluginContextResources(srv *mcpserver.MCPServer, manifest *plugin.Manifest) {
	for _, ctxFile := range manifest.ContextFiles {
		uri := pluginctx.ResourceURI(manifest.Name, ctxFile)
		dir := manifest.Dir
		file := ctxFile

		srv.AddResource(
			mcp.NewResource(uri, manifest.Name+" context: "+file,
				mcp.WithResourceDescription(fmt.Sprintf("Context instructions for %s plugin", manifest.Name)),
				mcp.WithMIMEType("text/markdown"),
			),
			func(_ context.Context, _ mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
				content, err := pluginctx.LoadFile(dir, file)
				if err != nil {
					return nil, err
				}
				return []mcp.ResourceContents{
					mcp.TextResourceContents{
						URI:      uri,
						MIMEType: "text/markdown",
						Text:     content,
					},
				}, nil
			},
		)
	}
}

// processToolActions handles side effects declared in tool results.
func processToolActions(srv *mcpserver.MCPServer, mgr *plugin.Manager, pluginName string, actions []protocol.Action) {
	for _, action := range actions {
		switch action.Type {
		case "invalidate_resources":
			invalidatePluginResources(srv, mgr, pluginName)
		default:
			log.Printf("[%s] unknown tool action: %s", pluginName, action.Type)
		}
	}
}

// invalidatePluginResources re-queries a resource provider and updates
// MCP registrations by diffing old vs new resource URIs.
func invalidatePluginResources(srv *mcpserver.MCPServer, mgr *plugin.Manager, pluginName string) {
	manifest, ok := mgr.Manifests()[pluginName]
	if !ok || !manifest.ProvidesResources() {
		return
	}
	handle := mgr.Handle(pluginName)
	if handle == nil {
		return
	}

	oldResources := handle.InitialResources()
	oldURIs := make(map[string]bool, len(oldResources))
	for _, r := range oldResources {
		oldURIs[r.URI] = true
	}

	newResources, err := handle.ListResources(context.Background())
	if err != nil {
		log.Printf("[%s] invalidate_resources failed: %v", pluginName, err)
		return
	}
	handle.SetResources(newResources)

	newURIs := make(map[string]bool, len(newResources))
	for _, r := range newResources {
		newURIs[r.URI] = true
	}
	var toRemove []string
	for uri := range oldURIs {
		if !newURIs[uri] {
			toRemove = append(toRemove, uri)
		}
	}
	if len(toRemove) > 0 {
		srv.DeleteResources(toRemove...)
	}

	registerHandleResources(srv, pluginName, handle)

	log.Printf("[%s] invalidate_resources: %d resources (%d removed)",
		pluginName, len(newResources), len(toRemove))
}

// registerPluginProvidedResources registers resources from plugins that
// declare provides.resources: true.
func registerPluginProvidedResources(srv *mcpserver.MCPServer, mgr *plugin.Manager) {
	for name, manifest := range mgr.Manifests() {
		if !manifest.ProvidesResources() {
			continue
		}
		handle := mgr.Handle(name)
		if handle == nil {
			continue
		}
		registerHandleResources(srv, name, handle)
	}
}

func registerHandleResources(srv *mcpserver.MCPServer, _ string, handle *plugin.Handle) {
	for _, res := range handle.InitialResources() {
		uri := res.URI
		mimeType := res.MIMEType
		if mimeType == "" {
			mimeType = "text/plain"
		}
		srv.AddResource(
			mcp.NewResource(uri, res.Name,
				mcp.WithResourceDescription(res.Description),
				mcp.WithMIMEType(mimeType),
			),
			func(ctx context.Context, _ mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
				content, actualMIME, err := handle.ReadResource(ctx, uri)
				if err != nil {
					return nil, fmt.Errorf("read resource %s: %w", uri, err)
				}
				return []mcp.ResourceContents{
					mcp.TextResourceContents{
						URI:      uri,
						MIMEType: actualMIME,
						Text:     content,
					},
				}, nil
			},
		)
	}
}

// isPluginError checks if the error is a protocol.Error using errors.As.
func isPluginError(err error, target **protocol.Error) bool {
	for {
		if pe, ok := err.(*protocol.Error); ok { //nolint:errorlint // checking concrete type intentionally
			*target = pe
			return true
		}
		unwrapper, ok := err.(interface{ Unwrap() error })
		if !ok {
			return false
		}
		err = unwrapper.Unwrap()
	}
}
