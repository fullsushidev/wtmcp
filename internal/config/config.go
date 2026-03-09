// Package config handles core configuration loading and environment
// variable resolution for what-the-mcp.
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// Config holds the core server configuration.
type Config struct {
	PluginDirs     []string      `yaml:"plugin_dirs"`
	CredentialsDir string        `yaml:"credentials_dir"`
	HTTP           HTTPConfig    `yaml:"http"`
	Cache          CacheConfig   `yaml:"cache"`
	Plugins        PluginsConfig `yaml:"plugins"`
	Output         OutputConfig  `yaml:"output"`
}

// HTTPConfig controls the HTTP proxy behavior.
type HTTPConfig struct {
	Timeout   time.Duration   `yaml:"timeout"`
	Retries   RetryConfig     `yaml:"retries"`
	RateLimit RateLimitConfig `yaml:"rate_limit"`
}

// RetryConfig controls retry behavior for HTTP requests.
type RetryConfig struct {
	Max     int    `yaml:"max"`
	Backoff string `yaml:"backoff"`
	RetryOn []int  `yaml:"retry_on"`
}

// RateLimitConfig controls request rate limiting.
type RateLimitConfig struct {
	Default   string            `yaml:"default"`
	PerPlugin map[string]string `yaml:"per_plugin"`
	PerDomain map[string]string `yaml:"per_domain"`
}

// CacheConfig controls the cache backend.
type CacheConfig struct {
	Backend             string        `yaml:"backend"`
	Dir                 string        `yaml:"dir"`
	MaxEntriesPerPlugin int           `yaml:"max_entries_per_plugin"`
	MaxEntrySize        int64         `yaml:"max_entry_size"`
	Eviction            string        `yaml:"eviction"`
	CleanupInterval     time.Duration `yaml:"cleanup_interval"`
}

// PluginsConfig controls plugin process management.
type PluginsConfig struct {
	MaxMessageSize    int64         `yaml:"max_message_size"`
	ToolCallTimeout   time.Duration `yaml:"tool_call_timeout"`
	InitTimeout       time.Duration `yaml:"init_timeout"`
	ShutdownTimeout   time.Duration `yaml:"shutdown_timeout"`
	ShutdownKillAfter time.Duration `yaml:"shutdown_kill_after"`
}

// OutputConfig controls tool result encoding.
type OutputConfig struct {
	Format       string `yaml:"format"`
	ToonFallback bool   `yaml:"toon_fallback"`
}

// DefaultConfig returns a Config with sensible defaults.
func DefaultConfig() *Config {
	return &Config{
		PluginDirs: []string{},
		HTTP: HTTPConfig{
			Timeout: 30 * time.Second,
			Retries: RetryConfig{
				Max:     3,
				Backoff: "exponential",
				RetryOn: []int{500, 502, 503, 504},
			},
		},
		Cache: CacheConfig{
			Backend:             "memory",
			MaxEntriesPerPlugin: 10000,
			MaxEntrySize:        1024 * 1024, // 1MB
			Eviction:            "lru",
			CleanupInterval:     60 * time.Second,
		},
		Plugins: PluginsConfig{
			MaxMessageSize:    10 * 1024 * 1024, // 10MB
			ToolCallTimeout:   60 * time.Second,
			InitTimeout:       30 * time.Second,
			ShutdownTimeout:   10 * time.Second,
			ShutdownKillAfter: 5 * time.Second,
		},
		Output: OutputConfig{
			Format:       "json",
			ToonFallback: true,
		},
	}
}

// Load reads a config file and merges with defaults. If configPath is empty,
// uses workdir/config.yaml. After loading, applies workdir-based defaults
// for any paths not explicitly set in the config file.
func Load(configPath, workdir string) (*Config, error) {
	cfg := DefaultConfig()

	if configPath == "" {
		configPath = filepath.Join(workdir, "config.yaml")
	}

	data, err := os.ReadFile(configPath) //nolint:gosec // config file path from user
	if err != nil {
		if os.IsNotExist(err) {
			applyWorkdirDefaults(cfg, workdir)
			return cfg, nil
		}
		return nil, fmt.Errorf("read config: %w", err)
	}

	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parse config %s: %w", configPath, err)
	}

	applyWorkdirDefaults(cfg, workdir)
	return cfg, nil
}

// applyWorkdirDefaults fills in paths that weren't set in the config
// using the standard workdir layout.
func applyWorkdirDefaults(cfg *Config, workdir string) {
	paths := Paths(workdir)

	if cfg.CredentialsDir == "" {
		cfg.CredentialsDir = paths.CredentialsDir
	} else {
		cfg.CredentialsDir = ResolveEnvVars(cfg.CredentialsDir)
	}

	if cfg.Cache.Dir == "" {
		cfg.Cache.Dir = paths.CacheDir
	} else {
		cfg.Cache.Dir = ResolveEnvVars(cfg.Cache.Dir)
	}

	// Build plugin dirs: system dir first, then user dir.
	// User plugins override system plugins with the same name.
	if len(cfg.PluginDirs) == 0 {
		cfg.PluginDirs = defaultPluginDirs(paths.PluginsDir)
	}
}

// defaultPluginDirs returns the plugin search path. System dirs are
// checked first; user dir is last (highest priority — overrides system
// plugins with the same name). Non-existent directories are included
// but silently skipped by Manager.Discover().
//
// Search order:
//  1. {binary}/../share/what-the-mcp/plugins (binary-relative)
//  2. /usr/share/what-the-mcp/plugins (system packages)
//  3. /usr/local/share/what-the-mcp/plugins (local installs, Homebrew)
//  4. {workdir}/plugins (user plugins)
func defaultPluginDirs(userDir string) []string {
	var dirs []string

	// Binary-relative: supports Homebrew, /usr/local, /opt, custom prefixes
	if exe, err := os.Executable(); err == nil {
		if resolved, err := filepath.EvalSymlinks(exe); err == nil {
			binRelative := filepath.Join(filepath.Dir(resolved), "..", "share", "what-the-mcp", "plugins")
			cleaned := filepath.Clean(binRelative)
			dirs = append(dirs, cleaned)
		}
	}

	// Standard system paths
	for _, sysDir := range []string{
		"/usr/share/what-the-mcp/plugins",
		"/usr/local/share/what-the-mcp/plugins",
	} {
		if !containsPath(dirs, sysDir) {
			dirs = append(dirs, sysDir)
		}
	}

	// User plugins last (highest priority override)
	dirs = append(dirs, userDir)
	return dirs
}

func containsPath(dirs []string, path string) bool {
	cleaned := filepath.Clean(path)
	for _, d := range dirs {
		if filepath.Clean(d) == cleaned {
			return true
		}
	}
	return false
}

// envVarPattern matches ${VAR} and ${VAR:-default} syntax.
var envVarPattern = regexp.MustCompile(`\$\$|\$\{([^}]+)\}`)

// ResolveEnvVars expands environment variable references in a string.
//
// Supported syntax:
//   - ${VAR}           — value of VAR, empty string if unset
//   - ${VAR:-default}  — value of VAR, or "default" if unset/empty
//   - $$               — literal dollar sign
func ResolveEnvVars(s string) string {
	return envVarPattern.ReplaceAllStringFunc(s, func(match string) string {
		if match == "$$" {
			return "$"
		}

		// Strip ${ and }
		inner := match[2 : len(match)-1]

		// Check for :-default syntax
		if idx := strings.Index(inner, ":-"); idx >= 0 {
			varName := inner[:idx]
			defaultVal := inner[idx+2:]
			if val, ok := os.LookupEnv(varName); ok && val != "" {
				return val
			}
			return defaultVal
		}

		// Simple ${VAR}
		return os.Getenv(inner)
	})
}

// ResolveEnvMap resolves all environment variable references in a
// string map, returning a new map with resolved values.
func ResolveEnvMap(m map[string]string) map[string]string {
	resolved := make(map[string]string, len(m))
	for k, v := range m {
		resolved[k] = ResolveEnvVars(v)
	}
	return resolved
}
