package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWorkDir(t *testing.T) {
	// Default
	t.Setenv("WHAT_THE_MCP_WORKDIR", "")
	dir := WorkDir()
	if dir == "" || dir == "." {
		t.Error("default workdir should resolve to home-based path")
	}

	// Override
	t.Setenv("WHAT_THE_MCP_WORKDIR", "/custom/path")
	dir = WorkDir()
	if dir != "/custom/path" {
		t.Errorf("WorkDir() = %q, want /custom/path", dir)
	}
}

func TestLoadDotEnvMainFile(t *testing.T) {
	dir := t.TempDir()

	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte("TEST_MAIN_VAR=main_value\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	// Clear so it can be set
	t.Setenv("TEST_MAIN_VAR", "")
	_ = os.Unsetenv("TEST_MAIN_VAR")

	if err := LoadDotEnv(dir); err != nil {
		t.Fatalf("LoadDotEnv: %v", err)
	}

	if got := os.Getenv("TEST_MAIN_VAR"); got != "main_value" {
		t.Errorf("TEST_MAIN_VAR = %q, want main_value", got)
	}
}

func TestLoadDotEnvDir(t *testing.T) {
	dir := t.TempDir()
	envDir := filepath.Join(dir, "env.d")
	if err := os.MkdirAll(envDir, 0o750); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(envDir, "aaa.env"), []byte("TEST_AAA=aaa_val\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(envDir, "bbb.env"), []byte("TEST_BBB=bbb_val\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	// Non-.env files should be ignored
	if err := os.WriteFile(filepath.Join(envDir, "skip.txt"), []byte("TEST_SKIP=nope\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	_ = os.Unsetenv("TEST_AAA")
	_ = os.Unsetenv("TEST_BBB")
	_ = os.Unsetenv("TEST_SKIP")

	if err := LoadDotEnv(dir); err != nil {
		t.Fatalf("LoadDotEnv: %v", err)
	}

	if os.Getenv("TEST_AAA") != "aaa_val" {
		t.Errorf("TEST_AAA = %q", os.Getenv("TEST_AAA"))
	}
	if os.Getenv("TEST_BBB") != "bbb_val" {
		t.Errorf("TEST_BBB = %q", os.Getenv("TEST_BBB"))
	}
	if os.Getenv("TEST_SKIP") != "" {
		t.Error("skip.txt should not have been loaded")
	}
}

func TestLoadDotEnvNoOverwrite(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte("TEST_EXISTING=from_file\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	// Set it first — should NOT be overwritten
	t.Setenv("TEST_EXISTING", "from_shell")

	if err := LoadDotEnv(dir); err != nil {
		t.Fatal(err)
	}

	if got := os.Getenv("TEST_EXISTING"); got != "from_shell" {
		t.Errorf("TEST_EXISTING = %q, want from_shell (env should not be overwritten)", got)
	}
}

func TestLoadDotEnvMissingDir(t *testing.T) {
	// Should not error on missing workdir
	if err := LoadDotEnv("/nonexistent/path"); err != nil {
		t.Errorf("should not error on missing dir: %v", err)
	}
}

func TestLoadEnvFile(t *testing.T) {
	dir := t.TempDir()
	envFile := filepath.Join(dir, "test.env")

	content := `
# Comment line
PLAIN_VAR=plain_value
QUOTED_VAR="quoted value"
SINGLE_QUOTED='single quoted'
export EXPORTED_VAR=exported_value
  SPACED_VAR = spaced_value

EMPTY_LINE_ABOVE=yes
`
	if err := os.WriteFile(envFile, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	for _, key := range []string{"PLAIN_VAR", "QUOTED_VAR", "SINGLE_QUOTED", "EXPORTED_VAR", "SPACED_VAR", "EMPTY_LINE_ABOVE"} {
		_ = os.Unsetenv(key)
	}

	if err := loadEnvFile(envFile); err != nil {
		t.Fatal(err)
	}

	tests := map[string]string{
		"PLAIN_VAR":        "plain_value",
		"QUOTED_VAR":       "quoted value",
		"SINGLE_QUOTED":    "single quoted",
		"EXPORTED_VAR":     "exported_value",
		"SPACED_VAR":       "spaced_value",
		"EMPTY_LINE_ABOVE": "yes",
	}

	for key, want := range tests {
		if got := os.Getenv(key); got != want {
			t.Errorf("%s = %q, want %q", key, got, want)
		}
	}
}

func TestPaths(t *testing.T) {
	p := Paths("/opt/wtmcp")

	if p.ConfigFile != "/opt/wtmcp/config.yaml" {
		t.Errorf("ConfigFile = %q", p.ConfigFile)
	}
	if p.EnvFile != "/opt/wtmcp/.env" {
		t.Errorf("EnvFile = %q", p.EnvFile)
	}
	if p.EnvDir != "/opt/wtmcp/env.d" {
		t.Errorf("EnvDir = %q", p.EnvDir)
	}
	if p.CredentialsDir != "/opt/wtmcp/credentials" {
		t.Errorf("CredentialsDir = %q", p.CredentialsDir)
	}
	if p.PluginsDir != "/opt/wtmcp/plugins" {
		t.Errorf("PluginsDir = %q", p.PluginsDir)
	}
	if p.CacheDir != "/opt/wtmcp/cache" {
		t.Errorf("CacheDir = %q", p.CacheDir)
	}
}
