"""Unit tests for tools_read.py — mock HTTP/cache protocol calls."""

from unittest.mock import patch

import handler
import tools_read


def _mock_http(status, body):
    """Return a mock for handler.http that returns (status, body)."""
    return patch.object(handler, "http", return_value=(status, body))


def _mock_cache_get(value=None):
    """Return a mock for handler.cache_get. None means cache miss."""
    return patch.object(handler, "cache_get", return_value=value)


def _mock_cache_set():
    """Return a mock for handler.cache_set."""
    return patch.object(handler, "cache_set")


MYSELF_RESPONSE = {
    "accountId": "abc123",
    "name": "jdoe",
    "displayName": "Jane Doe",
    "emailAddress": "jane@example.com",
    "active": True,
    "timeZone": "UTC",
    "avatarUrls": {"48x48": "https://example.com/avatar.png"},
    "locale": "en_US",
}

SAMPLE_ISSUES: list[dict] = [
    {
        "key": "PROJ-1",
        "fields": {
            "summary": "First issue",
            "status": {"name": "Open"},
            "assignee": {"displayName": "Alice"},
            "priority": {"name": "High"},
        },
    },
    {
        "key": "PROJ-2",
        "fields": {
            "summary": "Second issue",
            "status": {"name": "Closed"},
            "assignee": None,
            "priority": {"name": "Low"},
        },
    },
]

SEARCH_RESPONSE = {"total": 2, "issues": SAMPLE_ISSUES}


# --- jira_get_myself ---


class TestGetMyself:
    def test_cache_miss_fetches_and_caches(self):
        with _mock_cache_get(None), _mock_http(200, MYSELF_RESPONSE), _mock_cache_set() as mock_set:
            result = tools_read.get_myself({})
            assert result["accountId"] == "abc123"
            assert result["displayName"] == "Jane Doe"
            # Should not include raw fields like avatarUrls
            assert "avatarUrls" not in result
            # Should have cached
            mock_set.assert_called_once()
            assert mock_set.call_args[1]["ttl"] == 3600

    def test_cache_hit_returns_cached(self):
        cached = {"accountId": "abc123", "displayName": "Jane Doe"}
        with _mock_cache_get(cached) as mock_get:
            result = tools_read.get_myself({})
            assert result == cached
            mock_get.assert_called_once_with("myself")

    def test_http_error_returns_error_body(self):
        error_body = {"error": "Unauthorized"}
        with _mock_cache_get(None), _mock_http(401, error_body):
            result = tools_read.get_myself({})
            assert result == error_body


# --- jira_search ---


class TestSearch:
    def test_brief_mode(self):
        with _mock_cache_get(None), _mock_http(200, SEARCH_RESPONSE), _mock_cache_set():
            result = tools_read.search({"jql": "project = PROJ", "brief": True})
            assert result["total"] == 2
            assert result["count"] == 2
            assert result["issues"][0] == {
                "key": "PROJ-1",
                "summary": "First issue",
                "status": "Open",
                "assignee": "Alice",
                "priority": "High",
            }
            # Null assignee should become empty string
            assert result["issues"][1]["assignee"] == ""

    def test_full_mode(self):
        with _mock_cache_get(None), _mock_http(200, SEARCH_RESPONSE), _mock_cache_set():
            result = tools_read.search({"jql": "project = PROJ", "brief": False})
            # Full mode returns raw issues
            assert result["issues"][0]["fields"]["status"]["name"] == "Open"

    def test_truncated_warning(self):
        truncated = {"total": 100, "issues": SEARCH_RESPONSE["issues"]}
        with _mock_cache_get(None), _mock_http(200, truncated), _mock_cache_set():
            result = tools_read.search({"jql": "project = PROJ"})
            assert result["truncated"] is True
            assert "warning" in result

    def test_max_results_capped_at_200(self):
        with _mock_cache_get(None), _mock_http(200, SEARCH_RESPONSE) as mock_http, _mock_cache_set():
            tools_read.search({"jql": "project = PROJ", "max_results": 500})
            query = mock_http.call_args[1].get("query") or mock_http.call_args[0][2]
            assert query["maxResults"] == "200"

    def test_cache_hit(self):
        cached = {"total": 1, "count": 1, "issues": []}
        with _mock_cache_get(cached):
            result = tools_read.search({"jql": "project = PROJ"})
            assert result == cached

    def test_http_error(self):
        with _mock_cache_get(None), _mock_http(500, {"error": "Server Error"}):
            result = tools_read.search({"jql": "bad"})
            assert result == {"error": "Server Error"}


# --- jira_get_issues ---


class TestGetIssues:
    def test_single_key_brief(self):
        response = {"issues": [SAMPLE_ISSUES[0]]}
        with _mock_http(200, response):
            result = tools_read.get_issues({"issue_keys": "PROJ-1"})
            assert result["count"] == 1
            assert result["issues"][0]["key"] == "PROJ-1"
            assert result["issues"][0]["status"] == "Open"

    def test_multiple_keys(self):
        with _mock_http(200, SEARCH_RESPONSE):
            result = tools_read.get_issues({"issue_keys": "PROJ-1,PROJ-2"})
            assert result["count"] == 2

    def test_missing_key(self):
        response = {"issues": [SAMPLE_ISSUES[0]]}
        with _mock_http(200, response):
            result = tools_read.get_issues({"issue_keys": "PROJ-1,PROJ-999"})
            assert result["count"] == 2
            assert result["issues"][0]["key"] == "PROJ-1"
            assert result["issues"][1]["error"] == "Issue not found or not accessible"

    def test_full_mode(self):
        response = {"issues": [SAMPLE_ISSUES[0]]}
        with _mock_http(200, response):
            result = tools_read.get_issues({"issue_keys": "PROJ-1", "brief": False})
            # Full mode returns raw issue data
            assert "fields" in result["issues"][0]

    def test_empty_keys(self):
        result = tools_read.get_issues({"issue_keys": ""})
        assert result == {"issues": [], "count": 0}

    def test_invalid_key_raises(self):
        import pytest

        with pytest.raises(ValueError, match="Invalid issue key"):
            tools_read.get_issues({"issue_keys": "bad-key"})

    def test_http_error(self):
        with _mock_http(403, {"error": "Forbidden"}):
            result = tools_read.get_issues({"issue_keys": "PROJ-1"})
            assert result == {"error": "Forbidden"}


# --- jira_get_user ---


class TestGetUser:
    def test_alias_me(self):
        with _mock_http(200, MYSELF_RESPONSE):
            result = tools_read.get_user({"username": "me"})
            assert result["accountId"] == "abc123"
            assert result["displayName"] == "Jane Doe"

    def test_alias_myself(self):
        with _mock_http(200, MYSELF_RESPONSE):
            result = tools_read.get_user({"username": "myself"})
            assert result["displayName"] == "Jane Doe"

    def test_cloud_search(self):
        users = [{"accountId": "x", "displayName": "Bob", "name": "bob"}]
        with patch.object(handler, "is_cloud", True), _mock_http(200, users) as mock_http:
            result = tools_read.get_user({"username": "bob"})
            assert result["accountId"] == "x"
            query = mock_http.call_args[1].get("query") or mock_http.call_args[0][2]
            assert query == {"query": "bob"}

    def test_server_search(self):
        users = [{"key": "bob", "displayName": "Bob", "name": "bob"}]
        with patch.object(handler, "is_cloud", False), _mock_http(200, users) as mock_http:
            result = tools_read.get_user({"username": "bob"})
            assert result["accountId"] == "bob"
            query = mock_http.call_args[1].get("query") or mock_http.call_args[0][2]
            assert query == {"username": "bob"}

    def test_user_not_found(self):
        with patch.object(handler, "is_cloud", False), _mock_http(200, []):
            result = tools_read.get_user({"username": "nobody"})
            assert "error" in result
            assert "not found" in result["error"]

    def test_empty_username(self):
        result = tools_read.get_user({"username": ""})
        assert result == {"error": "username is required"}


# --- jira_get_transitions ---


class TestGetTransitions:
    def test_success(self):
        transitions = {
            "transitions": [
                {"id": "1", "name": "Start Progress"},
                {"id": "2", "name": "Close"},
            ]
        }
        with _mock_http(200, transitions):
            result = tools_read.get_transitions({"issue_key": "PROJ-1"})
            assert result["transitions"][0]["name"] == "Start Progress"

    def test_invalid_key(self):
        import pytest

        with pytest.raises(ValueError, match="Invalid issue key"):
            tools_read.get_transitions({"issue_key": "bad"})

    def test_http_error(self):
        with _mock_http(404, {"error": "Not Found"}):
            result = tools_read.get_transitions({"issue_key": "PROJ-999"})
            assert result == {"error": "Not Found"}


# --- jira_get_resolutions ---


class TestGetResolutions:
    def test_success(self):
        raw = [
            {"id": "1", "name": "Done", "description": "Work is complete"},
            {"id": "2", "name": "Won't Do", "description": "Not going to do this"},
        ]
        with _mock_http(200, raw):
            result = tools_read.get_resolutions({})
            assert len(result["resolutions"]) == 2
            assert result["resolutions"][0] == {"id": "1", "name": "Done"}
            # Descriptions are stripped
            assert "description" not in result["resolutions"][0]

    def test_http_error(self):
        with _mock_http(500, {"error": "Server Error"}):
            result = tools_read.get_resolutions({})
            assert result == {"error": "Server Error"}


# --- jira_get_link_types ---


class TestGetLinkTypes:
    def test_success(self):
        link_types = {
            "issueLinkTypes": [
                {"id": "1", "name": "Blocks", "inward": "is blocked by", "outward": "blocks"},
                {"id": "2", "name": "Clones", "inward": "is cloned by", "outward": "clones"},
            ]
        }
        with _mock_http(200, link_types):
            result = tools_read.get_link_types({})
            assert result["issueLinkTypes"][0]["name"] == "Blocks"

    def test_http_error(self):
        with _mock_http(403, {"error": "Forbidden"}):
            result = tools_read.get_link_types({})
            assert result == {"error": "Forbidden"}
