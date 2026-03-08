package auth

import "fmt"

// VariantConfig describes an auth configuration with optional variants.
type VariantConfig struct {
	// Type is the auth provider type (e.g., "bearer", "basic").
	// Used when there are no variants.
	Type string

	// Select chooses a variant: explicit name or "auto".
	Select string

	// VariantOrder preserves declaration order for auto-detection.
	VariantOrder []string

	// Variants maps variant names to their auth configs.
	Variants map[string]SingleAuthConfig
}

// SingleAuthConfig is the config for a single auth provider instance.
type SingleAuthConfig struct {
	Type     string
	Token    string
	Header   string
	Prefix   string
	Username string
	Password string
	SPN      string
	// OAuth2 fields would go here
}

// ResolveVariant selects the appropriate auth provider from a variant config.
//
// If no variants are defined, creates a provider from the single Type.
// If Select is not "auto", uses the explicitly named variant.
// If Select is "auto", tries each variant in order and uses the first
// one with valid credentials.
func ResolveVariant(cfg VariantConfig) (Provider, error) {
	if len(cfg.Variants) == 0 {
		return providerFromConfig(cfg.Type, SingleAuthConfig{
			Type:     cfg.Type,
			Token:    "", // filled by caller from resolved config
			Username: "",
			Password: "",
		})
	}

	if cfg.Select != "auto" && cfg.Select != "" {
		variant, ok := cfg.Variants[cfg.Select]
		if !ok {
			return nil, fmt.Errorf("unknown auth variant: %s", cfg.Select)
		}
		return providerFromConfig(variant.Type, variant)
	}

	// Auto: try each variant in declaration order
	for _, name := range cfg.VariantOrder {
		variant := cfg.Variants[name]
		p, err := providerFromConfig(variant.Type, variant)
		if err != nil {
			continue
		}
		if p.Available() {
			return p, nil
		}
	}

	return nil, fmt.Errorf("no auth variant has valid credentials")
}

func providerFromConfig(typeName string, cfg SingleAuthConfig) (Provider, error) {
	switch typeName {
	case "bearer":
		return NewBearerProvider(cfg.Token, cfg.Header, cfg.Prefix), nil
	case "basic":
		return NewBasicProvider(cfg.Username, cfg.Password), nil
	case "kerberos/spnego":
		return NewKerberosProvider(cfg.SPN), nil
	default:
		return nil, fmt.Errorf("unknown auth type: %s", typeName)
	}
}
