// Package server wires the MCP server to the plugin manager,
// registering tools from plugin manifests and serving via stdio.
package server

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"

	"gitlab.cee.redhat.com/bragctl/what-the-mcp/internal/plugin"
	"gitlab.cee.redhat.com/bragctl/what-the-mcp/internal/protocol"
)

// New creates an MCP server with tools from all loaded plugins.
func New(version string, manager *plugin.Manager) *mcpserver.MCPServer {
	srv := mcpserver.NewMCPServer(
		"what-the-mcp",
		version,
		mcpserver.WithToolCapabilities(true),
		mcpserver.WithResourceCapabilities(true, false),
	)

	// Register tools from all plugin manifests
	for _, manifest := range manager.Manifests() {
		registerPluginTools(srv, manager, manifest)
	}

	// Built-in management tools
	registerManagementTools(srv, manager)

	return srv
}

func registerPluginTools(srv *mcpserver.MCPServer, mgr *plugin.Manager, manifest *plugin.Manifest) {
	for _, toolDef := range manifest.Tools {
		tool := buildMCPTool(toolDef)
		toolName := toolDef.Name

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

			return mcp.NewToolResultText(string(result)), nil
		})
	}
}

func buildMCPTool(def plugin.ToolDef) mcp.Tool {
	schema := def.ParamsSchema()
	schemaJSON, _ := json.Marshal(schema)

	return mcp.NewTool(
		def.Name,
		mcp.WithDescription(def.Description),
		mcp.WithRawInputSchema(schemaJSON),
	)
}

func registerManagementTools(srv *mcpserver.MCPServer, mgr *plugin.Manager) {
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
			mcp.WithDescription("Reload a plugin by name"),
			mcp.WithString("name", mcp.Required(), mcp.Description("Plugin name to reload")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			name, ok := req.GetArguments()["name"].(string)
			if !ok || name == "" {
				return mcp.NewToolResultError("name is required"), nil
			}
			if err := mgr.Reload(ctx, name); err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			return mcp.NewToolResultText(fmt.Sprintf("plugin %s reloaded", name)), nil
		},
	)
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
