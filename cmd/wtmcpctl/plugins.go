package main

import (
	"fmt"
	"os"
	"sort"

	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/lipgloss/table"
	"github.com/charmbracelet/x/term"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/LeGambiArt/wtmcp/internal/plugin"
)

var (
	headerStyle   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("12"))
	borderStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	enabledStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("10"))
	disabledStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
)

type pluginEntry struct {
	Name        string
	Version     string
	Description string
	Enabled     bool
}

var pluginsCmd = &cobra.Command{
	Use:   "plugin",
	Short: "Manage plugin enable/disable state",
	Long: `Manage which plugins are enabled or disabled.

When run without a subcommand, shows an interactive selector where
you can toggle plugins on/off with Space and confirm with Enter.
The selection is saved to config.yaml.`,
	RunE: runPluginsInteractive,
}

var pluginsListCmd = &cobra.Command{
	Use:   "list",
	Short: "List plugins and their enable/disable status",
	Args:  cobra.NoArgs,
	RunE:  runPluginsListCmd,
}

var pluginsEnableCmd = &cobra.Command{
	Use:               "enable [plugin-name...]",
	Short:             "Enable one or more plugins",
	Args:              cobra.MinimumNArgs(1),
	ValidArgsFunction: completeDisabledPlugins,
	RunE:              runPluginsEnable,
}

var pluginsDisableCmd = &cobra.Command{
	Use:               "disable [plugin-name...]",
	Short:             "Disable one or more plugins",
	Args:              cobra.MinimumNArgs(1),
	ValidArgsFunction: completeEnabledPlugins,
	RunE:              runPluginsDisable,
}

var pluginsOnlyCmd = &cobra.Command{
	Use:   "only [plugin-name...]",
	Short: "Set an allowlist of plugins to load",
	Long: `Set an allowlist so only the specified plugins are loaded.
All other plugins will be skipped. Use --clear to remove the
allowlist and return to the default behavior (all plugins loaded
except those in plugins.disabled).`,
	ValidArgsFunction: completeAllPlugins,
	RunE:              runPluginsOnly,
}

func init() {
	pluginsListCmd.Flags().BoolP("plain", "p", false,
		"Plain text output (no colors or borders)")
	pluginsOnlyCmd.Flags().Bool("clear", false,
		"Remove the allowlist, returning to default behavior")

	pluginsCmd.AddCommand(pluginsListCmd, pluginsEnableCmd, pluginsDisableCmd, pluginsOnlyCmd)
}

// getPluginsDiscoveryResult discovers ALL plugins, ignoring the
// plugins.disabled config filter. The enabled/disabled status is
// determined from Config.Plugins.Disabled instead.
func getPluginsDiscoveryResult() (*plugin.DiscoveryResult, error) {
	return plugin.Discover(plugin.DiscoveryOptions{
		WorkdirOverride:     globalWorkdir,
		SkipConfigFiltering: true,
	})
}

func buildPluginEntries(result *plugin.DiscoveryResult) []pluginEntry {
	disabledSet := make(map[string]bool)
	for _, name := range result.Config.Plugins.Disabled {
		disabledSet[name] = true
	}

	var entries []pluginEntry
	for _, m := range result.Manager.Manifests() {
		entries = append(entries, pluginEntry{
			Name: m.Name, Version: m.Version,
			Description: m.Description, Enabled: !disabledSet[m.Name],
		})
	}
	// Sort: disabled first, then alphabetically within each group
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].Enabled != entries[j].Enabled {
			return !entries[i].Enabled // disabled first
		}
		return entries[i].Name < entries[j].Name
	})
	return entries
}

func runPluginsInteractive(_ *cobra.Command, _ []string) error {
	result, err := getPluginsDiscoveryResult()
	if err != nil {
		return err
	}

	entries := buildPluginEntries(result)
	if len(entries) == 0 {
		fmt.Println("No plugins found.")
		return nil
	}

	// Compute max widths for alignment
	maxName, maxVer := 0, 0
	for _, e := range entries {
		if len(e.Name) > maxName {
			maxName = len(e.Name)
		}
		if len(e.Version) > maxVer {
			maxVer = len(e.Version)
		}
	}
	fmtStr := fmt.Sprintf("%%-%ds  v%%-%ds  %%s", maxName, maxVer)

	options := make([]huh.Option[string], len(entries))
	var selected []string
	for i, e := range entries {
		label := fmt.Sprintf(fmtStr, e.Name, e.Version, e.Description)
		options[i] = huh.NewOption(label, e.Name)
		if e.Enabled {
			selected = append(selected, e.Name)
		}
	}
	if err := huh.NewForm(
		huh.NewGroup(
			huh.NewMultiSelect[string]().
				Title("Manage plugins (Space to toggle, Enter to save):").
				Options(options...).
				Value(&selected),
		),
	).Run(); err != nil {
		return err
	}

	// Compute disabled list: all names NOT in selected
	selectedSet := make(map[string]bool, len(selected))
	for _, s := range selected {
		selectedSet[s] = true
	}
	var disabled []string
	for _, e := range entries {
		if !selectedSet[e.Name] {
			disabled = append(disabled, e.Name)
		}
	}
	sort.Strings(disabled)

	if err := updateConfigDisabled(result.ConfigPath, disabled); err != nil {
		return err
	}

	if len(disabled) == 0 {
		fmt.Println("All plugins enabled.")
	} else {
		fmt.Printf("Disabled plugins updated in %s:\n", result.ConfigPath)
		for _, name := range disabled {
			fmt.Printf("  - %s\n", name)
		}
	}
	return nil
}

func runPluginsListCmd(cmd *cobra.Command, _ []string) error {
	plain, _ := cmd.Flags().GetBool("plain")

	result, err := getPluginsDiscoveryResult()
	if err != nil {
		return err
	}

	entries := buildPluginEntries(result)
	if len(entries) == 0 {
		fmt.Println("No plugins found.")
		return nil
	}

	if plain {
		for _, e := range entries {
			status := "enabled"
			if !e.Enabled {
				status = "disabled"
			}
			fmt.Printf("%s\t%s\t%s\t%s\n", e.Name, e.Version, status, e.Description)
		}
		return nil
	}

	data := make([][]string, len(entries))
	for i, e := range entries {
		status := enabledStyle.Render("enabled")
		if !e.Enabled {
			status = disabledStyle.Render("disabled")
		}
		data[i] = []string{e.Name, "v" + e.Version, status, e.Description}
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
		Headers("Plugin", "Version", "Status", "Description").
		Rows(data...)

	fmt.Println(t)
	return nil
}

func runPluginsEnable(_ *cobra.Command, args []string) error {
	for _, name := range args {
		if err := plugin.ValidatePluginName(name); err != nil {
			return fmt.Errorf("invalid plugin name %q: %w", name, err)
		}
	}

	result, err := getPluginsDiscoveryResult()
	if err != nil {
		return err
	}

	// Warn about unknown plugins
	allPlugins := allPluginNames(result)
	for _, name := range args {
		if !allPlugins[name] {
			fmt.Fprintf(os.Stderr, "warning: plugin %q not found in any plugin directory\n", name)
		}
	}

	// Load current disabled list, remove the specified names
	disabled, err := loadCurrentDisabled(result.ConfigPath)
	if err != nil {
		return err
	}

	removeSet := make(map[string]bool, len(args))
	for _, name := range args {
		removeSet[name] = true
	}
	var newDisabled []string
	for _, name := range disabled {
		if !removeSet[name] {
			newDisabled = append(newDisabled, name)
		}
	}

	if err := updateConfigDisabled(result.ConfigPath, newDisabled); err != nil {
		return err
	}

	for _, name := range args {
		fmt.Printf("Enabled plugin: %s\n", name)
	}
	return nil
}

func runPluginsDisable(_ *cobra.Command, args []string) error {
	for _, name := range args {
		if err := plugin.ValidatePluginName(name); err != nil {
			return fmt.Errorf("invalid plugin name %q: %w", name, err)
		}
	}

	result, err := getPluginsDiscoveryResult()
	if err != nil {
		return err
	}

	// Warn about unknown plugins
	allPlugins := allPluginNames(result)
	for _, name := range args {
		if !allPlugins[name] {
			fmt.Fprintf(os.Stderr, "warning: plugin %q not found in any plugin directory\n", name)
		}
	}

	// Load current disabled list, add the specified names
	disabled, err := loadCurrentDisabled(result.ConfigPath)
	if err != nil {
		return err
	}

	disabledSet := make(map[string]bool, len(disabled))
	for _, name := range disabled {
		disabledSet[name] = true
	}
	for _, name := range args {
		if !disabledSet[name] {
			disabled = append(disabled, name)
		}
	}
	sort.Strings(disabled)

	if err := updateConfigDisabled(result.ConfigPath, disabled); err != nil {
		return err
	}

	for _, name := range args {
		fmt.Printf("Disabled plugin: %s\n", name)
	}
	return nil
}

func runPluginsOnly(cmd *cobra.Command, args []string) error {
	clearList, _ := cmd.Flags().GetBool("clear")

	if clearList {
		result, err := getDiscoveryResult()
		if err != nil {
			return err
		}
		if err := updateConfigStringList(result.ConfigPath, "plugins", "enabled", nil); err != nil {
			return err
		}
		fmt.Println("Allowlist cleared: all plugins will be loaded (except disabled).")
		fmt.Println("Restart wtmcp for changes to take effect.")
		return nil
	}

	if len(args) == 0 {
		return fmt.Errorf("specify plugin names or use --clear")
	}

	for _, name := range args {
		if err := plugin.ValidatePluginName(name); err != nil {
			return fmt.Errorf("invalid plugin name %q: %w", name, err)
		}
	}

	result, err := getPluginsDiscoveryResult()
	if err != nil {
		return err
	}

	// Warn about unknown plugins
	allPlugins := allPluginNames(result)
	for _, name := range args {
		if !allPlugins[name] {
			fmt.Fprintf(os.Stderr, "warning: plugin %q not found in any plugin directory\n", name)
		}
	}

	enabled := make([]string, len(args))
	copy(enabled, args)
	sort.Strings(enabled)

	// Set enabled and clear disabled atomically
	if err := updateConfigStringList(result.ConfigPath, "plugins", "enabled", enabled); err != nil {
		return err
	}
	if err := updateConfigStringList(result.ConfigPath, "plugins", "disabled", nil); err != nil {
		return err
	}

	if len(enabled) == 1 {
		fmt.Printf("Allowlist set: only %q will be loaded.\n", enabled[0])
	} else {
		fmt.Printf("Allowlist set: only %d plugins will be loaded.\n", len(enabled))
		for _, name := range enabled {
			fmt.Printf("  - %s\n", name)
		}
	}
	fmt.Println("Restart wtmcp for changes to take effect.")
	return nil
}

func allPluginNames(result *plugin.DiscoveryResult) map[string]bool {
	names := make(map[string]bool)
	for name := range result.Manager.Manifests() {
		names[name] = true
	}
	return names
}

func loadCurrentDisabled(configPath string) ([]string, error) {
	data, err := os.ReadFile(configPath) //nolint:gosec // config file path from user
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var doc map[string]any
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}

	pluginsRaw, ok := doc["plugins"]
	if !ok {
		return nil, nil
	}
	plugins, ok := pluginsRaw.(map[string]any)
	if !ok {
		return nil, nil
	}
	disabledRaw, ok := plugins["disabled"]
	if !ok {
		return nil, nil
	}
	list, ok := disabledRaw.([]any)
	if !ok {
		return nil, nil
	}

	var disabled []string
	for _, item := range list {
		if s, ok := item.(string); ok {
			disabled = append(disabled, s)
		}
	}
	return disabled, nil
}

func updateConfigDisabled(configPath string, disabled []string) error {
	return updateConfigStringList(configPath, "plugins", "disabled", disabled)
}

// completeEnabledPlugins returns currently enabled plugin names for
// the disable subcommand, filtering out names already on the command line.
func completeEnabledPlugins(
	_ *cobra.Command, args []string, _ string,
) ([]string, cobra.ShellCompDirective) {
	result, err := getPluginsDiscoveryResult()
	if err != nil {
		return nil, cobra.ShellCompDirectiveError
	}

	disabledSet := make(map[string]bool)
	for _, name := range result.Config.Plugins.Disabled {
		disabledSet[name] = true
	}

	already := make(map[string]bool, len(args))
	for _, a := range args {
		already[a] = true
	}

	var names []string
	for name := range result.Manager.Manifests() {
		if !already[name] && !disabledSet[name] {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	return names, cobra.ShellCompDirectiveNoFileComp
}

// completeDisabledPlugins returns currently config-disabled plugin names
// for the enable subcommand, filtering out names already on the command line.
func completeDisabledPlugins(
	_ *cobra.Command, args []string, _ string,
) ([]string, cobra.ShellCompDirective) {
	result, err := getPluginsDiscoveryResult()
	if err != nil {
		return nil, cobra.ShellCompDirectiveError
	}

	disabledSet := make(map[string]bool)
	for _, name := range result.Config.Plugins.Disabled {
		disabledSet[name] = true
	}

	already := make(map[string]bool, len(args))
	for _, a := range args {
		already[a] = true
	}

	var names []string
	for name := range result.Manager.Manifests() {
		if !already[name] && disabledSet[name] {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	return names, cobra.ShellCompDirectiveNoFileComp
}

// completeAllPlugins returns all discovered plugin names for the only subcommand.
func completeAllPlugins(
	_ *cobra.Command, args []string, _ string,
) ([]string, cobra.ShellCompDirective) {
	result, err := getPluginsDiscoveryResult()
	if err != nil {
		return nil, cobra.ShellCompDirectiveError
	}

	already := make(map[string]bool, len(args))
	for _, a := range args {
		already[a] = true
	}

	var names []string
	for name := range result.Manager.Manifests() {
		if !already[name] {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	return names, cobra.ShellCompDirectiveNoFileComp
}
