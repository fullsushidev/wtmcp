"""Unit tests for helpers.py — pure functions, no mocking needed."""

import pytest
from helpers import extract_brief_issue, extract_user_fields, is_user_alias, validate_issue_key

# --- validate_issue_key ---


class TestValidateIssueKey:
    def test_valid_simple(self):
        assert validate_issue_key("PROJ-123") == "PROJ-123"

    def test_valid_uppercase(self):
        assert validate_issue_key("proj-1") == "PROJ-1"

    def test_valid_with_whitespace(self):
        assert validate_issue_key("  ABC-42  ") == "ABC-42"

    def test_valid_with_numbers_in_project(self):
        assert validate_issue_key("RHEL9-100") == "RHEL9-100"

    def test_valid_with_underscore(self):
        assert validate_issue_key("MY_PROJ-7") == "MY_PROJ-7"

    def test_invalid_no_dash(self):
        with pytest.raises(ValueError, match="Invalid issue key"):
            validate_issue_key("PROJ123")

    def test_invalid_no_number(self):
        with pytest.raises(ValueError, match="Invalid issue key"):
            validate_issue_key("PROJ-")

    def test_invalid_starts_with_number(self):
        with pytest.raises(ValueError, match="Invalid issue key"):
            validate_issue_key("123-ABC")

    def test_invalid_empty(self):
        with pytest.raises(ValueError, match="Invalid issue key"):
            validate_issue_key("")

    def test_invalid_just_whitespace(self):
        with pytest.raises(ValueError, match="Invalid issue key"):
            validate_issue_key("   ")

    def test_invalid_lowercase_dash(self):
        # Single lowercase letter doesn't match [A-Z]
        with pytest.raises(ValueError, match="Invalid issue key"):
            validate_issue_key("a-1")


# --- is_user_alias ---


class TestIsUserAlias:
    def test_me(self):
        assert is_user_alias("me") is True

    def test_myself(self):
        assert is_user_alias("myself") is True

    def test_currentuser(self):
        assert is_user_alias("currentUser") is True

    def test_case_insensitive(self):
        assert is_user_alias("ME") is True
        assert is_user_alias("Myself") is True
        assert is_user_alias("CURRENTUSER") is True

    def test_with_whitespace(self):
        assert is_user_alias("  me  ") is True

    def test_regular_username(self):
        assert is_user_alias("jdoe") is False

    def test_empty(self):
        assert is_user_alias("") is False


# --- extract_brief_issue ---


def _make_issue(key="TEST-1", summary="Fix bug", status="Open", assignee="Jane", priority="High"):
    """Build a Jira issue dict for testing."""
    fields = {"summary": summary}
    if status is not None:
        fields["status"] = {"name": status}
    if assignee is not None:
        fields["assignee"] = {"displayName": assignee}
    if priority is not None:
        fields["priority"] = {"name": priority}
    return {"key": key, "fields": fields}


class TestExtractBriefIssue:
    def test_full_fields(self):
        result = extract_brief_issue(_make_issue())
        assert result == {
            "key": "TEST-1",
            "summary": "Fix bug",
            "status": "Open",
            "assignee": "Jane",
            "priority": "High",
        }

    def test_missing_status(self):
        result = extract_brief_issue(_make_issue(status=None))
        assert result["status"] == ""

    def test_missing_assignee(self):
        result = extract_brief_issue(_make_issue(assignee=None))
        assert result["assignee"] == ""

    def test_missing_priority(self):
        result = extract_brief_issue(_make_issue(priority=None))
        assert result["priority"] == ""

    def test_empty_fields(self):
        result = extract_brief_issue({"key": "X-1", "fields": {}})
        assert result["summary"] == ""
        assert result["status"] == ""
        assert result["assignee"] == ""
        assert result["priority"] == ""

    def test_no_fields_key(self):
        result = extract_brief_issue({"key": "X-1"})
        assert result["key"] == "X-1"
        assert result["summary"] == ""

    def test_non_dict_status(self):
        # Edge case: status is a string (shouldn't happen, but be safe)
        issue = {"key": "X-1", "fields": {"status": "Open"}}
        result = extract_brief_issue(issue)
        assert result["status"] == ""


# --- extract_user_fields ---


class TestExtractUserFields:
    def test_cloud_user(self):
        user = {
            "accountId": "abc123",
            "name": "jdoe",
            "displayName": "Jane Doe",
            "emailAddress": "jane@example.com",
            "active": True,
            "timeZone": "America/New_York",
        }
        result = extract_user_fields(user)
        assert result["accountId"] == "abc123"
        assert result["displayName"] == "Jane Doe"
        assert result["emailAddress"] == "jane@example.com"
        assert result["active"] is True
        assert result["timeZone"] == "America/New_York"

    def test_server_user_key_fallback(self):
        user = {
            "key": "jdoe",
            "name": "jdoe",
            "displayName": "Jane Doe",
        }
        result = extract_user_fields(user)
        assert result["accountId"] == "jdoe"

    def test_missing_fields(self):
        result = extract_user_fields({})
        assert result["accountId"] is None
        assert result["displayName"] is None
        assert result["active"] is None
