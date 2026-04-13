"""Jira sprint and agile board tool implementations."""

import handler
from helpers import (
    escape_jql,
    extract_brief_issue,
    extract_issue_fields,
    extract_sprint_summary,
    http_error,
    natural_sort_key,
    parse_sprint_field,
)

_BRIEF_FIELDS = "summary,status,assignee,priority"


def list_available_sprints(params):
    """List sprints from a board or from recent tickets."""
    board_id = params.get("board_id")
    state = params.get("state")
    limit = int(params.get("limit", 20))

    if board_id:
        sprints = _get_board_sprints(board_id, state, limit)
    else:
        sprints = _get_sprints_from_tickets(state, limit)

    if isinstance(sprints, dict) and "error" in sprints:
        return sprints

    sprints.sort(key=lambda x: natural_sort_key(x.get("name", "")), reverse=True)
    sprints = sprints[:limit]

    return {"sprints": sprints, "count": len(sprints)}


def _get_board_sprints(board_id, state, limit):
    """Fetch sprints from a board via the agile API.

    The API returns sprints oldest-first. For closed/active sprints
    we want the most recent ones, so we paginate to the end and
    return the last `limit` entries.
    """
    all_sprints = []
    start = 0
    page_size = 50
    while True:
        query = {"startAt": str(start), "maxResults": str(page_size)}
        if state:
            query["state"] = state
        status, body, _ = handler.http("GET", f"/rest/agile/1.0/board/{board_id}/sprint", query=query)
        if status < 200 or status >= 300:
            return http_error(status, body)
        values = body.get("values", [])
        all_sprints.extend(extract_sprint_summary(s) for s in values)
        if body.get("isLast", True) or len(values) < page_size:
            break
        start += page_size
    # API returns oldest-first; return the most recent entries.
    return all_sprints[-limit:] if len(all_sprints) > limit else all_sprints


def _get_sprints_from_tickets(state, limit):
    """Extract sprint names from recent tickets via JQL."""
    sf = handler.sprint_field
    jql = "assignee = currentUser() AND updated >= -60d ORDER BY updated DESC"
    status, body, _ = handler.http(
        "GET",
        "/rest/api/2/search",
        query={"jql": jql, "maxResults": "100", "fields": sf},
    )
    if status < 200 or status >= 300:
        return http_error(status, body)

    seen = set()
    sprints = []
    for issue in body.get("issues", []):
        sprint_data = issue.get("fields", {}).get(sf)
        # sprint field can be a single object or a list
        items = sprint_data if isinstance(sprint_data, list) else ([sprint_data] if sprint_data else [])
        for obj in items:
            parsed = parse_sprint_field(obj)
            name = parsed.get("name")
            if not name or name in seen:
                continue
            seen.add(name)
            sprint_state = (parsed.get("state") or "").lower()
            if state and sprint_state != state.lower():
                continue
            sprints.append(extract_sprint_summary(parsed))
            if len(sprints) >= limit:
                return sprints
    return sprints


def get_sprint_issues(params):
    """Get issues from a sprint by name, with brief mode."""
    sprint_name = params.get("sprint_name", "")
    max_results = min(int(params.get("max_results", 200)), 1000)
    brief = params.get("brief", True)

    jql = f'sprint = "{escape_jql(sprint_name)}" ORDER BY updated DESC'
    status, body, _ = handler.http(
        "GET",
        "/rest/api/2/search",
        query={"jql": jql, "maxResults": str(max_results), "fields": "summary,status,assignee,priority"},
    )
    if status < 200 or status >= 300:
        return http_error(status, body)

    issues = body.get("issues", [])
    total = body.get("total", len(issues))
    result: dict = {"total": total, "count": len(issues), "sprint_name": sprint_name}

    if total > len(issues):
        result["truncated"] = True

    if brief:
        result["issues"] = [extract_brief_issue(i) for i in issues]
    else:
        result["issues"] = [extract_issue_fields(i, _BRIEF_FIELDS) for i in issues]
    return result


def search_by_sprint(params):
    """Search issues in a sprint with optional assignee/status filters."""
    sprint_name = params.get("sprint_name", "")
    assignee = params.get("assignee")
    issue_status = params.get("status")
    max_results = min(int(params.get("max_results", 200)), 1000)
    brief = params.get("brief", True)

    clauses = [f'sprint = "{escape_jql(sprint_name)}"']
    if assignee:
        if assignee.lower() == "currentuser()":
            clauses.append("assignee = currentUser()")
        else:
            clauses.append(f'assignee = "{escape_jql(assignee)}"')
    if issue_status:
        clauses.append(f'status = "{escape_jql(issue_status)}"')

    jql = " AND ".join(clauses) + " ORDER BY updated DESC"

    status, body, _ = handler.http(
        "GET",
        "/rest/api/2/search",
        query={"jql": jql, "maxResults": str(max_results), "fields": "summary,status,assignee,priority"},
    )
    if status < 200 or status >= 300:
        return http_error(status, body)

    issues = body.get("issues", [])
    total = body.get("total", len(issues))
    result: dict = {"total": total, "count": len(issues), "sprint_name": sprint_name}

    if brief:
        result["issues"] = [extract_brief_issue(i) for i in issues]
    else:
        result["issues"] = [extract_issue_fields(i, _BRIEF_FIELDS) for i in issues]
    return result


def get_all_sprints(params):
    """Get all sprints for a board with optional state filter."""
    board_id = params.get("board_id", "")
    state = params.get("state")
    max_results = params.get("max_results")

    sprints = _get_board_sprints(board_id, state, max_results or 1000)
    if isinstance(sprints, dict) and "error" in sprints:
        return sprints

    if max_results and len(sprints) > int(max_results):
        sprints = sprints[-int(max_results) :]

    return {"sprints": sprints, "count": len(sprints)}


def get_all_active_sprints(params):
    """Get active sprints for a board."""
    board_id = params.get("board_id", "")
    sprints = _get_board_sprints(board_id, "active", 100)
    if isinstance(sprints, dict) and "error" in sprints:
        return sprints
    return {"sprints": sprints, "count": len(sprints)}


def get_sprint_details(params):
    """Get detailed sprint information."""
    sprint_id = params.get("sprint_id", "")
    status, body, _ = handler.http("GET", f"/rest/agile/1.0/sprint/{sprint_id}")
    if status < 200 or status >= 300:
        return http_error(status, body)
    return body


def get_sprint_report(params):
    """Get sprint report (Server/DC only, Greenhopper API)."""
    board_id = params.get("board_id", "")
    sprint_id = params.get("sprint_id", "")

    if handler.is_cloud:
        return {
            "error": "Sprint reports are not available on Jira Cloud. "
            "The Greenhopper API is deprecated. "
            "Use jira_get_sprint_issues instead."
        }

    status, body, _ = handler.http(
        "GET",
        "/rest/greenhopper/1.0/rapid/charts/sprintreport",
        query={"rapidViewId": str(board_id), "sprintId": str(sprint_id)},
    )
    if status < 200 or status >= 300:
        return http_error(status, body)
    return body


def get_all_agile_boards(params):
    """Get agile boards, filtered to compact output."""
    project_key = params.get("project_key")
    board_type = params.get("board_type")

    query = {}
    if project_key:
        query["projectKeyOrId"] = project_key
    if board_type:
        query["type"] = board_type

    status, body, _ = handler.http("GET", "/rest/agile/1.0/board", query=query or None)
    if status < 200 or status >= 300:
        return http_error(status, body)

    boards = []
    for b in body.get("values", []):
        board = {"id": b.get("id"), "name": b.get("name"), "type": b.get("type")}
        location = b.get("location", {})
        if location.get("projectKey"):
            board["projectKey"] = location["projectKey"]
        boards.append(board)

    return {"boards": boards, "count": len(boards)}


def get_issues_for_board(params):
    """Get issues for a board with brief mode."""
    board_id = params.get("board_id", "")
    max_results = min(int(params.get("max_results", 50)), 200)
    brief = params.get("brief", True)

    status, body, _ = handler.http(
        "GET",
        f"/rest/agile/1.0/board/{board_id}/issue",
        query={"maxResults": str(max_results), "fields": "summary,status,assignee,priority"},
    )
    if status < 200 or status >= 300:
        return http_error(status, body)

    issues = body.get("issues", [])
    total = body.get("total", len(issues))
    result: dict = {"total": total, "count": len(issues)}

    if brief:
        result["issues"] = [extract_brief_issue(i) for i in issues]
    else:
        result["issues"] = [extract_issue_fields(i, _BRIEF_FIELDS) for i in issues]
    return result


def get_all_issues_for_sprint_in_board(params):
    """Get issues for a sprint in a board with brief mode."""
    board_id = params.get("board_id", "")
    sprint_id = params.get("sprint_id", "")
    max_results = min(int(params.get("max_results", 200)), 1000)
    brief = params.get("brief", True)

    status, body, _ = handler.http(
        "GET",
        f"/rest/agile/1.0/board/{board_id}/sprint/{sprint_id}/issue",
        query={"maxResults": str(max_results), "fields": "summary,status,assignee,priority"},
    )
    if status < 200 or status >= 300:
        return http_error(status, body)

    issues = body.get("issues", [])
    total = body.get("total", len(issues))
    result: dict = {"total": total, "count": len(issues)}

    if brief:
        result["issues"] = [extract_brief_issue(i) for i in issues]
    else:
        result["issues"] = [extract_issue_fields(i, _BRIEF_FIELDS) for i in issues]
    return result


TOOLS = {
    "jira_list_available_sprints": list_available_sprints,
    "jira_get_sprint_issues": get_sprint_issues,
    "jira_search_by_sprint": search_by_sprint,
    "jira_get_all_sprints": get_all_sprints,
    "jira_get_all_active_sprints": get_all_active_sprints,
    "jira_get_sprint_details": get_sprint_details,
    "jira_get_sprint_report": get_sprint_report,
    "jira_get_all_agile_boards": get_all_agile_boards,
    "jira_get_issues_for_board": get_issues_for_board,
    "jira_get_all_issues_for_sprint_in_board": get_all_issues_for_sprint_in_board,
}
