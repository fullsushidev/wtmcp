package plugin

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/LeGambiArt/wtmcp/internal/protocol"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/LeGambiArt/wtmcp/internal/auth"
	"github.com/LeGambiArt/wtmcp/internal/cache"
	"github.com/LeGambiArt/wtmcp/internal/config"
	"github.com/LeGambiArt/wtmcp/internal/proxy"
)

// Manager discovers, loads, and manages plugin lifecycles.
type Manager struct {
	handles    map[string]*Handle
	manifests  map[string]*Manifest
	envGroups  config.EnvGroups
	authReg    *auth.Registry
	proxy      *proxy.Proxy
	cache      cache.Store
	cfg        *config.Config
	svcHandler *serviceHandlerImpl
}

// NewManager creates a plugin manager.
func NewManager(authReg *auth.Registry, p *proxy.Proxy, c cache.Store, cfg *config.Config, envGroups config.EnvGroups) *Manager {
	return &Manager{
		handles:    make(map[string]*Handle),
		manifests:  make(map[string]*Manifest),
		envGroups:  envGroups,
		authReg:    authReg,
		proxy:      p,
		cache:      c,
		cfg:        cfg,
		svcHandler: &serviceHandlerImpl{proxy: p, cache: c},
	}
}

// Discover scans directories for plugin.yaml files and loads manifests.
// First directory wins for a given plugin name; duplicates in later
// directories are skipped with a warning. userDir, if non-empty,
// identifies the user plugins directory — plugins from it are
// restricted (e.g., cannot declare provides.auth).
func (m *Manager) Discover(dirs []string, userDir string) error {
	for _, dir := range dirs {
		entries, err := os.ReadDir(dir)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return fmt.Errorf("read plugin dir %s: %w", dir, err)
		}
		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			manifestPath := filepath.Join(dir, entry.Name(), "plugin.yaml")
			if _, err := os.Stat(manifestPath); os.IsNotExist(err) {
				continue
			}
			manifest, err := LoadManifest(manifestPath)
			if err != nil {
				log.Printf("skipping plugin %s: %v", entry.Name(), err)
				continue
			}
			if !manifest.IsEnabled() {
				log.Printf("plugin %s is disabled", manifest.Name)
				continue
			}
			if existing, ok := m.manifests[manifest.Name]; ok {
				log.Printf("WARNING: plugin %q in %s skipped — already registered from %s",
					manifest.Name, manifest.Dir, existing.Dir)
				continue
			}
			if manifest.ProvidesAuth() && userDir != "" && dir == userDir {
				log.Printf("WARNING: user plugin %q declares provides.auth — skipped (not allowed)",
					manifest.Name)
				continue
			}
			m.manifests[manifest.Name] = manifest
		}
	}
	return nil
}

// LoadAll loads all discovered plugins in dependency order.
// Auth-providing plugins are loaded first (two-pass loading).
func (m *Manager) LoadAll(ctx context.Context) error {
	sorted, err := m.topologicalSort()
	if err != nil {
		return fmt.Errorf("dependency resolution: %w", err)
	}

	// Pass 1: load auth-providing plugins
	for _, name := range sorted {
		manifest := m.manifests[name]
		if manifest.ProvidesAuth() {
			if err := m.Load(ctx, name); err != nil {
				log.Printf("failed to load auth provider %s: %v", name, err)
			}
		}
	}

	// Pass 2: load everything else
	for _, name := range sorted {
		manifest := m.manifests[name]
		if manifest.ProvidesAuth() {
			continue // already loaded
		}
		if err := m.Load(ctx, name); err != nil {
			log.Printf("failed to load plugin %s: %v", name, err)
		}
	}

	return nil
}

// Load starts a single plugin by name.
func (m *Manager) Load(ctx context.Context, name string) error {
	manifest, ok := m.manifests[name]
	if !ok {
		return fmt.Errorf("unknown plugin: %s", name)
	}

	// Resolve config
	resolvedCfg := m.resolveConfig(manifest)
	cfgJSON, err := json.Marshal(resolvedCfg)
	if err != nil {
		return fmt.Errorf("marshal config for %s: %w", name, err)
	}
	manifest.SetResolvedConfig(cfgJSON)

	// Register with proxy
	vars := m.pluginVars(manifest)
	pa := &proxy.PluginAuth{
		BaseURL:        config.ResolveVars(manifest.Services.HTTP.BaseURL, vars),
		AllowedDomains: manifest.Services.HTTP.AllowedDomains,
	}

	if m.isKerberosAuth(manifest) {
		spn := config.ResolveVars(manifest.Services.Auth.SPN, vars)
		pa.Client = proxy.NewKerberosClient(spn)
		log.Printf("[%s] using kerberos client (spn=%q)", name, spn)
	} else {
		pa.Provider = m.resolveAuth(manifest)
	}

	m.proxy.RegisterPlugin(name, pa)

	// Create handle and start
	processCfg := ProcessConfig{
		InitTimeout:       m.cfg.Plugins.InitTimeout,
		ShutdownTimeout:   m.cfg.Plugins.ShutdownTimeout,
		ShutdownKillAfter: m.cfg.Plugins.ShutdownKillAfter,
		MaxMessageSize:    int(m.cfg.Plugins.MaxMessageSize),
	}

	handle := NewHandle(manifest, m.svcHandler, processCfg, m.cfg.Plugins.ToolCallTimeout, vars)

	if manifest.Execution == "persistent" {
		if err := handle.Start(ctx); err != nil {
			return err
		}
	}

	m.handles[name] = handle
	log.Printf("loaded plugin %s (v%s, %s)", name, manifest.Version, manifest.Execution)
	return nil
}

// Unload stops a plugin.
func (m *Manager) Unload(ctx context.Context, name string) error {
	handle, ok := m.handles[name]
	if !ok {
		return fmt.Errorf("plugin not loaded: %s", name)
	}
	if err := handle.Stop(ctx); err != nil {
		return err
	}
	delete(m.handles, name)
	log.Printf("unloaded plugin %s", name)
	return nil
}

// Reload stops and restarts a plugin.
func (m *Manager) Reload(ctx context.Context, name string) error {
	if _, ok := m.handles[name]; ok {
		if err := m.Unload(ctx, name); err != nil {
			return err
		}
	}
	return m.Load(ctx, name)
}

// ShutdownAll stops all loaded plugins.
func (m *Manager) ShutdownAll(ctx context.Context) {
	for name, handle := range m.handles {
		if err := handle.Stop(ctx); err != nil {
			log.Printf("error stopping %s: %v", name, err)
		}
	}
}

// CallTool dispatches a tool call to the appropriate plugin.
func (m *Manager) CallTool(_ context.Context, toolName string) (string, *Handle) {
	for name, manifest := range m.manifests {
		for _, tool := range manifest.Tools {
			if tool.Name == toolName {
				handle, ok := m.handles[name]
				if !ok {
					return name, nil
				}
				return name, handle
			}
		}
	}
	return "", nil
}

// Manifests returns all discovered manifests.
func (m *Manager) Manifests() map[string]*Manifest {
	return m.manifests
}

// ToolOwner returns the plugin name that owns a tool.
func (m *Manager) ToolOwner(toolName string) string {
	name, _ := m.CallTool(context.Background(), toolName)
	return name
}

// pluginVars returns the scoped env.d variables for a plugin based
// on its credential_group. Returns nil if no group is declared or
// no matching env.d file exists.
func (m *Manager) pluginVars(manifest *Manifest) map[string]string {
	if manifest.CredentialGroup == "" {
		return nil
	}
	return m.envGroups.Get(manifest.CredentialGroup)
}

func (m *Manager) resolveConfig(manifest *Manifest) map[string]string {
	return config.ResolveVarsMap(manifest.Config, m.pluginVars(manifest))
}

// isKerberosAuth checks if a plugin uses Kerberos auth (without variants).
// Variant-based Kerberos (like Jira's server-kerberos) goes through the
// normal resolveAuth path; only pure Kerberos plugins get a per-plugin client.
func (m *Manager) isKerberosAuth(manifest *Manifest) bool {
	authCfg := manifest.Services.Auth
	if len(authCfg.Variants) > 0 {
		return false
	}
	return authCfg.Type == "kerberos" || authCfg.Type == "kerberos/spnego"
}

func (m *Manager) resolveAuth(manifest *Manifest) auth.Provider {
	authCfg := manifest.Services.Auth
	if authCfg.Type == "" && len(authCfg.Variants) == 0 {
		return nil
	}

	vars := m.pluginVars(manifest)
	resolve := func(s string) string { return config.ResolveVars(s, vars) }

	var variantCfg auth.VariantConfig
	if len(authCfg.Variants) > 0 {
		variantCfg.Select = resolve(authCfg.Select)
		variantCfg.VariantOrder = authCfg.VariantOrder
		variantCfg.Variants = make(map[string]auth.SingleAuthConfig)
		for name, v := range authCfg.Variants {
			variantCfg.Variants[name] = auth.SingleAuthConfig{
				Type:            v.Type,
				Token:           resolve(v.Token),
				Header:          v.Header,
				Prefix:          v.Prefix,
				Username:        resolve(v.Username),
				Password:        resolve(v.Password),
				SPN:             resolve(v.SPN),
				Scopes:          v.Scopes,
				CredentialsFile: resolve(v.CredentialsFile),
				TokenFile:       resolve(v.TokenFile),
				CredentialsDir:  m.cfg.CredentialsDir,
			}
		}
	} else {
		// Single auth type — resolve vars and wrap as a single
		// variant so ResolveVariant gets the full config.
		variantCfg.Select = "default"
		variantCfg.VariantOrder = []string{"default"}
		variantCfg.Variants = map[string]auth.SingleAuthConfig{
			"default": {
				Type:            authCfg.Type,
				Token:           resolve(authCfg.Token),
				Header:          authCfg.Header,
				Prefix:          authCfg.Prefix,
				Username:        resolve(authCfg.Username),
				Password:        resolve(authCfg.Password),
				SPN:             resolve(authCfg.SPN),
				Scopes:          authCfg.Scopes,
				CredentialsFile: resolve(authCfg.CredentialsFile),
				TokenFile:       resolve(authCfg.TokenFile),
				CredentialsDir:  m.cfg.CredentialsDir,
			},
		}
	}

	provider, err := auth.ResolveVariant(variantCfg)
	if err != nil {
		log.Printf("[%s] auth resolution failed: %v", manifest.Name, err)
		return nil
	}
	return provider
}

func (m *Manager) topologicalSort() ([]string, error) {
	// Pre-filter: skip plugins with unresolvable dependencies
	// instead of aborting the entire sort.
	skipped := make(map[string]bool)
	for name, manifest := range m.manifests {
		for _, dep := range manifest.DependsOn {
			if _, exists := m.manifests[dep]; !exists {
				log.Printf("WARNING: plugin %s depends on %s which is not available — skipping",
					name, dep)
				skipped[name] = true
				break
			}
		}
	}

	// Build adjacency from depends_on (excluding skipped)
	deps := make(map[string][]string)
	for name, manifest := range m.manifests {
		if skipped[name] {
			continue
		}
		deps[name] = manifest.DependsOn
	}

	var sorted []string
	visited := make(map[string]bool)
	visiting := make(map[string]bool)

	var visit func(string) error
	visit = func(name string) error {
		if visited[name] {
			return nil
		}
		if visiting[name] {
			return fmt.Errorf("circular dependency involving %s", name)
		}
		visiting[name] = true

		for _, dep := range deps[name] {
			if err := visit(dep); err != nil {
				return err
			}
		}

		visiting[name] = false
		visited[name] = true
		sorted = append(sorted, name)
		return nil
	}

	// Visit all plugins, sorted by priority for deterministic order
	names := m.sortedByPriority()
	for _, name := range names {
		if skipped[name] {
			continue
		}
		if err := visit(name); err != nil {
			return nil, err
		}
	}

	return sorted, nil
}

func (m *Manager) sortedByPriority() []string {
	type entry struct {
		name     string
		priority int
	}
	var entries []entry
	for name, manifest := range m.manifests {
		entries = append(entries, entry{name: name, priority: manifest.Priority})
	}
	// Simple insertion sort — plugin count is small
	for i := 1; i < len(entries); i++ {
		for j := i; j > 0 && entries[j].priority < entries[j-1].priority; j-- {
			entries[j], entries[j-1] = entries[j-1], entries[j]
		}
	}
	names := make([]string, len(entries))
	for i, e := range entries {
		names[i] = e.name
	}
	return names
}

// serviceHandlerImpl implements ServiceHandler by delegating to proxy and cache.
type serviceHandlerImpl struct {
	proxy *proxy.Proxy
	cache cache.Store
}

func (s *serviceHandlerImpl) HandleHTTP(pluginName string, req protocol.Message) protocol.Message {
	return s.proxy.Execute(context.Background(), pluginName, req)
}

func (s *serviceHandlerImpl) HandleCache(pluginName string, req protocol.Message) protocol.Message {
	ctx := context.Background()
	namespace := pluginName // default namespace

	switch req.Type {
	case protocol.TypeCacheGet:
		if err := cache.ValidateKey(req.Key); err != nil {
			return cacheError(req.ID, req.Type, err)
		}
		value, hit, err := s.cache.Get(ctx, namespace, req.Key)
		if err != nil {
			return cacheError(req.ID, req.Type, err)
		}
		h := hit
		resp := protocol.Message{ID: req.ID, Type: protocol.TypeCacheGet, Hit: &h}
		if hit {
			resp.Value = value
		}
		return resp

	case protocol.TypeCacheSet:
		if err := cache.ValidateKey(req.Key); err != nil {
			return cacheError(req.ID, req.Type, err)
		}
		ttl := time.Duration(0)
		if req.TTL != nil {
			ttl = time.Duration(*req.TTL) * time.Second
		}
		err := s.cache.Set(ctx, namespace, req.Key, req.Value, ttl)
		ok := err == nil
		resp := protocol.Message{ID: req.ID, Type: protocol.TypeCacheSet, OK: &ok}
		if err != nil {
			resp.Error = &protocol.Error{Code: "cache_error", Message: err.Error()}
		}
		return resp

	case protocol.TypeCacheDel:
		if err := cache.ValidateKey(req.Key); err != nil {
			return cacheError(req.ID, req.Type, err)
		}
		deleted, err := s.cache.Del(ctx, namespace, req.Key)
		if err != nil {
			return cacheError(req.ID, req.Type, err)
		}
		ok := true
		return protocol.Message{ID: req.ID, Type: protocol.TypeCacheDel, OK: &ok, Deleted: &deleted}

	case protocol.TypeCacheList:
		keys, err := s.cache.List(ctx, namespace, req.Pattern)
		if err != nil {
			return cacheError(req.ID, req.Type, err)
		}
		return protocol.Message{ID: req.ID, Type: protocol.TypeCacheList, Keys: keys}

	case protocol.TypeCacheFlush:
		count, err := s.cache.Flush(ctx, namespace)
		if err != nil {
			return cacheError(req.ID, req.Type, err)
		}
		ok := true
		return protocol.Message{ID: req.ID, Type: protocol.TypeCacheFlush, OK: &ok, Count: &count}

	default:
		return protocol.Message{
			ID:    req.ID,
			Type:  req.Type,
			Error: &protocol.Error{Code: "unknown_cache_op", Message: "unknown cache operation: " + req.Type},
		}
	}
}

func cacheError(id, msgType string, err error) protocol.Message {
	return protocol.Message{
		ID:    id,
		Type:  msgType,
		Error: &protocol.Error{Code: "cache_error", Message: err.Error()},
	}
}
