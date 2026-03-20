package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/LeGambiArt/oauth2flow"

	googleauth "github.com/LeGambiArt/wtmcp/internal/google"
)

// OAuthPlugin represents an OAuth plugin configuration.
type OAuthPlugin struct {
	Name            string
	TokenFile       string
	CredentialsFile string
	Scopes          []string
}

var oauthCmd = &cobra.Command{
	Use:   "oauth",
	Short: "Manage OAuth authentication for plugins",
}

var oauthListCmd = &cobra.Command{
	Use:   "list",
	Short: "List OAuth plugins and their authentication status",
	Args:  cobra.NoArgs,
	RunE: func(_ *cobra.Command, _ []string) error {
		return oauthList()
	},
}

var oauthAuthCmd = &cobra.Command{
	Use:               "auth [plugin-name...]",
	Short:             "Authenticate one or more plugins using OAuth",
	ValidArgsFunction: completeOAuthPlugins,
	RunE:              runOAuthAuth,
}

func init() {
	oauthAuthCmd.Flags().BoolP("all", "a", false,
		"Authenticate all non-authenticated plugins")

	oauthCmd.AddCommand(oauthListCmd, oauthAuthCmd)
}

// completeOAuthPlugins returns discovered OAuth plugin names for completion,
// filtering out names already specified on the command line.
func completeOAuthPlugins(
	_ *cobra.Command, args []string, _ string,
) ([]string, cobra.ShellCompDirective) {
	plugins, err := discoverOAuthPlugins()
	if err != nil {
		return nil, cobra.ShellCompDirectiveError
	}

	already := make(map[string]bool, len(args))
	for _, a := range args {
		already[a] = true
	}

	var names []string
	for _, p := range plugins {
		if !already[p.Name] {
			names = append(names, p.Name)
		}
	}
	return names, cobra.ShellCompDirectiveNoFileComp
}

func runOAuthAuth(cmd *cobra.Command, args []string) error {
	authAll, _ := cmd.Flags().GetBool("all")

	if !authAll && len(args) == 0 {
		return fmt.Errorf("requires plugin name(s) or --all flag")
	}
	if authAll && len(args) > 0 {
		return fmt.Errorf("cannot specify both --all flag and plugin names")
	}

	oauthPlugins, err := discoverOAuthPlugins()
	if err != nil {
		return fmt.Errorf("discover plugins: %w", err)
	}

	var pluginsToAuth []string

	if authAll {
		credDir, err := getCredentialsDir()
		if err != nil {
			return fmt.Errorf("cannot determine credentials directory: %w", err)
		}

		for _, plugin := range oauthPlugins {
			if !isAuthenticated(credDir, &plugin) {
				pluginsToAuth = append(pluginsToAuth, plugin.Name)
			}
		}

		if len(pluginsToAuth) == 0 {
			fmt.Println("All plugins are already authenticated.")
			return nil
		}
	} else {
		pluginsToAuth = args
	}

	var failed []string
	for i, pluginName := range pluginsToAuth {
		if i > 0 {
			fmt.Println()
			fmt.Println("---")
			fmt.Println()
		}

		if err := oauthAuth(pluginName, oauthPlugins); err != nil {
			fmt.Fprintf(os.Stderr, "Error authenticating %s: %v\n", pluginName, err)
			failed = append(failed, pluginName)
		}
	}

	if len(pluginsToAuth) > 1 {
		fmt.Println()
		fmt.Println("---")
		fmt.Println()
		fmt.Println("Authentication Summary:")
		fmt.Printf("  Success: %d/%d\n", len(pluginsToAuth)-len(failed), len(pluginsToAuth))
		if len(failed) > 0 {
			fmt.Println("  Failed:")
			for _, name := range failed {
				fmt.Printf("    - %s\n", name)
			}
		}
	}

	if len(failed) > 0 {
		return fmt.Errorf("%d plugin(s) failed to authenticate", len(failed))
	}
	return nil
}

// discoverOAuthPlugins scans the plugins directory and builds a list of OAuth plugins.
func discoverOAuthPlugins() ([]OAuthPlugin, error) {
	manifests, err := discoverPlugins()
	if err != nil {
		return nil, err
	}

	var plugins []OAuthPlugin
	for _, manifest := range manifests {
		// Only include plugins with OAuth2 authentication
		if manifest.Services.Auth.Type == "oauth2" &&
			manifest.Services.Auth.TokenFile != "" &&
			len(manifest.Services.Auth.Scopes) > 0 {
			plugins = append(plugins, OAuthPlugin{
				Name:            manifest.Name,
				TokenFile:       manifest.Services.Auth.TokenFile,
				CredentialsFile: manifest.Services.Auth.CredentialsFile,
				Scopes:          manifest.Services.Auth.Scopes,
			})
		}
	}

	return plugins, nil
}

// isAuthenticated checks if a plugin has a valid or refreshable token.
func isAuthenticated(credDir string, plugin *OAuthPlugin) bool {
	tokenPath := filepath.Join(credDir, plugin.TokenFile)

	if _, err := os.Stat(tokenPath); err != nil {
		return false
	}

	tok, err := googleauth.LoadToken(tokenPath)
	if err != nil {
		return false
	}

	// Consider authenticated if valid or has refresh token
	return tok.Valid() || tok.RefreshToken != ""
}

func oauthList() error {
	plugins, err := discoverOAuthPlugins()
	if err != nil {
		return fmt.Errorf("discover plugins: %w", err)
	}

	if len(plugins) == 0 {
		fmt.Println("No OAuth plugins found.")
		return nil
	}

	credDir, err := getCredentialsDir()
	if err != nil {
		return fmt.Errorf("cannot determine credentials directory: %w", err)
	}

	fmt.Println("OAuth Plugin Status:")
	fmt.Println()

	for _, plugin := range plugins {
		tokenPath := filepath.Join(credDir, plugin.TokenFile)
		status := "✗"
		statusText := "not authenticated"

		if _, err := os.Stat(tokenPath); err == nil {
			// Token file exists, check if it's valid
			tok, loadErr := googleauth.LoadToken(tokenPath)
			switch {
			case loadErr != nil:
				status = "!"
				statusText = "invalid token file"
			case tok.Valid():
				status = "✓"
				statusText = "authenticated (valid)"
			case tok.RefreshToken != "":
				status = "✓"
				statusText = "authenticated (needs refresh)"
			default:
				status = "!"
				statusText = "expired (no refresh token)"
			}
		}

		fmt.Printf("  %s  %-20s  %s\n", status, plugin.Name, statusText)
	}

	fmt.Println()
	fmt.Printf("Credentials directory: %s\n", credDir)
	return nil
}

func oauthAuth(pluginName string, plugins []OAuthPlugin) error {
	// Find the plugin
	var plugin *OAuthPlugin
	for i := range plugins {
		if plugins[i].Name == pluginName {
			plugin = &plugins[i]
			break
		}
	}

	if plugin == nil {
		return fmt.Errorf("unknown plugin: %s", pluginName)
	}

	credDir, err := getCredentialsDir()
	if err != nil {
		return fmt.Errorf("cannot determine credentials directory: %w", err)
	}

	clientCredsPath := filepath.Join(credDir, plugin.CredentialsFile)
	tokenPath := filepath.Join(credDir, plugin.TokenFile)

	// Check if client credentials exist
	if _, err := os.Stat(clientCredsPath); err != nil {
		return fmt.Errorf("client credentials not found at %s\nPlease ensure you have set up OAuth credentials", clientCredsPath)
	}

	fmt.Printf("Authenticating plugin: %s\n", plugin.Name)
	fmt.Println()
	fmt.Println("Starting OAuth2 flow...")
	fmt.Println("Your browser will open automatically for authorization.")
	fmt.Println()

	// Use oauth2flow to handle the complete OAuth2 flow
	token, err := oauth2flow.Run(context.Background(), oauth2flow.Config{
		ClientCredentialsFile: clientCredsPath,
		TokenFile:             tokenPath,
		Scopes:                plugin.Scopes,
	})
	if err != nil {
		return fmt.Errorf("oauth2 flow failed: %w", err)
	}

	// Verify token was saved
	if token == nil {
		return fmt.Errorf("oauth2 flow completed but no token received")
	}

	fmt.Println()
	fmt.Printf("✓ Successfully authenticated %s\n", plugin.Name)
	fmt.Printf("Token saved to: %s\n", tokenPath)
	return nil
}
