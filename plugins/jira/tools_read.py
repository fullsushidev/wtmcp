"""Jira read-only tool implementations."""

import hashlib

import handler
from helpers import (
    extract_brief_issue,
    extract_user_fields,
    http_error,
    is_user_alias,
    validate_issue_key,
)


def get_myself(_params):
    """Get authenticated user profile, cached for 1 hour."""
    cached = handler.cache_get("myself")
    if cached:
        handler.log("returning cached user profile")
        return cached

    status, body, _ = handler.http("GET", "/rest/api/2/myself")
    if status < 200 or status >= 300:
        return http_error(status, body)
    if not isinstance(body, dict):
        return http_error(status, body)

    result = extract_user_fields(body)
    handler.cache_set("myself", result, ttl=3600)
    return result


def search(params):
    """Search issues using JQL with optional brief mode."""
    jql = params.get("jql", "")
    max_results = min(int(params.get("max_results", 50)), 200)
    fields = params.get("fields", "summary,status,assignee,priority")
    brief = params.get("brief", True)

    key_input = f"{jql}|{max_results}|{fields}|{brief}"
    cache_key = f"search:{hashlib.sha256(key_input.encode()).hexdigest()[:12]}"
    cached = handler.cache_get(cache_key)
    if cached:
        handler.log(f"returning cached search: {jql[:60]}")
        return cached

    status, body, _ = handler.http(
        "GET",
        "/rest/api/2/search",
        query={
            "jql": jql,
            "maxResults": str(max_results),
            "fields": fields,
        },
    )
    if status < 200 or status >= 300 or not isinstance(body, dict):
        return http_error(status, body)

    issues = body.get("issues", [])
    total = body.get("total", len(issues))

    result = {"total": total, "count": len(issues)}

    if total > len(issues):
        result["truncated"] = True
        result["warning"] = f"Showing {len(issues)} of {total} results. Narrow the JQL or increase max_results."

    if brief:
        result["issues"] = [extract_brief_issue(i) for i in issues]
    else:
        result["issues"] = issues

    handler.cache_set(cache_key, result, ttl=300)
    return result


def get_issues(params):
    """Get issues by key with optional brief mode."""
    raw_keys = params.get("issue_keys", "")
    keys = [validate_issue_key(k) for k in raw_keys.split(",") if k.strip()]
    if not keys:
        return {"issues": [], "count": 0}

    fields = params.get("fields", "summary,status,assignee,priority")
    brief = params.get("brief", True)

    # Batch fetch via JQL: key in (K1, K2, ...)
    key_list = ",".join(keys)
    jql = f"key in ({key_list})"

    status, body, _ = handler.http(
        "GET",
        "/rest/api/2/search",
        query={
            "jql": jql,
            "maxResults": str(len(keys)),
            "fields": fields,
        },
    )
    if status < 200 or status >= 300 or not isinstance(body, dict):
        return http_error(status, body)

    # Build lookup for ordering and missing-key detection.
    fetched = {}
    for issue in body.get("issues", []):
        fetched[issue.get("key", "")] = issue

    results = []
    for k in keys:
        issue = fetched.get(k)
        if issue:
            results.append(extract_brief_issue(issue) if brief else issue)
        else:
            results.append({"key": k, "error": "Issue not found or not accessible"})

    return {"issues": results, "count": len(results)}


def get_user(params):
    """Look up a Jira user."""
    username = params.get("username", "")
    if not username.strip():
        return {"error": "username is required"}

    # Handle self-referencing aliases.
    if is_user_alias(username):
        status, body, _ = handler.http("GET", "/rest/api/2/myself")
        if 200 <= status < 300 and isinstance(body, dict):
            return extract_user_fields(body)
        return http_error(status, body)

    # Cloud uses query=, Server uses username=.
    if handler.is_cloud:
        query = {"query": username}
    else:
        query = {"username": username}

    status, body, _ = handler.http("GET", "/rest/api/2/user/search", query=query)
    if status < 200 or status >= 300:
        return http_error(status, body)

    users = body if isinstance(body, list) else []
    if not users:
        return {"error": f"User '{username}' not found"}

    return extract_user_fields(users[0])


def get_transitions(params):
    """Get available workflow transitions for an issue."""
    issue_key = validate_issue_key(params.get("issue_key", ""))
    status, body, _ = handler.http("GET", f"/rest/api/2/issue/{issue_key}/transitions")
    if status < 200 or status >= 300:
        return http_error(status, body)
    return body


def get_resolutions(_params):
    """Get all available resolution values."""
    status, body, _ = handler.http("GET", "/rest/api/2/resolution")
    if status < 200 or status >= 300:
        return http_error(status, body)
    if isinstance(body, list):
        return {"resolutions": [{"id": r.get("id"), "name": r.get("name")} for r in body]}
    return body


def get_link_types(_params):
    """List available issue link types."""
    status, body, _ = handler.http("GET", "/rest/api/2/issueLinkType")
    if status < 200 or status >= 300:
        return http_error(status, body)
    return body


TOOLS = {
    "jira_get_myself": get_myself,
    "jira_search": search,
    "jira_get_issues": get_issues,
    "jira_get_user": get_user,
    "jira_get_transitions": get_transitions,
    "jira_get_resolutions": get_resolutions,
    "jira_get_link_types": get_link_types,
}
