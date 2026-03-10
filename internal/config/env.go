package config

import (
	"bufio"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// WorkDir returns the base directory for all wtmcp data.
// Checks WTMCP_WORKDIR env var (falls back to WHAT_THE_MCP_WORKDIR
// for backwards compat), then ~/.config/wtmcp.
func WorkDir() string {
	if dir := os.Getenv("WTMCP_WORKDIR"); dir != "" {
		return dir
	}
	if dir := os.Getenv("WHAT_THE_MCP_WORKDIR"); dir != "" {
		return dir
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "."
	}
	return filepath.Join(home, ".config", "wtmcp")
}

// EnvGroups maps credential group names to their variables.
// Group name is derived from the env.d filename without the .env
// extension (e.g., env.d/jira.env → group "jira").
type EnvGroups map[string]map[string]string

// Get returns the variables for a credential group, or nil if the
// group does not exist.
func (g EnvGroups) Get(group string) map[string]string {
	if g == nil {
		return nil
	}
	return g[group]
}

// LoadEnvGroups reads env.d/*.env files and returns them as scoped
// groups. Each file becomes a group keyed by its filename without
// the .env extension. Variables are NOT loaded into the process
// environment — they are only available through the returned map.
//
// Plugin credentials and configuration must be set via env.d files;
// shell-exported environment variables are not used for plugin
// variable resolution.
func LoadEnvGroups(workdir string) (EnvGroups, error) {
	groups := make(EnvGroups)

	envDir := filepath.Join(workdir, "env.d")
	entries, err := os.ReadDir(envDir)
	if err != nil {
		if os.IsNotExist(err) {
			return groups, nil
		}
		return nil, fmt.Errorf("read %s: %w", envDir, err)
	}

	// Sort for deterministic order
	var files []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".env") {
			files = append(files, e.Name())
		}
	}
	sort.Strings(files)

	for _, name := range files {
		path := filepath.Join(envDir, name)
		vars, err := parseEnvFile(path)
		if err != nil {
			return nil, fmt.Errorf("load %s: %w", path, err)
		}
		group := strings.TrimSuffix(name, ".env")
		groups[group] = vars
		log.Printf("loaded env group: %s (%d vars)", group, len(vars))
	}

	return groups, nil
}

// parseEnvFile reads a .env file and returns its variables as a map.
// Lines starting with # are comments. Empty lines are skipped.
// Format: KEY=VALUE (double-quoted and single-quoted values have
// quotes stripped). The "export" prefix is also stripped.
func parseEnvFile(path string) (map[string]string, error) {
	f, err := os.Open(path) //nolint:gosec // env file path from config
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()

	vars := make(map[string]string)
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		// Skip empty lines and comments
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Skip export prefix
		line = strings.TrimPrefix(line, "export ")

		// Split on first =
		idx := strings.IndexByte(line, '=')
		if idx < 0 {
			continue
		}

		key := strings.TrimSpace(line[:idx])
		value := strings.TrimSpace(line[idx+1:])

		// Strip surrounding double quotes
		if len(value) >= 2 && value[0] == '"' && value[len(value)-1] == '"' {
			value = value[1 : len(value)-1]
		}
		// Strip surrounding single quotes
		if len(value) >= 2 && value[0] == '\'' && value[len(value)-1] == '\'' {
			value = value[1 : len(value)-1]
		}

		vars[key] = value
	}

	return vars, scanner.Err()
}

// StandardPaths returns the conventional paths derived from the workdir.
type StandardPaths struct {
	WorkDir        string
	ConfigFile     string
	EnvDir         string
	CredentialsDir string
	PluginsDir     string
	CacheDir       string
}

// Paths returns the standard directory layout for a workdir.
func Paths(workdir string) StandardPaths {
	return StandardPaths{
		WorkDir:        workdir,
		ConfigFile:     filepath.Join(workdir, "config.yaml"),
		EnvDir:         filepath.Join(workdir, "env.d"),
		CredentialsDir: filepath.Join(workdir, "credentials"),
		PluginsDir:     filepath.Join(workdir, "plugins"),
		CacheDir:       filepath.Join(workdir, "cache"),
	}
}
