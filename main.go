// what-the-mcp is an MCP server with a language-agnostic plugin protocol.
package main

import "fmt"

// Version and BuildDate are set via ldflags at build time.
var (
	Version   = "dev"
	BuildDate = "unknown"
)

func main() {
	fmt.Printf("what-the-mcp %s (built %s)\n", Version, BuildDate)
}
