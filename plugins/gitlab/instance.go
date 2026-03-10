package main

import (
	"crypto/tls"
	"fmt"
	"net/http"
	"os"
	"strings"

	gogitlab "gitlab.com/gitlab-org/api/client-go"
)

// instance holds a named GitLab client.
type instance struct {
	Name   string
	URL    string
	Client *gogitlab.Client
}

// instances maps instance names to their clients.
var instances map[string]*instance

// defaultInstance is used when only one instance is configured
// or when no instance param is provided.
var defaultInstance string

// discoverInstances scans environment variables for GitLab instances.
//
// Multi-instance: GITLAB_{NAME}_TOKEN + GITLAB_{NAME}_URL
// Legacy single: GITLAB_TOKEN + GITLAB_URL
func discoverInstances() error {
	instances = make(map[string]*instance)
	defaultInstance = ""

	// Scan for multi-instance pattern: GITLAB_{NAME}_TOKEN
	for _, env := range os.Environ() {
		key, value, ok := strings.Cut(env, "=")
		if !ok || value == "" {
			continue
		}
		if !strings.HasPrefix(key, "GITLAB_") || !strings.HasSuffix(key, "_TOKEN") {
			continue
		}
		if key == "GITLAB_TOKEN" || key == "GITLAB_SSL_VERIFY" {
			continue
		}

		// GITLAB_INTERNAL_TOKEN → "internal"
		name := strings.ToLower(key[7 : len(key)-6])
		if name == "" {
			continue
		}

		urlKey := fmt.Sprintf("GITLAB_%s_URL", strings.ToUpper(name))
		url := os.Getenv(urlKey)
		if url == "" {
			url = "https://gitlab.com"
		}

		client, err := newClient(url, value, name)
		if err != nil {
			return fmt.Errorf("instance %s: %w", name, err)
		}
		instances[name] = &instance{Name: name, URL: url, Client: client}
	}

	// Legacy fallback: GITLAB_TOKEN
	if len(instances) == 0 {
		token := os.Getenv("GITLAB_TOKEN")
		if token == "" {
			return fmt.Errorf("no GitLab instances configured (set GITLAB_TOKEN or GITLAB_{NAME}_TOKEN)")
		}
		url := os.Getenv("GITLAB_URL")
		if url == "" {
			url = "https://gitlab.com"
		}
		client, err := newClient(url, token, "")
		if err != nil {
			return fmt.Errorf("gitlab: %w", err)
		}
		instances["default"] = &instance{Name: "default", URL: url, Client: client}
	}

	// Set default instance
	if len(instances) == 1 {
		for name := range instances {
			defaultInstance = name
		}
	}

	return nil
}

// resolveInstance returns the client for the given instance name.
// If name is empty, returns the default (only works with single instance).
func resolveInstance(name string) (*gogitlab.Client, error) {
	if name == "" {
		if defaultInstance == "" {
			names := make([]string, 0, len(instances))
			for n := range instances {
				names = append(names, n)
			}
			return nil, fmt.Errorf("instance is required (available: %s)", strings.Join(names, ", "))
		}
		name = defaultInstance
	}

	inst, ok := instances[name]
	if !ok {
		names := make([]string, 0, len(instances))
		for n := range instances {
			names = append(names, n)
		}
		return nil, fmt.Errorf("unknown instance %q (available: %s)", name, strings.Join(names, ", "))
	}
	return inst.Client, nil
}

func newClient(url, token, instanceName string) (*gogitlab.Client, error) {
	opts := []gogitlab.ClientOptionFunc{
		gogitlab.WithBaseURL(url),
	}

	// Check SSL verify settings
	if !sslVerify(instanceName) {
		opts = append(opts, gogitlab.WithHTTPClient(&http.Client{
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12, InsecureSkipVerify: true}, //nolint:gosec // user-configured SSL skip
			},
		}))
	}

	return gogitlab.NewClient(token, opts...)
}

func sslVerify(instanceName string) bool {
	// Per-instance: GITLAB_{NAME}_SSL_VERIFY
	if instanceName != "" {
		if v := os.Getenv(fmt.Sprintf("GITLAB_%s_SSL_VERIFY", strings.ToUpper(instanceName))); v != "" {
			return !strings.EqualFold(v, "false")
		}
	}
	// Global: GITLAB_SSL_VERIFY
	if v := os.Getenv("GITLAB_SSL_VERIFY"); v != "" {
		return !strings.EqualFold(v, "false")
	}
	return true
}
