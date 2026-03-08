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
	// Parse flags
	configPath := ""
	workdirOverride := ""
	for i, arg := range os.Args[1:] {
		if arg == "--config" && i+2 < len(os.Args) {
			configPath = os.Args[i+2]
		}
		if arg == "--workdir" && i+2 < len(os.Args) {
			workdirOverride = os.Args[i+2]
		}
	}

	// Resolve workdir
	workdir := config.WorkDir()
	if workdirOverride != "" {
		workdir = workdirOverride
	}

	// Load .env files before anything else
	if err := config.LoadDotEnv(workdir); err != nil {
		return fmt.Errorf("load env: %w", err)
	}

	// Load config (uses workdir for defaults)
	cfg, err := config.Load(configPath, workdir)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	authReg := auth.NewRegistry()

	// Initialize Kerberos if available
	if err := auth.InitKerberos(); err != nil {
		log.Printf("kerberos not available: %v", err)
	} else if auth.KerberosAvailable() {
		defer auth.CloseKerberos()
		log.Println("kerberos/spnego auth available")
	}

	cacheStore := cache.NewMemoryStore()
	httpProxy := proxy.New(nil, cfg.Plugins.MaxMessageSize)

	mgr := plugin.NewManager(authReg, httpProxy, cacheStore, cfg)

	if err := mgr.Discover(cfg.PluginDirs); err != nil {
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

	srv := server.New(Version, mgr, cfg)
	log.Printf("what-the-mcp %s starting (workdir: %s)", Version, workdir)
	return mcpserver.ServeStdio(srv)
}
