"""Unit tests for tools_sprint.py — mock HTTP protocol calls."""

from unittest.mock import patch

import handler
import tools_sprint

SAMPLE_SPRINTS: list[dict] = [
    {"id": 1, "name": "Sprint 1", "state": "closed", "startDate": "2026-01-01", "endDate": "2026-01-14"},
    {"id": 2, "name": "Sprint 2", "state": "active", "startDate": "2026-01-15", "endDate": "2026-01-28"},
]

SPRINT_LIST_RESPONSE = {"values": SAMPLE_SPRINTS, "isLast": True}

SEARCH_ISSUES = {
    "total": 2,
    "issues": [
        {
            "key": "PROJ-1",
            "fields": {
                "summary": "Issue 1",
                "status": {"name": "Open"},
                "assignee": {"displayName": "Alice"},
                "priority": {"name": "High"},
            },
        },
        {
            "key": "PROJ-2",
            "fields": {
                "summary": "Issue 2",
                "status": {"name": "Closed"},
                "assignee": None,
                "priority": {"name": "Low"},
            },
        },
    ],
}


def _mock_http(status, body):
    return patch.object(handler, "http", return_value=(status, body, {}))


# --- jira_list_available_sprints ---


class TestListAvailableSprints:
    def test_from_board(self):
        with _mock_http(200, SPRINT_LIST_RESPONSE):
            result = tools_sprint.list_available_sprints({"board_id": "10"})
            assert result["count"] == 2
            # Sorted by name descending (natural sort)
            assert result["sprints"][0]["name"] == "Sprint 2"

    def test_http_error(self):
        with _mock_http(404, {"error": "Board not found"}):
            result = tools_sprint.list_available_sprints({"board_id": "999"})
            assert "error" in result


# --- jira_get_sprint_issues ---


class TestGetSprintIssues:
    def test_brief_mode(self):
        with _mock_http(200, SEARCH_ISSUES):
            result = tools_sprint.get_sprint_issues({"sprint_name": "Sprint 2", "brief": True})
            assert result["total"] == 2
            assert result["sprint_name"] == "Sprint 2"
            assert result["issues"][0]["key"] == "PROJ-1"
            assert "fields" not in result["issues"][0]

    def test_full_mode(self):
        with _mock_http(200, SEARCH_ISSUES):
            result = tools_sprint.get_sprint_issues({"sprint_name": "Sprint 2", "brief": False})
            assert result["issues"][0]["key"] == "PROJ-1"
            assert result["issues"][0]["status"] == "Open"
            assert "fields" not in result["issues"][0]

    def test_truncated(self):
        truncated = {"total": 500, "issues": SEARCH_ISSUES["issues"]}
        with _mock_http(200, truncated):
            result = tools_sprint.get_sprint_issues({"sprint_name": "Sprint 2"})
            assert result["truncated"] is True

    def test_http_error(self):
        with _mock_http(400, {"error": "Bad JQL"}):
            result = tools_sprint.get_sprint_issues({"sprint_name": "Bad"})
            assert "error" in result


# --- jira_search_by_sprint ---


class TestSearchBySprint:
    def test_basic(self):
        with _mock_http(200, SEARCH_ISSUES):
            result = tools_sprint.search_by_sprint({"sprint_name": "Sprint 2"})
            assert result["count"] == 2

    def test_with_filters(self):
        with _mock_http(200, SEARCH_ISSUES) as mock_http:
            tools_sprint.search_by_sprint(
                {
                    "sprint_name": "Sprint 2",
                    "assignee": "jdoe",
                    "status": "Closed",
                }
            )
            query = mock_http.call_args[1].get("query") or mock_http.call_args[0][2]
            jql = query["jql"]
            assert 'assignee = "jdoe"' in jql
            assert 'status = "Closed"' in jql

    def test_current_user_assignee(self):
        with _mock_http(200, SEARCH_ISSUES) as mock_http:
            tools_sprint.search_by_sprint({"sprint_name": "Sprint 2", "assignee": "currentUser()"})
            query = mock_http.call_args[1].get("query") or mock_http.call_args[0][2]
            assert "assignee = currentUser()" in query["jql"]


# --- jira_get_all_sprints ---


class TestGetAllSprints:
    def test_success(self):
        with _mock_http(200, SPRINT_LIST_RESPONSE):
            result = tools_sprint.get_all_sprints({"board_id": "10"})
            assert result["count"] == 2

    def test_with_max_results(self):
        with _mock_http(200, SPRINT_LIST_RESPONSE):
            result = tools_sprint.get_all_sprints({"board_id": "10", "max_results": 1})
            assert result["count"] == 1


# --- jira_get_all_active_sprints ---


class TestGetAllActiveSprints:
    def test_success(self):
        active_resp = {"values": [SAMPLE_SPRINTS[1]], "isLast": True}
        with _mock_http(200, active_resp):
            result = tools_sprint.get_all_active_sprints({"board_id": "10"})
            assert result["count"] == 1
            assert result["sprints"][0]["state"] == "active"


# --- jira_get_sprint_details ---


class TestGetSprintDetails:
    def test_success(self):
        sprint = {"id": 2, "name": "Sprint 2", "state": "active", "goal": "Ship feature X"}
        with _mock_http(200, sprint):
            result = tools_sprint.get_sprint_details({"sprint_id": 2})
            assert result["name"] == "Sprint 2"
            assert result["goal"] == "Ship feature X"

    def test_not_found(self):
        with _mock_http(404, {"error": "Sprint not found"}):
            result = tools_sprint.get_sprint_details({"sprint_id": 999})
            assert "error" in result


# --- jira_get_sprint_report ---


class TestGetSprintReport:
    def test_cloud_returns_error(self):
        with patch.object(handler, "is_cloud", True):
            result = tools_sprint.get_sprint_report({"board_id": "10", "sprint_id": "2"})
            assert "error" in result
            assert "Cloud" in result["error"]

    def test_server_success(self):
        report = {"sprint": {"id": 2}, "completedIssues": [], "issuesNotCompletedInCurrentSprint": []}
        with patch.object(handler, "is_cloud", False), _mock_http(200, report):
            result = tools_sprint.get_sprint_report({"board_id": "10", "sprint_id": "2"})
            assert "sprint" in result


# --- jira_get_all_agile_boards ---


class TestGetAllAgileBoards:
    def test_success(self):
        boards_resp = {
            "values": [
                {"id": 1, "name": "Team Board", "type": "scrum", "location": {"projectKey": "PROJ"}},
                {"id": 2, "name": "Kanban", "type": "kanban", "location": {}},
            ]
        }
        with _mock_http(200, boards_resp):
            result = tools_sprint.get_all_agile_boards({})
            assert result["count"] == 2
            assert result["boards"][0]["projectKey"] == "PROJ"
            assert "projectKey" not in result["boards"][1]  # no projectKey in location

    def test_with_project_filter(self):
        with _mock_http(200, {"values": []}) as mock_http:
            tools_sprint.get_all_agile_boards({"project_key": "PROJ"})
            query = mock_http.call_args[1].get("query") or mock_http.call_args[0][2]
            assert query["projectKeyOrId"] == "PROJ"


# --- jira_get_issues_for_board ---


class TestGetIssuesForBoard:
    def test_brief_mode(self):
        with _mock_http(200, SEARCH_ISSUES):
            result = tools_sprint.get_issues_for_board({"board_id": "10", "brief": True})
            assert result["count"] == 2
            assert "fields" not in result["issues"][0]

    def test_full_mode(self):
        with _mock_http(200, SEARCH_ISSUES):
            result = tools_sprint.get_issues_for_board({"board_id": "10", "brief": False})
            assert result["issues"][0]["key"] == "PROJ-1"
            assert result["issues"][0]["status"] == "Open"
            assert "fields" not in result["issues"][0]


# --- jira_get_all_issues_for_sprint_in_board ---


class TestGetAllIssuesForSprintInBoard:
    def test_brief_mode(self):
        with _mock_http(200, SEARCH_ISSUES):
            result = tools_sprint.get_all_issues_for_sprint_in_board(
                {
                    "board_id": "10",
                    "sprint_id": "2",
                    "brief": True,
                }
            )
            assert result["count"] == 2
            assert result["issues"][0]["key"] == "PROJ-1"

    def test_http_error(self):
        with _mock_http(404, {"error": "Not found"}):
            result = tools_sprint.get_all_issues_for_sprint_in_board({"board_id": "10", "sprint_id": "999"})
            assert "error" in result
