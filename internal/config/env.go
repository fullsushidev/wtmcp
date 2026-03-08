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

// WorkDir returns the base directory for all what-the-mcp data.
// Checks WHAT_THE_MCP_WORKDIR env var, falls back to
// ~/.config/what-the-mcp.
func WorkDir() string {
	if dir := os.Getenv("WHAT_THE_MCP_WORKDIR"); dir != "" {
		return dir
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "."
	}
	return filepath.Join(home, ".config", "what-the-mcp")
}

// LoadDotEnv loads environment variables from .env files in the workdir.
//
// Loading order:
//  1. workdir/.env (if exists)
//  2. workdir/env.d/*.env (alphabetical order, if directory exists)
//
// Existing environment variables are NOT overwritten — process env
// takes precedence. This allows temporary overrides via shell export.
func LoadDotEnv(workdir string) error {
	// Load main .env
	mainEnv := filepath.Join(workdir, ".env")
	if err := loadEnvFile(mainEnv); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("load %s: %w", mainEnv, err)
	}

	// Load env.d/*.env
	envDir := filepath.Join(workdir, "env.d")
	entries, err := os.ReadDir(envDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("read %s: %w", envDir, err)
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
		if err := loadEnvFile(path); err != nil {
			return fmt.Errorf("load %s: %w", path, err)
		}
		log.Printf("loaded env: %s", name)
	}

	return nil
}

// loadEnvFile reads a .env file and sets environment variables.
// Lines starting with # are comments. Empty lines are skipped.
// Format: KEY=VALUE (no quotes handling needed for simple values,
// but double-quoted values have quotes stripped).
// Existing env vars are NOT overwritten.
func loadEnvFile(path string) error {
	f, err := os.Open(path) //nolint:gosec // env file path from config
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()

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

		// Don't overwrite existing env vars
		if _, exists := os.LookupEnv(key); !exists {
			_ = os.Setenv(key, value)
		}
	}

	return scanner.Err()
}

// StandardPaths returns the conventional paths derived from the workdir.
type StandardPaths struct {
	WorkDir        string
	ConfigFile     string
	EnvFile        string
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
		EnvFile:        filepath.Join(workdir, ".env"),
		EnvDir:         filepath.Join(workdir, "env.d"),
		CredentialsDir: filepath.Join(workdir, "credentials"),
		PluginsDir:     filepath.Join(workdir, "plugins"),
		CacheDir:       filepath.Join(workdir, "cache"),
	}
}
