package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/lipgloss/table"
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
	Use:   "plugins",
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
	RunE: func(_ *cobra.Command, _ []string) error {
		return runPluginsList()
	},
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

func init() {
	pluginsCmd.AddCommand(pluginsListCmd, pluginsEnableCmd, pluginsDisableCmd)
}

func buildPluginEntries(result *plugin.DiscoveryResult) []pluginEntry {
	var entries []pluginEntry
	for _, m := range result.Manager.Manifests() {
		entries = append(entries, pluginEntry{
			Name: m.Name, Version: m.Version,
			Description: m.Description, Enabled: true,
		})
	}
	for _, m := range result.Manager.ConfigDisabledPlugins() {
		entries = append(entries, pluginEntry{
			Name: m.Name, Version: m.Version,
			Description: m.Description, Enabled: false,
		})
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Name < entries[j].Name
	})
	return entries
}

func runPluginsInteractive(_ *cobra.Command, _ []string) error {
	result, err := getDiscoveryResult()
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
	for i, e := range entries {
		label := fmt.Sprintf(fmtStr, e.Name, e.Version, e.Description)
		options[i] = huh.NewOption(label, e.Name).Selected(e.Enabled)
	}

	var selected []string
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

func runPluginsList() error {
	result, err := getDiscoveryResult()
	if err != nil {
		return err
	}

	entries := buildPluginEntries(result)
	if len(entries) == 0 {
		fmt.Println("No plugins found.")
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

	t := table.New().
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

	result, err := getDiscoveryResult()
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

	result, err := getDiscoveryResult()
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

func allPluginNames(result *plugin.DiscoveryResult) map[string]bool {
	names := make(map[string]bool)
	for name := range result.Manager.Manifests() {
		names[name] = true
	}
	for name := range result.Manager.ConfigDisabledPlugins() {
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
	data, err := os.ReadFile(configPath) //nolint:gosec // config file path from user
	if err != nil && !os.IsNotExist(err) {
		return err
	}

	var doc map[string]any
	if len(data) > 0 {
		if err := yaml.Unmarshal(data, &doc); err != nil {
			return fmt.Errorf("parse config: %w", err)
		}
	}
	if doc == nil {
		doc = make(map[string]any)
	}

	pluginsRaw, ok := doc["plugins"]
	var plugins map[string]any
	if ok {
		plugins, _ = pluginsRaw.(map[string]any)
	}
	if plugins == nil {
		plugins = make(map[string]any)
	}

	if len(disabled) > 0 {
		plugins["disabled"] = disabled
	} else {
		delete(plugins, "disabled")
	}

	if len(plugins) > 0 {
		doc["plugins"] = plugins
	} else {
		delete(doc, "plugins")
	}

	out, err := yaml.Marshal(doc)
	if err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(configPath), 0o700); err != nil {
		return fmt.Errorf("create config directory: %w", err)
	}

	// Atomic write: temp file + rename
	tmp, err := os.CreateTemp(filepath.Dir(configPath), ".config-*.yaml.tmp")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	tmpName := tmp.Name()

	if _, err := tmp.Write(out); err != nil {
		tmp.Close()        //nolint:errcheck,gosec // closing before cleanup
		os.Remove(tmpName) //nolint:errcheck,gosec // best-effort cleanup
		return fmt.Errorf("write temp file: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()        //nolint:errcheck,gosec // closing before cleanup
		os.Remove(tmpName) //nolint:errcheck,gosec // best-effort cleanup
		return fmt.Errorf("sync temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName) //nolint:errcheck,gosec // best-effort cleanup
		return fmt.Errorf("close temp file: %w", err)
	}

	if err := os.Chmod(tmpName, 0o600); err != nil {
		os.Remove(tmpName) //nolint:errcheck,gosec // best-effort cleanup
		return fmt.Errorf("chmod temp file: %w", err)
	}

	if err := os.Rename(tmpName, configPath); err != nil {
		os.Remove(tmpName) //nolint:errcheck,gosec // best-effort cleanup
		return fmt.Errorf("rename config file: %w", err)
	}

	return nil
}

// completeEnabledPlugins returns currently enabled plugin names for
// the disable subcommand, filtering out names already on the command line.
func completeEnabledPlugins(
	_ *cobra.Command, args []string, _ string,
) ([]string, cobra.ShellCompDirective) {
	result, err := getDiscoveryResult()
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

// completeDisabledPlugins returns currently config-disabled plugin names
// for the enable subcommand, filtering out names already on the command line.
func completeDisabledPlugins(
	_ *cobra.Command, args []string, _ string,
) ([]string, cobra.ShellCompDirective) {
	result, err := getDiscoveryResult()
	if err != nil {
		return nil, cobra.ShellCompDirectiveError
	}

	already := make(map[string]bool, len(args))
	for _, a := range args {
		already[a] = true
	}

	var names []string
	for name := range result.Manager.ConfigDisabledPlugins() {
		if !already[name] {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	return names, cobra.ShellCompDirectiveNoFileComp
}
