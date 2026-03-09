"""Jira plugin helper functions.

Pure utility functions with no protocol or I/O dependencies.
"""

import re

_ISSUE_KEY_RE = re.compile(r"^[A-Z][A-Z0-9_]+-\d+$")

_USER_ALIASES = frozenset({"me", "myself", "currentuser"})


def validate_issue_key(key):
    """Validate and return a cleaned issue key.

    Raises ValueError if key doesn't match PROJECT-123 format.
    """
    cleaned = key.strip().upper()
    if not _ISSUE_KEY_RE.match(cleaned):
        raise ValueError(f"Invalid issue key: '{key}' (expected format: PROJECT-123)")
    return cleaned


def is_user_alias(username):
    """Check if username is a self-referencing alias (me, myself, currentUser)."""
    return username.lower().strip() in _USER_ALIASES


def extract_brief_issue(issue):
    """Extract compact summary from a Jira issue response.

    Returns dict with key, summary, status, assignee, priority.
    """
    fields = issue.get("fields", {})
    status = fields.get("status")
    assignee = fields.get("assignee")
    priority = fields.get("priority")
    return {
        "key": issue.get("key", ""),
        "summary": fields.get("summary", ""),
        "status": status.get("name", "") if isinstance(status, dict) else "",
        "assignee": assignee.get("displayName", "") if isinstance(assignee, dict) else "",
        "priority": priority.get("name", "") if isinstance(priority, dict) else "",
    }


def text_to_adf(text):
    """Convert plain text to Atlassian Document Format (ADF).

    Jira Cloud API v3 requires comment/description bodies in ADF.
    Splits on newlines into paragraphs.
    """
    if not text:
        return {"version": 1, "type": "doc", "content": [{"type": "paragraph", "content": []}]}

    content = []
    for para in text.split("\n"):
        if para.strip():
            content.append({"type": "paragraph", "content": [{"type": "text", "text": para}]})
        else:
            content.append({"type": "paragraph", "content": []})

    return {"version": 1, "type": "doc", "content": content}


def normalize_components(components):
    """Normalize a list of component names or dicts to [{name: ...}]."""
    return [c if isinstance(c, dict) else {"name": str(c)} for c in components]


def parse_sprint_field(sprint_data):
    """Parse sprint field from Cloud (dict) or Server (string) format.

    Server format: "com.atlassian...@abc[id=123,name=Sprint 1,state=active,...]"
    Cloud format: {"id": 123, "name": "Sprint 1", "state": "active", ...}
    """
    if isinstance(sprint_data, dict):
        return sprint_data
    if isinstance(sprint_data, str):
        content = sprint_data
        bracket_start = content.find("[")
        bracket_end = content.rfind("]")
        if bracket_start != -1 and bracket_end != -1:
            content = content[bracket_start + 1 : bracket_end]
        info = {}
        for pair in content.split(","):
            if "=" in pair:
                key, value = pair.split("=", 1)
                info[key.strip()] = value.strip()
        return info
    return {}


def natural_sort_key(name):
    """Sort key for numeric-aware ordering of sprint names.

    "Sprint 9" sorts before "Sprint 10" instead of after.
    """
    return [(0, int(c), "") if c.isdigit() else (1, 0, c.lower()) for c in re.split(r"(\d+)", name)]


def escape_jql(value):
    """Escape a value for safe use in double-quoted JQL strings."""
    value = value.replace("\\", "\\\\")
    value = value.replace('"', '\\"')
    value = value.replace("\n", "\\n")
    value = value.replace("\r", "\\r")
    value = value.replace("\0", "")
    return value


def extract_sprint_summary(sprint):
    """Extract compact sprint info: id, name, state, dates."""
    return {
        "id": sprint.get("id"),
        "name": sprint.get("name"),
        "state": sprint.get("state"),
        "startDate": sprint.get("startDate"),
        "endDate": sprint.get("endDate"),
    }


def extract_user_fields(user):
    """Extract standard fields from a Jira user dict."""
    return {
        "accountId": user.get("accountId") or user.get("key"),
        "name": user.get("name"),
        "displayName": user.get("displayName"),
        "emailAddress": user.get("emailAddress"),
        "active": user.get("active"),
        "timeZone": user.get("timeZone"),
    }
