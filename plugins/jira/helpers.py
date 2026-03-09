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
