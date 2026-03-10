#!/usr/bin/env python3
"""Confluence plugin handler.

Persistent handler with init/shutdown lifecycle. All HTTP calls go
through the core proxy (no HTTP libraries needed). Auth is handled
by the core. Cache is provided by the core cache service.

Zero dependencies beyond Python stdlib.
"""

import json
import sys

_next_id = 0
config = {}


def _send(msg):
    """Write a JSON message to stdout (core reads it)."""
    print(json.dumps(msg, separators=(",", ":")), flush=True)


def _gen_id(prefix="svc"):
    global _next_id
    _next_id += 1
    return f"{prefix}-{_next_id}"


def _recv():
    """Read a JSON message from stdin (core writes it)."""
    line = sys.stdin.readline()
    if not line:
        sys.exit(0)
    return json.loads(line.strip())


def http(method, path, query=None, body=None, headers=None):
    """Make an HTTP request via the core proxy.

    Returns (status, body, headers). Status 0 means transport error.
    """
    msg = {
        "id": _gen_id("http"),
        "type": "http_request",
        "method": method,
        "path": path,
    }
    if query:
        msg["query"] = query
    if body is not None:
        msg["body"] = body
    if headers:
        msg["headers"] = headers
    _send(msg)
    resp = _recv()
    status = resp.get("status", 0)
    if status == 0:
        return 0, {"error": resp.get("error", "request failed")}, {}
    return status, resp.get("body", {}), resp.get("headers", {})


def cache_get(key):
    """Get a value from the core cache. Returns value or None."""
    _send({"id": _gen_id("cache"), "type": "cache_get", "key": key})
    resp = _recv()
    if resp.get("hit"):
        return resp["value"]
    return None


def cache_set(key, value, ttl=None):
    """Set a value in the core cache."""
    msg = {
        "id": _gen_id("cache"),
        "type": "cache_set",
        "key": key,
        "value": value,
    }
    if ttl is not None:
        msg["ttl"] = ttl
    _send(msg)
    _recv()  # consume ack


def log(message):
    """Write a log message to stderr (captured by core)."""
    print(f"confluence: {message}", file=sys.stderr)


# --- Tools ---


def confluence_get_page(params):
    page_id = params.get("page_id")
    if not page_id:
        raise ValueError("page_id is required")

    status, body, _ = http(
        "GET",
        f"/rest/api/content/{page_id}",
        query={"expand": "body.storage,version,space"},
    )
    if status >= 400:
        return {"error": f"HTTP {status}", "details": body}
    return body


def confluence_get_page_by_title(params):
    title = params.get("title")
    space_key = params.get("space_key")
    if not title or not space_key:
        raise ValueError("title and space_key are required")

    status, body, _ = http(
        "GET",
        "/rest/api/content",
        query={
            "title": title,
            "spaceKey": space_key,
            "expand": "body.storage,version,space",
            "limit": 1,
        },
    )
    if status >= 400:
        return {"error": f"HTTP {status}", "details": body}

    results = body.get("results", [])
    if not results:
        return {"error": f"No page found with title '{title}' in space '{space_key}'"}
    return results[0]


def confluence_search(params):
    cql = params.get("cql")
    if not cql:
        raise ValueError("cql is required")
    limit = params.get("limit", 25)

    status, body, _ = http(
        "GET",
        "/rest/api/content/search",
        query={"cql": cql, "limit": limit},
    )
    if status >= 400:
        return {"error": f"HTTP {status}", "details": body}
    return body


def confluence_get_pages_by_title(params):
    title = params.get("title")
    space_key = params.get("space_key")
    if not title or not space_key:
        raise ValueError("title and space_key are required")

    status, body, _ = http(
        "GET",
        "/rest/api/content",
        query={
            "title": title,
            "spaceKey": space_key,
            "expand": "body.storage,version,space",
        },
    )
    if status >= 400:
        return {"error": f"HTTP {status}", "details": body}

    results = body.get("results", [])
    return {"results": results, "count": len(results)}


def confluence_get_spaces(params):
    limit = params.get("limit", 50)

    status, body, _ = http(
        "GET",
        "/rest/api/space",
        query={"limit": limit, "expand": "description"},
    )
    if status >= 400:
        return {"error": f"HTTP {status}", "details": body}
    return body


def confluence_get_space(params):
    space_key = params.get("space_key")
    if not space_key:
        raise ValueError("space_key is required")

    status, body, _ = http(
        "GET",
        f"/rest/api/space/{space_key}",
        query={"expand": "description,homepage"},
    )
    if status >= 400:
        return {"error": f"HTTP {status}", "details": body}
    return body


def confluence_create_page(params):
    space_key = params.get("space_key")
    title = params.get("title")
    body_html = params.get("body")
    parent_id = params.get("parent_id")
    dry_run = params.get("dry_run", True)

    if not space_key or not title or not body_html:
        raise ValueError("space_key, title, and body are required")

    if dry_run:
        preview = body_html[:200] + "..." if len(body_html) > 200 else body_html
        return {
            "dry_run": True,
            "action": "confluence_create_page",
            "space_key": space_key,
            "title": title,
            "parent_id": parent_id,
            "body_preview": preview,
        }

    payload = {
        "type": "page",
        "title": title,
        "space": {"key": space_key},
        "body": {
            "storage": {
                "value": body_html,
                "representation": "storage",
            }
        },
    }
    if parent_id:
        payload["ancestors"] = [{"id": parent_id}]

    status, body, _ = http("POST", "/rest/api/content", body=payload)
    if status >= 400:
        return {"error": f"HTTP {status}", "details": body}
    return body


def confluence_update_page(params):
    page_id = params.get("page_id")
    title = params.get("title")
    body_html = params.get("body")
    dry_run = params.get("dry_run", True)

    if not page_id or not title or not body_html:
        raise ValueError("page_id, title, and body are required")

    if dry_run:
        preview = body_html[:200] + "..." if len(body_html) > 200 else body_html
        return {
            "dry_run": True,
            "action": "confluence_update_page",
            "page_id": page_id,
            "title": title,
            "body_preview": preview,
        }

    # Fetch current version number
    status, current, _ = http(
        "GET",
        f"/rest/api/content/{page_id}",
        query={"expand": "version"},
    )
    if status >= 400:
        return {"error": f"HTTP {status} fetching current version", "details": current}

    current_version = current.get("version", {}).get("number", 0)

    payload = {
        "version": {"number": current_version + 1},
        "title": title,
        "type": "page",
        "body": {
            "storage": {
                "value": body_html,
                "representation": "storage",
            }
        },
    }

    status, body, _ = http("PUT", f"/rest/api/content/{page_id}", body=payload)
    if status >= 400:
        return {"error": f"HTTP {status}", "details": body}
    return body


def confluence_add_comment(params):
    page_id = params.get("page_id")
    comment = params.get("comment")
    dry_run = params.get("dry_run", True)

    if not page_id or not comment:
        raise ValueError("page_id and comment are required")

    if dry_run:
        return {
            "dry_run": True,
            "action": "confluence_add_comment",
            "page_id": page_id,
            "comment_preview": comment[:200],
        }

    payload = {
        "type": "comment",
        "container": {"id": page_id, "type": "page"},
        "body": {
            "storage": {
                "value": comment,
                "representation": "storage",
            }
        },
    }

    status, body, _ = http("POST", "/rest/api/content", body=payload)
    if status >= 400:
        return {"error": f"HTTP {status}", "details": body}
    return body


def confluence_get_page_children(params):
    page_id = params.get("page_id")
    if not page_id:
        raise ValueError("page_id is required")

    status, body, _ = http("GET", f"/rest/api/content/{page_id}/child/page")
    if status >= 400:
        return {"error": f"HTTP {status}", "details": body}

    if isinstance(body, dict):
        results = body.get("results", [])
    elif isinstance(body, list):
        results = body
    else:
        results = []

    return {"results": results, "count": len(results)}


def confluence_get_page_history(params):
    page_id = params.get("page_id")
    if not page_id:
        raise ValueError("page_id is required")

    status, body, _ = http("GET", f"/rest/api/content/{page_id}/history")
    if status >= 400:
        return {"error": f"HTTP {status}", "details": body}
    return body


# --- Tool registry ---

TOOLS = {
    "confluence_get_page": confluence_get_page,
    "confluence_get_page_by_title": confluence_get_page_by_title,
    "confluence_search": confluence_search,
    "confluence_get_pages_by_title": confluence_get_pages_by_title,
    "confluence_get_spaces": confluence_get_spaces,
    "confluence_get_space": confluence_get_space,
    "confluence_create_page": confluence_create_page,
    "confluence_update_page": confluence_update_page,
    "confluence_add_comment": confluence_add_comment,
    "confluence_get_page_children": confluence_get_page_children,
    "confluence_get_page_history": confluence_get_page_history,
}


def main():
    log("handler starting")

    while True:
        msg = _recv()
        msg_id = msg.get("id")
        msg_type = msg.get("type")

        if msg_type == "init":
            global config
            config = msg.get("config", {})
            log(f"init: url={config.get('confluence_url', '?')}")
            _send({"id": msg_id, "type": "init_ok"})
            continue

        if msg_type == "shutdown":
            log("shutting down")
            _send({"id": msg_id, "type": "shutdown_ok"})
            break

        if msg_type == "tool_call":
            tool = msg.get("tool")
            handler_fn = TOOLS.get(tool)
            if not handler_fn:
                _send(
                    {
                        "id": msg_id,
                        "type": "tool_result",
                        "error": {
                            "code": "unknown_tool",
                            "message": f"Unknown: {tool}",
                        },
                    }
                )
                continue

            try:
                result = handler_fn(msg.get("params", {}))
                _send({"id": msg_id, "type": "tool_result", "result": result})
            except Exception as e:
                log(f"error in {tool}: {e}")
                _send(
                    {
                        "id": msg_id,
                        "type": "tool_result",
                        "error": {"code": "handler_error", "message": str(e)},
                    }
                )
            continue

        log(f"unknown message type: {msg_type}")


if __name__ == "__main__":
    main()
