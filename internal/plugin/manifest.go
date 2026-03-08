package plugin

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

// pluginNamePattern defines valid plugin names:
// lowercase alphanumeric, hyphens, underscores, 2-64 chars.
var pluginNamePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,62}[a-z0-9]$`)

// Manifest holds plugin metadata loaded from plugin.yaml.
type Manifest struct {
	Name        string `yaml:"name"`
	Version     string `yaml:"version"`
	Description string `yaml:"description"`

	Execution   string `yaml:"execution"`   // "oneshot" or "persistent"
	Concurrency int    `yaml:"concurrency"` // default: 1
	Handler     string `yaml:"handler"`

	DependsOn []string `yaml:"depends_on"`
	Env       []string `yaml:"env"` // additional env vars to pass through

	Services ServiceConfig     `yaml:"services"`
	Provides ProvidesConfig    `yaml:"provides"`
	Config   map[string]string `yaml:"config"`

	Tools        []ToolDef `yaml:"tools"`
	ContextFiles []string  `yaml:"context_files"`
	Priority     int       `yaml:"priority"`
	Enabled      *bool     `yaml:"enabled"`

	Output OutputConfig `yaml:"output"`

	// Dir is the directory containing this manifest (set at load time).
	Dir string `yaml:"-"`

	// resolvedConfig holds the resolved config values (env vars expanded).
	// Set by the plugin manager after loading.
	resolvedConfig json.RawMessage `yaml:"-"`
}

// ServiceConfig declares what services a plugin requires.
type ServiceConfig struct {
	Auth  AuthServiceConfig  `yaml:"auth"`
	HTTP  HTTPServiceConfig  `yaml:"http"`
	Cache CacheServiceConfig `yaml:"cache"`
}

// AuthServiceConfig declares auth requirements.
type AuthServiceConfig struct {
	Type            string                       `yaml:"type"`
	Token           string                       `yaml:"token"`
	Header          string                       `yaml:"header"`
	Prefix          string                       `yaml:"prefix"`
	Username        string                       `yaml:"username"`
	Password        string                       `yaml:"password"`
	SPN             string                       `yaml:"spn"`
	Scopes          []string                     `yaml:"scopes"`
	CredentialsFile string                       `yaml:"credentials_file"`
	TokenFile       string                       `yaml:"token_file"`
	Select          string                       `yaml:"select"`
	Variants        map[string]AuthServiceConfig `yaml:"variants"`
	VariantOrder    []string                     `yaml:"-"` // populated from YAML key order
}

// HTTPServiceConfig declares HTTP proxy settings.
type HTTPServiceConfig struct {
	BaseURL        string   `yaml:"base_url"`
	AllowedDomains []string `yaml:"allowed_domains"`
}

// CacheServiceConfig declares cache settings.
type CacheServiceConfig struct {
	Enabled    *bool  `yaml:"enabled"`
	Namespace  string `yaml:"namespace"`
	DefaultTTL int    `yaml:"default_ttl"`
}

// ProvidesConfig declares what services a plugin provides.
type ProvidesConfig struct {
	Auth *ProvidesAuthConfig `yaml:"auth"`
}

// ProvidesAuthConfig describes a plugin-provided auth type.
type ProvidesAuthConfig struct {
	Type        string `yaml:"type"`
	Description string `yaml:"description"`
}

// OutputConfig allows per-plugin output format override.
type OutputConfig struct {
	Format string `yaml:"format"`
}

// ToolDef declares an MCP tool with its parameter schema.
type ToolDef struct {
	Name        string              `yaml:"name"`
	Description string              `yaml:"description"`
	Params      map[string]ParamDef `yaml:"params"`
}

// ParamDef describes a tool parameter.
type ParamDef struct {
	Type        string    `yaml:"type"`
	Description string    `yaml:"description"`
	Required    bool      `yaml:"required"`
	Default     any       `yaml:"default"`
	Items       *ItemsDef `yaml:"items"`
}

// ItemsDef describes array item types.
type ItemsDef struct {
	Type string `yaml:"type"`
}

// IsEnabled returns whether the plugin is enabled (defaults to true).
func (m *Manifest) IsEnabled() bool {
	if m.Enabled == nil {
		return true
	}
	return *m.Enabled
}

// CacheEnabled returns whether cache is enabled (defaults to true).
func (m *Manifest) CacheEnabled() bool {
	if m.Services.Cache.Enabled == nil {
		return true
	}
	return *m.Services.Cache.Enabled
}

// CacheNamespace returns the cache namespace (defaults to plugin name).
func (m *Manifest) CacheNamespace() string {
	if m.Services.Cache.Namespace != "" {
		return m.Services.Cache.Namespace
	}
	return m.Name
}

// SetResolvedConfig sets the resolved config JSON for the plugin.
func (m *Manifest) SetResolvedConfig(cfg json.RawMessage) {
	m.resolvedConfig = cfg
}

// ProvidesAuth returns true if this plugin provides an auth type.
func (m *Manifest) ProvidesAuth() bool {
	return m.Provides.Auth != nil && m.Provides.Auth.Type != ""
}

// HandlerPath returns the absolute path to the handler executable.
func (m *Manifest) HandlerPath() string {
	return filepath.Join(m.Dir, m.Handler)
}

// LoadManifest reads and validates a plugin.yaml file.
func LoadManifest(path string) (*Manifest, error) {
	data, err := os.ReadFile(path) //nolint:gosec // plugin loading requires reading from variable paths
	if err != nil {
		return nil, fmt.Errorf("read manifest: %w", err)
	}

	var m Manifest
	if err := yaml.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("parse manifest %s: %w", path, err)
	}

	m.Dir = filepath.Dir(path)

	// Set defaults
	if m.Concurrency == 0 {
		m.Concurrency = 1
	}
	if m.Execution == "" {
		m.Execution = "persistent"
	}

	// Extract variant order from the raw YAML to preserve declaration order
	if m.Services.Auth.Variants != nil {
		m.Services.Auth.VariantOrder, err = extractVariantOrder(data)
		if err != nil {
			return nil, fmt.Errorf("parse auth variants order: %w", err)
		}
	}

	if err := m.Validate(); err != nil {
		return nil, fmt.Errorf("invalid manifest %s: %w", path, err)
	}

	return &m, nil
}

// Validate checks the manifest for correctness.
func (m *Manifest) Validate() error {
	if !pluginNamePattern.MatchString(m.Name) {
		return fmt.Errorf("invalid plugin name %q: must match [a-z0-9][a-z0-9_-]{0,62}[a-z0-9]", m.Name)
	}

	if m.Execution != "oneshot" && m.Execution != "persistent" {
		return fmt.Errorf("execution must be 'oneshot' or 'persistent', got %q", m.Execution)
	}

	if m.Handler == "" {
		return fmt.Errorf("handler is required")
	}

	// Verify handler path stays within plugin directory
	absHandler, err := filepath.Abs(filepath.Join(m.Dir, m.Handler))
	if err != nil {
		return fmt.Errorf("cannot resolve handler path: %w", err)
	}
	absDir, err := filepath.Abs(m.Dir)
	if err != nil {
		return fmt.Errorf("cannot resolve plugin dir: %w", err)
	}
	if !strings.HasPrefix(absHandler, absDir+string(filepath.Separator)) {
		return fmt.Errorf("handler path escapes plugin directory: %s", m.Handler)
	}

	// Validate base_url if set
	if m.Services.HTTP.BaseURL != "" {
		u, err := url.Parse(m.Services.HTTP.BaseURL)
		if err != nil {
			return fmt.Errorf("invalid base_url: %w", err)
		}
		if u.Scheme != "https" && u.Scheme != "http" {
			return fmt.Errorf("base_url must use http or https: %s", m.Services.HTTP.BaseURL)
		}
		if u.RawQuery != "" || u.Fragment != "" {
			return fmt.Errorf("base_url must not contain query or fragment: %s", m.Services.HTTP.BaseURL)
		}
	}

	// Validate tool names
	for _, tool := range m.Tools {
		if tool.Name == "" {
			return fmt.Errorf("tool name is required")
		}
	}

	return nil
}

// ParamsSchema converts the tool's parameter definitions to JSON Schema
// as required by the MCP spec.
func (t *ToolDef) ParamsSchema() map[string]any {
	properties := make(map[string]any)
	var required []string

	for name, p := range t.Params {
		prop := map[string]any{"type": p.Type}
		if p.Description != "" {
			prop["description"] = p.Description
		}
		if p.Default != nil {
			prop["default"] = p.Default
		}
		if p.Type == "array" && p.Items != nil {
			prop["items"] = map[string]any{"type": p.Items.Type}
		}
		properties[name] = prop
		if p.Required {
			required = append(required, name)
		}
	}

	schema := map[string]any{
		"type":       "object",
		"properties": properties,
	}
	if len(required) > 0 {
		schema["required"] = required
	}
	return schema
}

// extractVariantOrder parses the YAML to get auth variant keys in
// declaration order, since Go maps don't preserve insertion order.
func extractVariantOrder(data []byte) ([]string, error) {
	var raw struct {
		Services struct {
			Auth struct {
				Variants yaml.Node `yaml:"variants"`
			} `yaml:"auth"`
		} `yaml:"services"`
	}
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return nil, err
	}

	node := &raw.Services.Auth.Variants
	if node.Kind != yaml.MappingNode {
		return nil, nil
	}

	var order []string
	for i := 0; i < len(node.Content)-1; i += 2 {
		order = append(order, node.Content[i].Value)
	}
	return order, nil
}
