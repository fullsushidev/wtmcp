// Package server wires the MCP server to the plugin manager,
// registering tools from plugin manifests and serving via stdio.
package server

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"

	"github.com/LeGambiArt/wtmcp/internal/config"
	"github.com/LeGambiArt/wtmcp/internal/encoding"

	"github.com/LeGambiArt/wtmcp/internal/plugin"
	"github.com/LeGambiArt/wtmcp/internal/pluginctx"
	"github.com/LeGambiArt/wtmcp/internal/protocol"
)

// New creates an MCP server with tools from all loaded plugins.
func New(version string, manager *plugin.Manager, cfg *config.Config) *mcpserver.MCPServer {
	srv := mcpserver.NewMCPServer(
		"wtmcp",
		version,
		mcpserver.WithToolCapabilities(true),
		mcpserver.WithResourceCapabilities(true, false),
	)

	// Register tools from all plugin manifests
	for _, manifest := range manager.Manifests() {
		outputFormat := cfg.Output.Format
		if manifest.Output.Format != "" {
			outputFormat = manifest.Output.Format
		}
		registerPluginTools(srv, manager, manifest, outputFormat, cfg.Output.ToonFallback)
	}

	// Register context files as MCP resources
	registerContextResources(srv, manager)

	// Built-in management tools
	registerManagementTools(srv, manager, cfg)

	return srv
}

func registerPluginTools(srv *mcpserver.MCPServer, mgr *plugin.Manager, manifest *plugin.Manifest, outputFormat string, toonFallback bool) {
	for _, toolDef := range manifest.Tools {
		tool := buildMCPTool(toolDef)
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

			result, err := handle.CallTool(ctx, toolName, params)
			if err != nil {
				var pluginErr *protocol.Error
				if isPluginError(err, &pluginErr) {
					return mcp.NewToolResultError(
						fmt.Sprintf("[%s] %s", pluginErr.Code, pluginErr.Message),
					), nil
				}
				return mcp.NewToolResultError(err.Error()), nil
			}

			// Apply output encoding (JSON passthrough or TOON)
			encoded := encoding.FormatResult(result, format, fallback)
			return mcp.NewToolResultText(encoded), nil
		})
	}
}

func buildMCPTool(def plugin.ToolDef) mcp.Tool {
	schema := def.ParamsSchema()
	schemaJSON, _ := json.Marshal(schema)
	return mcp.NewToolWithRawSchema(def.Name, def.Description, schemaJSON)
}

func registerManagementTools(srv *mcpserver.MCPServer, mgr *plugin.Manager, cfg *config.Config) {
	// plugin_list: list all loaded plugins
	srv.AddTool(
		mcp.NewTool("plugin_list",
			mcp.WithDescription("List all loaded plugins and their status"),
		),
		func(_ context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			var plugins []map[string]any
			for name, manifest := range mgr.Manifests() {
				plugins = append(plugins, map[string]any{
					"name":        name,
					"version":     manifest.Version,
					"description": manifest.Description,
					"execution":   manifest.Execution,
					"tools":       len(manifest.Tools),
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
			if err := ReloadPlugin(ctx, srv, mgr, cfg, name); err != nil {
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
func ReloadPlugin(ctx context.Context, srv *mcpserver.MCPServer, mgr *plugin.Manager, cfg *config.Config, name string) error {
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
		registerPluginTools(srv, mgr, manifest, outputFormat, cfg.Output.ToonFallback)
		registerPluginContextResources(srv, manifest)
	}

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
