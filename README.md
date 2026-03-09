# what-the-mcp

MCP server with a language-agnostic plugin system. Plugins are simple
executables (Python, bash, or any language) that communicate with the
core over JSON-lines on stdin/stdout. The core handles auth, HTTP
proxying, caching, and output encoding so plugins stay minimal.

## Architecture

```
┌─────────────────────────────────────────────────┐
│  what-the-mcp (Go)                              │
│                                                 │
│  MCP Server ─── Plugin Manager ─── HTTP Proxy   │
│  (mcp-go)       Discovery         Auth inject   │
│                 Lifecycle          TLS verify   │
│                 Dispatch           Rate limit   │
│                                                 │
│              Cache Store ─── Auth Providers     │
│              (memory/fs)     Bearer, Basic,     │
│                              Kerberos, OAuth2   │
└────────┬──────────────────────────┬─────────────┘
         │ stdio (MCP/JSON-RPC)     │ stdin/stdout (JSON-lines)
    ┌────┴────┐               ┌─────┴──────────┐
    │ AI      │               │ Plugins        │
    │ Client  │               │ Zero deps      │
    └─────────┘               │ No HTTP libs   │
                              │ No auth code   │
                              └────────────────┘
```

## Features

- **Plugin protocol**: JSON-lines over stdin/stdout, any language
- **Auth**: Bearer, Basic, Kerberos/SPNEGO, OAuth2 with token refresh,
  auto-detection from available credentials
- **HTTP proxy**: Auth injection, domain validation, TLS enforcement,
  binary response encoding, multipart upload support
- **Cache**: In-memory store with namespace isolation and TTL
- **Output**: TOON encoding for ~40% token savings (optional)
- **Plugin setup**: Manifest-declared wizard metadata for CLI tooling

## Project Layout

```
cmd/                    Entry point
internal/
  auth/                 Auth providers (bearer, basic, kerberos, oauth2)
  cache/                Key-value cache with TTL
  config/               Env var resolution, YAML config
  encoding/             TOON output encoding
  plugin/               Manager, manifest, transport, dispatch
  protocol/             Wire protocol message types
  proxy/                HTTP proxy with auth injection
  server/               MCP server integration
plugins/
  jira/                 Jira plugin (Python, zero external deps)
tests/
  plugins/jira/         Plugin unit tests
staging/                Dev/test files (gitignored)
```

## Building

```bash
make build
```

The binary gets version and build date via ldflags.

## Running

```bash
# With a workdir containing plugins and env config
./what-the-mcp --workdir ~/.bragctl

# Or with explicit config
./what-the-mcp --config config.yaml --workdir /path/to/workdir
```

The workdir layout:

```
~/.bragctl/
  config.yaml           Core config (optional)
  .env                  Environment variables
  env.d/*.env           Additional env files
  plugins/
    jira/
      plugin.yaml       Plugin manifest
      handler.py        Plugin executable
```

## Plugin Protocol

Plugins are directories with a `plugin.yaml` manifest and a handler
executable. The core starts the handler as a child process and
communicates over stdin/stdout using JSON-lines.

```yaml
# plugin.yaml
name: jira
version: "0.2.0"
execution: persistent
handler: ./handler.py

services:
  auth:
    type: bearer
    token: "${JIRA_TOKEN}"
  http:
    base_url: "${JIRA_URL}"

tools:
  - name: jira_search
    description: "Search Jira issues using JQL"
    params:
      jql:
        type: string
        required: true
```

The handler uses simple read/write loops:

```python
# handler.py — zero dependencies beyond stdlib
import json, sys

def http(method, path, query=None):
    """All HTTP goes through the core proxy (auth injected)."""
    msg = {"id": "1", "type": "http_request", "method": method, "path": path}
    if query:
        msg["query"] = query
    print(json.dumps(msg), flush=True)
    return json.loads(sys.stdin.readline())
```

See `design/plugin-protocol.md` for the full specification.

## Jira Plugin

Tools across read, write, sprint, and export operations:

| Category | Examples |
|----------|---------|
| Read | `jira_search`, `jira_get_myself`, `jira_get_transitions` |
| Write | `jira_create_issue`, `jira_add_comment`, `jira_assign_issue` |
| Sprint | `jira_list_available_sprints`, `jira_get_sprint_issues` |
| Export | `jira_export_sprint_data`, `jira_download_attachment` |

All write tools default to `dry_run=true`. Cloud-aware (ADF format,
accountId assignments). Auth variants: Cloud Basic, Server Bearer,
Server Kerberos.

## Testing

```bash
# Go tests
go test ./...

# Python plugin tests
.venv/bin/pytest tests/ -v

# All pre-commit checks
pre-commit run --all-files
```

Pre-commit hooks: trailing whitespace, YAML, golangci-lint, gofmt,
go vet, go test, ruff lint, ruff format, ty type check, pytest.

## Development

```bash
# Create Python venv for plugin tooling
uv venv .venv
uv pip install ruff ty pytest

# Build and run e2e test
make build
bash staging/e2e_test.sh

# Live Jira integration test (requires credentials)
source staging/env.d/jira.env
TEST_ISSUE_KEY=PROJ-123 python3 staging/tests/live_jira_test.py
```

## License

This project is licensed under the GNU General Public License v3.0.
See [LICENSE](LICENSE) for the full text.
