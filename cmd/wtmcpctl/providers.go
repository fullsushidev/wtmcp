package main

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/lipgloss/table"
	"github.com/charmbracelet/x/term"
	"github.com/spf13/cobra"

	"github.com/LeGambiArt/wtmcp/internal/auth"
)

type providerEntry struct {
	Name   string   `json:"name"`
	Status string   `json:"status"`
	UsedBy []string `json:"used_by"`
}

var providerCmd = &cobra.Command{
	Use:   "provider",
	Short: "Manage auth provider enable/disable state",
	Long: `Manage which auth providers are enabled or disabled.

Disabling a provider cascades to plugins that depend on it:
  - Single-type plugins using the provider are disabled
  - Variant-based plugins lose the disabled variant
  - Plugins with explicit variant selection are disabled entirely
    (to prevent silent auth downgrade)

Changes take effect on the next wtmcp restart.`,
}

var providerListCmd = &cobra.Command{
	Use:   "list",
	Short: "List auth providers and their status",
	Args:  cobra.NoArgs,
	RunE:  runProviderList,
}

var providerEnableCmd = &cobra.Command{
	Use:               "enable <provider...>",
	Short:             "Enable one or more auth providers",
	Args:              cobra.MinimumNArgs(1),
	ValidArgsFunction: completeDisabledProviders,
	RunE:              runProviderEnable,
}

var providerDisableCmd = &cobra.Command{
	Use:               "disable <provider...>",
	Short:             "Disable one or more auth providers",
	Args:              cobra.MinimumNArgs(1),
	ValidArgsFunction: completeEnabledProviders,
	RunE:              runProviderDisable,
}

func init() {
	providerListCmd.Flags().Bool("json", false, "JSON output")
	providerListCmd.Flags().BoolP("plain", "p", false, "Plain text output (no colors or borders)")

	providerCmd.AddCommand(providerListCmd, providerEnableCmd, providerDisableCmd)
}

func validateProviderNames(names []string) error {
	for _, name := range names {
		if name == "kerberos" {
			return fmt.Errorf("invalid provider name %q; did you mean %q?", name, "kerberos/spnego")
		}
		if !auth.IsKnownProviderType(name) {
			return fmt.Errorf("unknown provider %q; valid providers: %v", name, auth.KnownProviderTypes)
		}
	}
	return nil
}

// findProviderUsage scans all discovered plugin manifests and returns
// a map of provider type → list of "plugin (variant)" usage strings.
func findProviderUsage() (map[string][]string, error) {
	result, err := getPluginsDiscoveryResult()
	if err != nil {
		return nil, err
	}

	usage := make(map[string][]string)
	for _, m := range result.Manager.Manifests() {
		authCfg := m.Services.Auth
		if authCfg.Type == "" && len(authCfg.Variants) == 0 {
			continue
		}
		if len(authCfg.Variants) > 0 {
			for _, vName := range authCfg.VariantOrder {
				v := authCfg.Variants[vName]
				normalized := auth.NormalizeProviderType(v.Type)
				usage[normalized] = append(usage[normalized],
					fmt.Sprintf("%s (%s)", m.Name, vName))
			}
		} else {
			normalized := auth.NormalizeProviderType(authCfg.Type)
			usage[normalized] = append(usage[normalized], m.Name)
		}
	}
	return usage, nil
}

func buildProviderEntries(disabledSet map[string]bool, usage map[string][]string) []providerEntry {
	entries := make([]providerEntry, 0, len(auth.KnownProviderTypes))
	for _, name := range auth.KnownProviderTypes {
		status := "enabled"
		if disabledSet[name] {
			status = "disabled"
		}
		usedBy := usage[name]
		if usedBy == nil {
			usedBy = []string{}
		}
		entries = append(entries, providerEntry{
			Name:   name,
			Status: status,
			UsedBy: usedBy,
		})
	}
	return entries
}

func runProviderList(cmd *cobra.Command, _ []string) error {
	jsonOutput, _ := cmd.Flags().GetBool("json")
	plain, _ := cmd.Flags().GetBool("plain")

	result, err := getDiscoveryResult()
	if err != nil {
		return err
	}

	disabledSet := make(map[string]bool)
	for _, name := range result.Config.Providers.Disabled {
		disabledSet[name] = true
	}

	usage, err := findProviderUsage()
	if err != nil {
		return err
	}

	entries := buildProviderEntries(disabledSet, usage)

	if jsonOutput {
		data, _ := json.MarshalIndent(entries, "", "  ")
		fmt.Println(string(data))
		return nil
	}

	if plain {
		for _, e := range entries {
			usedBy := "—"
			if len(e.UsedBy) > 0 {
				usedBy = strings.Join(e.UsedBy, ", ")
			}
			fmt.Printf("%s\t%s\t%s\n", e.Name, e.Status, usedBy)
		}
		return nil
	}

	data := make([][]string, len(entries))
	for i, e := range entries {
		status := enabledStyle.Render("enabled")
		if e.Status == "disabled" {
			status = disabledStyle.Render("disabled")
		}
		usedBy := disabledStyle.Render("—")
		if len(e.UsedBy) > 0 {
			usedBy = strings.Join(e.UsedBy, ", ")
		}
		data[i] = []string{e.Name, status, usedBy}
	}

	w, _, _ := term.GetSize(os.Stdout.Fd())
	if w <= 0 {
		w = 80
	}

	t := table.New().
		Width(w).
		Border(lipgloss.RoundedBorder()).
		BorderStyle(borderStyle).
		StyleFunc(func(row, _ int) lipgloss.Style {
			if row == table.HeaderRow {
				return headerStyle
			}
			return lipgloss.NewStyle()
		}).
		Headers("Provider", "Status", "Used By").
		Rows(data...)

	fmt.Println(t)
	return nil
}

func runProviderEnable(_ *cobra.Command, args []string) error {
	if err := validateProviderNames(args); err != nil {
		return err
	}

	result, err := getDiscoveryResult()
	if err != nil {
		return err
	}

	disabled := make([]string, 0)
	for _, name := range result.Config.Providers.Disabled {
		if !contains(args, name) {
			disabled = append(disabled, name)
		}
	}
	sort.Strings(disabled)

	if err := updateConfigStringList(result.ConfigPath, "providers", "disabled", disabled); err != nil {
		return err
	}

	for _, name := range args {
		fmt.Printf("Enabled provider: %s\n", name)
	}
	fmt.Println("\nRestart wtmcp for changes to take effect.")
	return nil
}

func runProviderDisable(_ *cobra.Command, args []string) error {
	if err := validateProviderNames(args); err != nil {
		return err
	}

	result, err := getDiscoveryResult()
	if err != nil {
		return err
	}

	disabledSet := make(map[string]bool)
	for _, name := range result.Config.Providers.Disabled {
		disabledSet[name] = true
	}
	for _, name := range args {
		disabledSet[name] = true
	}
	disabled := make([]string, 0, len(disabledSet))
	for name := range disabledSet {
		disabled = append(disabled, name)
	}
	sort.Strings(disabled)

	if err := updateConfigStringList(result.ConfigPath, "providers", "disabled", disabled); err != nil {
		return err
	}

	for _, name := range args {
		fmt.Printf("Disabled provider: %s\n", name)
	}

	// Show affected plugins
	usage, err := findProviderUsage()
	if err == nil {
		for _, name := range args {
			if plugins, ok := usage[name]; ok && len(plugins) > 0 {
				fmt.Printf("\nAffected plugins for %s:\n", name)
				for _, p := range plugins {
					fmt.Printf("  - %s\n", p)
				}
			}
		}
	}

	fmt.Println("\nRestart wtmcp for changes to take effect.")
	return nil
}

func contains(slice []string, s string) bool {
	for _, item := range slice {
		if item == s {
			return true
		}
	}
	return false
}

// completeEnabledProviders returns currently enabled provider names.
func completeEnabledProviders(
	_ *cobra.Command, args []string, _ string,
) ([]string, cobra.ShellCompDirective) {
	result, err := getDiscoveryResult()
	if err != nil {
		return nil, cobra.ShellCompDirectiveError
	}

	disabledSet := make(map[string]bool)
	for _, name := range result.Config.Providers.Disabled {
		disabledSet[name] = true
	}

	already := make(map[string]bool, len(args))
	for _, a := range args {
		already[a] = true
	}

	var names []string
	for _, name := range auth.KnownProviderTypes {
		if !already[name] && !disabledSet[name] {
			names = append(names, name)
		}
	}
	return names, cobra.ShellCompDirectiveNoFileComp
}

// completeDisabledProviders returns currently disabled provider names.
func completeDisabledProviders(
	_ *cobra.Command, args []string, _ string,
) ([]string, cobra.ShellCompDirective) {
	result, err := getDiscoveryResult()
	if err != nil {
		return nil, cobra.ShellCompDirectiveError
	}

	disabledSet := make(map[string]bool)
	for _, name := range result.Config.Providers.Disabled {
		disabledSet[name] = true
	}

	already := make(map[string]bool, len(args))
	for _, a := range args {
		already[a] = true
	}

	var names []string
	for _, name := range auth.KnownProviderTypes {
		if !already[name] && disabledSet[name] {
			names = append(names, name)
		}
	}
	return names, cobra.ShellCompDirectiveNoFileComp
}
