// Package pluginctx loads and serves plugin context/instruction files
// as MCP resources.
package pluginctx

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// LoadFile reads a context file from a plugin directory.
// Returns the content as a string, or an error if the file doesn't exist.
func LoadFile(pluginDir, filename string) (string, error) {
	path := filepath.Join(pluginDir, filename)

	// Verify the file stays within the plugin directory.
	// Resolve symlinks to prevent escaping via symlink chains.
	absPath, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("resolve context path: %w", err)
	}
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		absPath = resolved
	}
	absDir, err := filepath.Abs(pluginDir)
	if err != nil {
		return "", fmt.Errorf("resolve plugin dir: %w", err)
	}
	if resolvedDir, err := filepath.EvalSymlinks(pluginDir); err == nil {
		absDir = resolvedDir
	}
	if !strings.HasPrefix(absPath, absDir+string(filepath.Separator)) {
		return "", fmt.Errorf("context file escapes plugin directory: %s", filename)
	}

	data, err := os.ReadFile(absPath) //nolint:gosec // path validated above
	if err != nil {
		return "", fmt.Errorf("read context file %s: %w", filename, err)
	}
	return string(data), nil
}

// ResourceURI builds the MCP resource URI for a plugin context file.
func ResourceURI(pluginName, filename string) string {
	return fmt.Sprintf("wtmcp://plugin/%s/context/%s", pluginName, filename)
}
