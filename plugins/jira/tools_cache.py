"""Jira export, cache query, and diagnostic tools (Phase 4).

Export tools write JSON files to disk. Cache query tools read local
JSON files. These are pure local I/O — no Jira API calls (except
debug_fields which queries the Jira field list).
"""

import json
import os
from datetime import datetime, timezone
from pathlib import Path

import handler
from helpers import (
    calculate_sprint_metrics,
    extract_brief_issue,
    extract_nested_field,
    validate_issue_key,
)


def _validate_export_path(file_path):
    """Validate export path is under the working directory or /tmp."""
    resolved = Path(file_path).resolve()
    cwd = Path.cwd().resolve()
    tmp = Path("/tmp").resolve()
    if not (resolved.is_relative_to(cwd) or resolved.is_relative_to(tmp)):
        raise ValueError(f"Export path must be under working directory or /tmp: {file_path}")
    return resolved


def export_sprint_data(params):
    """Export all issues for a sprint to a local JSON file."""
    board_id = params.get("board_id", "")
    sprint_id = params.get("sprint_id", "")
    output_file = params.get("output_file", "")

    path = _validate_export_path(output_file)

    # Fetch sprint info
    status, sprint_info, _ = handler.http("GET", f"/rest/agile/1.0/sprint/{sprint_id}")
    if status < 200 or status >= 300:
        return sprint_info

    # Fetch issues
    status, issues_resp, _ = handler.http(
        "GET",
        f"/rest/agile/1.0/board/{board_id}/sprint/{sprint_id}/issue",
        query={"maxResults": "1000", "fields": "*all"},
    )
    if status < 200 or status >= 300:
        return issues_resp

    issues = issues_resp.get("issues", [])
    export_data = {
        "export_metadata": {
            "export_timestamp": datetime.now(timezone.utc).isoformat(),
            "board_id": board_id,
            "sprint_id": sprint_id,
            "tool": "jira_export_sprint_data",
        },
        "sprint_info": sprint_info,
        "issues": issues,
    }

    path.parent.mkdir(parents=True, exist_ok=True)
    fd = os.open(str(path), os.O_WRONLY | os.O_CREAT | os.O_TRUNC, 0o600)
    with os.fdopen(fd, "w", encoding="utf-8") as f:
        json.dump(export_data, f, indent=2, ensure_ascii=False)

    return {
        "success": True,
        "file_path": str(path),
        "issue_count": len(issues),
        "sprint_name": sprint_info.get("name") if isinstance(sprint_info, dict) else None,
    }


def export_board_sprints(params):
    """Export all sprints for a board to a local JSON file."""
    board_id = params.get("board_id", "")
    state = params.get("state")
    output_file = params.get("output_file", "")

    path = _validate_export_path(output_file)

    # Paginate sprints
    all_sprints: list = []
    start = 0
    while True:
        query: dict = {"startAt": str(start), "maxResults": "50"}
        if state:
            query["state"] = state
        status, body, _ = handler.http("GET", f"/rest/agile/1.0/board/{board_id}/sprint", query=query)
        if status < 200 or status >= 300:
            return body
        values = body.get("values", [])
        all_sprints.extend(values)
        if body.get("isLast", True) or len(values) < 50:
            break
        start += 50

    export_data = {
        "export_metadata": {
            "export_timestamp": datetime.now(timezone.utc).isoformat(),
            "board_id": board_id,
            "state": state,
            "tool": "jira_export_board_sprints",
        },
        "sprints": {"values": all_sprints, "isLast": True},
    }

    path.parent.mkdir(parents=True, exist_ok=True)
    fd = os.open(str(path), os.O_WRONLY | os.O_CREAT | os.O_TRUNC, 0o600)
    with os.fdopen(fd, "w", encoding="utf-8") as f:
        json.dump(export_data, f, indent=2, ensure_ascii=False)

    return {"success": True, "file_path": str(path), "sprint_count": len(all_sprints)}


def export_sprint_report(params):
    """Export sprint report to a local JSON file (Server/DC only)."""
    board_id = params.get("board_id", "")
    sprint_id = params.get("sprint_id", "")
    output_file = params.get("output_file", "")

    if handler.is_cloud:
        return {"error": "Sprint reports are not available on Jira Cloud."}

    path = _validate_export_path(output_file)

    status, report, _ = handler.http(
        "GET",
        "/rest/greenhopper/1.0/rapid/charts/sprintreport",
        query={"rapidViewId": str(board_id), "sprintId": str(sprint_id)},
    )
    if status < 200 or status >= 300:
        return report

    export_data = {
        "export_metadata": {
            "export_timestamp": datetime.now(timezone.utc).isoformat(),
            "board_id": board_id,
            "sprint_id": sprint_id,
            "tool": "jira_export_sprint_report",
        },
        "report": report,
    }

    path.parent.mkdir(parents=True, exist_ok=True)
    fd = os.open(str(path), os.O_WRONLY | os.O_CREAT | os.O_TRUNC, 0o600)
    with os.fdopen(fd, "w", encoding="utf-8") as f:
        json.dump(export_data, f, indent=2, ensure_ascii=False)

    return {"success": True, "file_path": str(path)}


def query_local_sprint_data(params):
    """Query and filter locally exported sprint data."""
    file_path = params.get("file_path", "")
    assignee = params.get("assignee")
    issue_status = params.get("status")
    issue_type = params.get("issue_type")
    labels = params.get("labels")
    priority = params.get("priority")
    brief = params.get("brief", True)

    path = _validate_export_path(file_path)
    if not path.exists():
        return {"error": f"File not found: {file_path}"}

    with path.open("r", encoding="utf-8") as f:
        data = json.load(f)

    issues = data.get("issues", [])
    filtered = issues

    if assignee:
        a = assignee.lower()
        filtered = [
            i for i in filtered if a in (i.get("fields", {}).get("assignee") or {}).get("displayName", "").lower()
        ]

    if issue_status:
        s = issue_status.lower()
        filtered = [i for i in filtered if s in (i.get("fields", {}).get("status") or {}).get("name", "").lower()]

    if issue_type:
        t = issue_type.lower()
        filtered = [i for i in filtered if t in (i.get("fields", {}).get("issuetype") or {}).get("name", "").lower()]

    if labels:
        filtered = [i for i in filtered if all(lbl in i.get("fields", {}).get("labels", []) for lbl in labels)]

    if priority:
        p = priority.lower()
        filtered = [i for i in filtered if p in (i.get("fields", {}).get("priority") or {}).get("name", "").lower()]

    result: dict = {
        "total_issues": len(issues),
        "filtered_count": len(filtered),
        "sprint_info": data.get("sprint_info"),
    }

    if brief:
        result["issues"] = [extract_brief_issue(i) for i in filtered]
    else:
        result["issues"] = filtered
    return result


def compare_sprints(params):
    """Compare metrics across multiple exported sprint files."""
    file_paths = params.get("file_paths", [])
    if isinstance(file_paths, str):
        file_paths = [p.strip() for p in file_paths.split(",") if p.strip()]

    sprint_data = []
    for fp in file_paths:
        path = _validate_export_path(fp)
        if not path.exists():
            return {"error": f"File not found: {fp}"}

        with path.open("r", encoding="utf-8") as f:
            data = json.load(f)

        issues = data.get("issues", [])
        sprint_info = data.get("sprint_info", {})
        metrics = calculate_sprint_metrics(issues)
        metrics["sprint_name"] = sprint_info.get("name", "Unknown")
        metrics["sprint_id"] = sprint_info.get("id")
        metrics["file_path"] = str(path)
        sprint_data.append(metrics)

    return {"sprints": sprint_data, "comparison_count": len(sprint_data)}


def sprint_metrics_summary(params):
    """Generate metrics summary from exported sprint data."""
    file_path = params.get("file_path", "")

    path = _validate_export_path(file_path)
    if not path.exists():
        return {"error": f"File not found: {file_path}"}

    with path.open("r", encoding="utf-8") as f:
        data = json.load(f)

    issues = data.get("issues", [])
    sprint_info = data.get("sprint_info", {})
    metrics = calculate_sprint_metrics(issues)
    metrics["sprint_name"] = sprint_info.get("name", "Unknown")
    metrics["sprint_id"] = sprint_info.get("id")
    metrics["sprint_state"] = sprint_info.get("state")
    metrics["source_file"] = str(path)
    return metrics


def read_cache_summary(params):
    """Read and summarize a cached/exported Jira response file."""
    file_path = params.get("file_path", "")
    fields = params.get("fields")
    issue_keys = params.get("issue_keys")
    max_issues = int(params.get("max_issues", 20))

    path = _validate_export_path(file_path)
    if not path.exists():
        return {"error": f"File not found: {file_path}"}

    with path.open("r", encoding="utf-8") as f:
        cache_data = json.load(f)

    data = cache_data.get("data", cache_data)
    issues: list = []
    if isinstance(data, dict) and "issues" in data:
        issues = data["issues"]
    elif isinstance(data, list):
        issues = data

    if not issues:
        return {"file_path": file_path, "issue_count": 0, "message": "No issues found"}

    if issue_keys:
        keys = issue_keys if isinstance(issue_keys, list) else [k.strip() for k in issue_keys.split(",")]
        issues = [i for i in issues if i.get("key") in keys]

    total = len(issues)
    issues = issues[:max_issues]

    default_fields = ["key", "summary", "status.name", "priority.name", "assignee.displayName"]
    extract = fields if fields else default_fields

    summaries = []
    for issue in issues:
        fields_data = issue.get("fields", {})
        row: dict = {}
        for field in extract:
            if field == "key":
                row["key"] = issue.get("key", "")
            else:
                row[field] = extract_nested_field(fields_data, field)
        summaries.append(row)

    result: dict = {"file_path": file_path, "total_issues": total, "returned": len(summaries), "issues": summaries}
    if total > max_issues:
        result["note"] = f"Showing {max_issues} of {total}. Use issue_keys to filter."
    return result


def get_issue_from_cache(params):
    """Get a specific issue from a cached/exported file."""
    file_path = params.get("file_path", "")
    issue_key = validate_issue_key(params.get("issue_key", ""))

    path = _validate_export_path(file_path)
    if not path.exists():
        return {"error": f"File not found: {file_path}"}

    with path.open("r", encoding="utf-8") as f:
        cache_data = json.load(f)

    data = cache_data.get("data", cache_data)
    issues: list = []
    if isinstance(data, dict) and "issues" in data:
        issues = data["issues"]
    elif isinstance(data, list):
        issues = data

    for issue in issues:
        if issue.get("key") == issue_key:
            fields_data = issue.get("fields", {})
            result: dict = {"key": issue.get("key"), "self": issue.get("self")}
            # Extract core fields into readable format
            for field, path_str in [
                ("summary", "summary"),
                ("description", "description"),
                ("status", "status.name"),
                ("priority", "priority.name"),
                ("issuetype", "issuetype.name"),
                ("resolution", "resolution.name"),
                ("assignee", "assignee.displayName"),
                ("reporter", "reporter.displayName"),
                ("created", "created"),
                ("updated", "updated"),
                ("labels", "labels"),
            ]:
                result[field] = extract_nested_field(fields_data, path_str)
            return result

    return {"error": f"Issue {issue_key} not found in {file_path}"}


def debug_fields(params):
    """List Jira custom fields, optionally filtered by name."""
    search = params.get("search", "")

    status, body, _ = handler.http("GET", "/rest/api/2/field")
    if status < 200 or status >= 300:
        return body

    fields_list = body if isinstance(body, list) else []
    custom_fields = []
    matching = []

    search_lower = search.lower() if search else None

    for field in fields_list:
        fid = field.get("id", "")
        fname = field.get("name", "")
        ftype = (field.get("schema") or {}).get("type", "")
        info = {"id": fid, "name": fname, "type": ftype}

        if fid.startswith("customfield_"):
            custom_fields.append(info)

        if search_lower and search_lower in fname.lower():
            matching.append(info)

    result: dict = {"total_fields": len(fields_list), "custom_fields_count": len(custom_fields)}

    if search_lower:
        result["search"] = search
        result["matching_fields"] = matching
        result["match_count"] = len(matching)
    else:
        result["sample_custom_fields"] = custom_fields[:10]

    return result


TOOLS = {
    "jira_export_sprint_data": export_sprint_data,
    "jira_export_board_sprints": export_board_sprints,
    "jira_export_sprint_report": export_sprint_report,
    "jira_query_local_sprint_data": query_local_sprint_data,
    "jira_compare_sprints": compare_sprints,
    "jira_sprint_metrics_summary": sprint_metrics_summary,
    "jira_read_cache_summary": read_cache_summary,
    "jira_get_issue_from_cache": get_issue_from_cache,
    "jira_debug_fields": debug_fields,
}
