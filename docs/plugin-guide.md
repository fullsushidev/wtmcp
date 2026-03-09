# Plugin Development Guide

This guide covers everything you need to write a what-the-mcp plugin.
Plugins can be written in any language — the protocol is JSON-lines
over stdin/stdout.

## Plugin Structure

A plugin is a directory containing:

```
my-plugin/
  plugin.yaml       # Required: manifest declaring tools and services
  handler.py        # Required: executable that handles tool calls
  context.md        # Optional: instructions for AI assistants
```

## Manifest (plugin.yaml)

The manifest declares what the plugin does, what tools it exposes,
and what services it needs from the core.

```yaml
name: my-plugin
version: "1.0.0"
description: "What this plugin does"

# "oneshot" runs handler once per call, "persistent" keeps it running
execution: persistent
handler: ./handler.py

# Services the core provides to this plugin
services:
  auth:
    type: bearer
    token: "${MY_TOKEN}"
  http:
    base_url: "${MY_API_URL}"
  cache:
    enabled: true
    default_ttl: 300

# Config values passed to the handler (env vars resolved at load time)
config:
  api_url: "${MY_API_URL}"

# Tool declarations (registered as MCP tools)
tools:
  - name: my_tool
    description: "What this tool does"
    params:
      query:
        type: string
        required: true
        description: "Search query"
      limit:
        type: integer
        default: 10

enabled: true
priority: 50
```

### Parameter Types

Tools declare parameters with JSON Schema types:

| Type | Description |
|------|-------------|
| `string` | Text value |
| `integer` | Whole number |
| `number` | Float/integer |
| `boolean` | true/false |
| `array` | List (use `items.type` for element type) |

### Auth Variants

For plugins that support multiple auth methods:

```yaml
services:
  auth:
    select: "${AUTH_TYPE:-auto}"
    variants:
      cloud:
        type: basic
        username: "${EMAIL}"
        password: "${TOKEN}"
      server:
        type: bearer
        token: "${TOKEN}"
      kerberos:
        type: kerberos/spnego
        spn: "HTTP@${HOST}"
```

When `select` is `auto`, the core picks the first variant with
valid credentials.

## Wire Protocol

All communication is JSON objects separated by newlines (JSON-lines).
Each message has an `id` and `type` field. Messages are correlated
by `id`.

### Lifecycle (persistent plugins only)

```
Core → Plugin:  {"id": "init", "type": "init", "config": {...}}
Plugin → Core:  {"id": "init", "type": "init_ok"}
...tool calls...
Core → Plugin:  {"id": "shutdown", "type": "shutdown"}
Plugin → Core:  {"id": "shutdown", "type": "shutdown_ok"}
```

### Tool Calls

```
Core → Plugin:  {"id": "req-1", "type": "tool_call", "tool": "my_tool",
                  "params": {"query": "test"}}
Plugin → Core:  {"id": "req-1", "type": "tool_result",
                  "result": {"items": [...]}}
```

Error response:

```json
{"id": "req-1", "type": "tool_result",
 "error": {"code": "not_found", "message": "No results"}}
```

### HTTP Proxy

Plugins never make HTTP calls directly. Instead, they send
`http_request` messages and the core handles auth, TLS, retries:

```
Plugin → Core:  {"id": "http-1", "type": "http_request",
                  "method": "GET", "path": "/api/items",
                  "query": {"q": "test", "limit": "10"}}

Core → Plugin:  {"id": "http-1", "type": "http_response",
                  "status": 200,
                  "headers": {"Content-Type": "application/json"},
                  "body": {"items": [...]}}
```

POST with body:

```json
{"id": "http-2", "type": "http_request",
 "method": "POST", "path": "/api/items",
 "headers": {"Content-Type": "application/json"},
 "body": {"name": "New Item"}}
```

Full URL override (for URLs outside base_url):

```json
{"id": "http-3", "type": "http_request",
 "method": "GET", "url": "https://other.example.com/api/data"}
```

#### Binary Responses

- JSON responses: `body` is the parsed JSON object
- Text responses (`text/*`): `body` is a JSON string
- Binary responses: `body` is base64-encoded,
  `"body_encoding": "base64"` is set

#### Multipart Upload

For file uploads, use `multipart` instead of `body`:

```json
{"id": "http-4", "type": "http_request",
 "method": "POST", "path": "/api/upload",
 "multipart": [
   {"field": "file", "filename": "doc.pdf",
    "content_type": "application/pdf",
    "body": "<base64-encoded>", "body_encoding": "base64"},
   {"field": "comment", "body": "Uploaded via automation"}
 ]}
```

The core assembles the `multipart/form-data` body and sets the
`Content-Type` header with the boundary. Do not set `Content-Type`
yourself for multipart requests.

### Cache

The core provides a key-value cache. Plugins use it through the
protocol:

```
Plugin → Core:  {"id": "c-1", "type": "cache_get", "key": "my-data"}
Core → Plugin:  {"id": "c-1", "type": "cache_get", "hit": true,
                  "value": {"cached": "data"}}
```

```
Plugin → Core:  {"id": "c-2", "type": "cache_set", "key": "my-data",
                  "value": {"new": "data"}, "ttl": 3600}
Core → Plugin:  {"id": "c-2", "type": "cache_set", "ok": true}
```

Other operations: `cache_del`, `cache_list` (glob pattern),
`cache_flush` (clear namespace).

## Complete Examples

### Bash Oneshot Plugin

The simplest possible plugin. No main loop needed — the core spawns
the handler for each tool call and sends one message on stdin.

**plugin.yaml:**
```yaml
name: hello
version: "1.0.0"
description: "A greeting plugin"
execution: oneshot
handler: ./handler.sh
services: {}
tools:
  - name: hello_world
    description: "Says hello to someone"
    params:
      name:
        type: string
        default: "World"
enabled: true
```

**handler.sh:**
```bash
#!/bin/bash
read -r INPUT
ID=$(echo "$INPUT" | jq -r '.id')
NAME=$(echo "$INPUT" | jq -r '.params.name // "World"')

echo "{}" | jq -c --arg id "$ID" --arg name "$NAME" \
  '{id: $id, type: "tool_result", result: {message: ("Hello, " + $name + "!")}}'
```

### Python Persistent Plugin with HTTP and Cache

A persistent plugin that queries an API and caches results. Zero
dependencies beyond Python stdlib.

**plugin.yaml:**
```yaml
name: weather
version: "1.0.0"
description: "Weather lookup"
execution: persistent
handler: ./handler.py
services:
  http:
    base_url: "https://api.weather.example.com"
  cache:
    default_ttl: 600
tools:
  - name: weather_get
    description: "Get weather for a city"
    params:
      city:
        type: string
        required: true
enabled: true
```

**handler.py:**
```python
#!/usr/bin/env python3
import json
import sys


def send(msg):
    print(json.dumps(msg, separators=(",", ":")), flush=True)


def recv():
    line = sys.stdin.readline()
    if not line:
        sys.exit(0)
    return json.loads(line.strip())


_next_id = 0


def gen_id():
    global _next_id
    _next_id += 1
    return f"svc-{_next_id}"


def http(method, path, query=None):
    msg = {"id": gen_id(), "type": "http_request", "method": method, "path": path}
    if query:
        msg["query"] = query
    send(msg)
    resp = recv()
    return resp.get("status", 0), resp.get("body", {})


def cache_get(key):
    send({"id": gen_id(), "type": "cache_get", "key": key})
    resp = recv()
    return resp["value"] if resp.get("hit") else None


def cache_set(key, value, ttl=None):
    msg = {"id": gen_id(), "type": "cache_set", "key": key, "value": value}
    if ttl:
        msg["ttl"] = ttl
    send(msg)
    recv()


def weather_get(params):
    city = params["city"]

    cached = cache_get(f"weather:{city}")
    if cached:
        return cached

    status, body = http("GET", f"/v1/weather", query={"city": city})
    if 200 <= status < 300:
        cache_set(f"weather:{city}", body, ttl=600)
    return body


TOOLS = {"weather_get": weather_get}

# Main loop
while True:
    msg = recv()
    msg_type = msg.get("type")

    if msg_type == "init":
        send({"id": msg["id"], "type": "init_ok"})
    elif msg_type == "shutdown":
        send({"id": msg["id"], "type": "shutdown_ok"})
        break
    elif msg_type == "tool_call":
        fn = TOOLS.get(msg.get("tool"))
        if fn:
            try:
                result = fn(msg.get("params", {}))
                send({"id": msg["id"], "type": "tool_result", "result": result})
            except Exception as e:
                send({"id": msg["id"], "type": "tool_result",
                      "error": {"code": "error", "message": str(e)}})
        else:
            send({"id": msg["id"], "type": "tool_result",
                  "error": {"code": "unknown_tool", "message": msg.get("tool")}})
```

## Setup Wizard Metadata

Plugins can declare a `setup` section with human-facing metadata for
configuration wizards (e.g., `bragctl init`):

```yaml
setup:
  credentials:
    MY_API_URL:
      description: "API base URL"
      example: "https://api.example.com"
      secret: false
    MY_TOKEN:
      description: "API authentication token"
      help_url: "https://docs.example.com/tokens"
      instructions: "Go to Settings > API Tokens > Create"
      secret: true
  validation_tool: my_get_status
  post_setup_message: "Restart the MCP server for changes to take effect."
```

For plugins with auth variants, add variant labels:

```yaml
setup:
  variants:
    cloud:
      label: "Cloud (hosted)"
      description: "For *.example.com instances"
      required: [MY_API_URL, MY_EMAIL, MY_TOKEN]
    server:
      label: "Self-hosted"
      required: [MY_API_URL, MY_TOKEN]
```

## Plugin Environment

Plugins do **not** inherit the core's environment. They receive only
safe variables (`PATH`, `HOME`, `SHELL`, etc.) plus any declared in
the manifest's `env` section. Credential variables like `MY_TOKEN`
are resolved by the core and delivered via the `config` block in
protocol messages — never as environment variables.

## Security

- Plugins are semi-trusted: they run with the same OS privileges as
  the core. Only install plugins you trust.
- Auth tokens are injected by the HTTP proxy. Plugins never see them.
- The proxy enforces HTTPS when auth is configured.
- The proxy validates that request URLs match the plugin's declared
  `base_url` domain or `allowed_domains` list.
- Cache namespaces are isolated — plugins cannot read other plugins'
  cached data.
