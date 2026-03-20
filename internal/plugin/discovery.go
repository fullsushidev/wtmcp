package plugin

import (
	"fmt"
	"path/filepath"

	"github.com/LeGambiArt/wtmcp/internal/config"
)

// DiscoveryOptions configures plugin discovery behavior.
type DiscoveryOptions struct {
	ConfigPath         string // Optional config file path
	WorkdirOverride    string // Optional workdir override
	SkipConfigDisabled bool   // If true, ignore plugins.disabled during discovery
}

// DiscoveryResult contains the results of plugin discovery.
type DiscoveryResult struct {
	Workdir    string
	ConfigPath string // Resolved config file path (for write-back)
	Config     *config.Config
	EnvGroups  map[string]map[string]string
	EnvErrors  map[string]string
	Manager    *Manager
}

// Discover performs plugin discovery without loading plugins.
// This is the common discovery logic used by CLI tools and the check command.
// For the main server runtime, use the full initialization in run().
func Discover(opts DiscoveryOptions) (*DiscoveryResult, error) {
	// Resolve workdir
	workdir := config.WorkDir()
	if opts.WorkdirOverride != "" {
		workdir = opts.WorkdirOverride
	}

	// Load scoped env.d groups (not into process env)
	envResult, err := config.LoadEnvGroups(workdir)
	if err != nil {
		return nil, fmt.Errorf("load env: %w", err)
	}

	// Resolve config path (same defaulting logic as config.Load)
	cfgPath := opts.ConfigPath
	if cfgPath == "" {
		cfgPath = filepath.Join(workdir, "config.yaml")
	}

	// Load config (uses workdir for defaults)
	cfg, err := config.Load(opts.ConfigPath, workdir)
	if err != nil {
		return nil, fmt.Errorf("load config: %w", err)
	}

	// When SkipConfigDisabled is set, temporarily clear the disabled
	// list so all plugins end up in Manifests(). Restore it after
	// discovery so Config.Plugins.Disabled is available to the caller.
	var savedDisabled []string
	if opts.SkipConfigDisabled {
		savedDisabled = cfg.Plugins.Disabled
		cfg.Plugins.Disabled = nil
	}

	// Create manager with nil dependencies (discovery only)
	mgr := NewManager(nil, nil, nil, cfg, envResult.Groups, envResult.Errors, workdir)

	// Discover plugins (without loading/starting them)
	if err := mgr.Discover(cfg.PluginDirs, cfg.UserPluginDir); err != nil {
		return nil, fmt.Errorf("plugin discovery: %w", err)
	}

	if opts.SkipConfigDisabled {
		cfg.Plugins.Disabled = savedDisabled
	}

	return &DiscoveryResult{
		Workdir:    workdir,
		ConfigPath: cfgPath,
		Config:     cfg,
		EnvGroups:  envResult.Groups,
		EnvErrors:  envResult.Errors,
		Manager:    mgr,
	}, nil
}
