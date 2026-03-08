// what-the-mcp is an MCP server with a language-agnostic plugin protocol.
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	mcpserver "github.com/mark3labs/mcp-go/server"

	"gitlab.cee.redhat.com/bragctl/what-the-mcp/internal/auth"
	"gitlab.cee.redhat.com/bragctl/what-the-mcp/internal/cache"
	"gitlab.cee.redhat.com/bragctl/what-the-mcp/internal/config"
	"gitlab.cee.redhat.com/bragctl/what-the-mcp/internal/plugin"
	"gitlab.cee.redhat.com/bragctl/what-the-mcp/internal/proxy"
	"gitlab.cee.redhat.com/bragctl/what-the-mcp/internal/server"
)

// Version and BuildDate are set via ldflags at build time.
var (
	Version   = "dev"
	BuildDate = "unknown"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "version" {
		fmt.Printf("what-the-mcp %s (built %s)\n", Version, BuildDate)
		return
	}
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	cfg := config.DefaultConfig()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	authReg := auth.NewRegistry()
	cacheStore := cache.NewMemoryStore()
	httpProxy := proxy.New(nil, cfg.Plugins.MaxMessageSize)

	mgr := plugin.NewManager(authReg, httpProxy, cacheStore, cfg)

	pluginDirs := cfg.PluginDirs
	if dir := defaultUserPluginDir(); dir != "" {
		pluginDirs = append(pluginDirs, dir)
	}
	if err := mgr.Discover(pluginDirs); err != nil {
		return fmt.Errorf("plugin discovery: %w", err)
	}

	if err := mgr.LoadAll(ctx); err != nil {
		return fmt.Errorf("plugin loading: %w", err)
	}

	go func() {
		<-ctx.Done()
		log.Println("shutting down plugins...")
		mgr.ShutdownAll(context.Background())
	}()

	srv := server.New(Version, mgr)
	log.Printf("what-the-mcp %s starting (stdio)", Version)
	return mcpserver.ServeStdio(srv)
}

func defaultUserPluginDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return home + "/.config/what-the-mcp/plugins"
}
