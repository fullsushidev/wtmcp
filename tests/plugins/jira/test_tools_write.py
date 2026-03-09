"""Unit tests for tools_write.py — mock HTTP protocol calls."""

from unittest.mock import patch

import handler
import tools_write


def _mock_http(status, body):
    return patch.object(handler, "http", return_value=(status, body))


# --- jira_create_issue ---


class TestCreateIssue:
    def test_dry_run(self):
        result = tools_write.create_issue(
            {
                "project": "PROJ",
                "issue_type": "Bug",
                "summary": "Test bug",
                "dry_run": True,
            }
        )
        assert result["dry_run"] is True
        assert result["fields"]["project"] == {"key": "PROJ"}
        assert result["fields"]["issuetype"] == {"name": "Bug"}

    def test_dry_run_with_all_fields(self):
        result = tools_write.create_issue(
            {
                "project": "PROJ",
                "issue_type": "Story",
                "summary": "Test",
                "description": "Desc",
                "assignee": "jdoe",
                "priority": "High",
                "labels": ["bug", "urgent"],
                "components": ["Web", "API"],
                "dry_run": True,
            }
        )
        assert result["fields"]["labels"] == ["bug", "urgent"]
        assert result["fields"]["components"] == [{"name": "Web"}, {"name": "API"}]
        assert result["fields"]["priority"] == {"name": "High"}

    def test_create_success(self):
        resp = {"key": "PROJ-123", "id": "10001", "self": "https://jira/issue/10001"}
        with _mock_http(201, resp):
            result = tools_write.create_issue(
                {
                    "project": "PROJ",
                    "issue_type": "Bug",
                    "summary": "Test",
                    "dry_run": False,
                }
            )
            assert result["key"] == "PROJ-123"

    def test_create_error(self):
        with _mock_http(400, {"errors": {"summary": "required"}}):
            result = tools_write.create_issue(
                {
                    "project": "PROJ",
                    "issue_type": "Bug",
                    "summary": "",
                    "dry_run": False,
                }
            )
            assert "errors" in result

    def test_cloud_description_uses_adf(self):
        with patch.object(handler, "is_cloud", True):
            result = tools_write.create_issue(
                {
                    "project": "PROJ",
                    "issue_type": "Bug",
                    "summary": "Test",
                    "description": "Hello",
                    "dry_run": True,
                }
            )
            desc = result["fields"]["description"]
            assert desc["type"] == "doc"
            assert desc["version"] == 1


# --- jira_add_comment / jira_edit_comment ---


class TestComments:
    def test_add_comment_dry_run(self):
        result = tools_write.add_comment({"issue_key": "PROJ-1", "comment": "Nice work!", "dry_run": True})
        assert result["dry_run"] is True
        assert result["comment_preview"] == "Nice work!"

    def test_add_comment_success(self):
        with _mock_http(201, {"id": "100"}):
            result = tools_write.add_comment({"issue_key": "PROJ-1", "comment": "Done", "dry_run": False})
            assert result["success"] is True
            assert result["id"] == "100"

    def test_edit_comment_dry_run(self):
        result = tools_write.edit_comment(
            {
                "issue_key": "PROJ-1",
                "comment_id": "100",
                "comment": "Updated",
                "dry_run": True,
            }
        )
        assert result["dry_run"] is True
        assert result["comment_id"] == "100"

    def test_edit_comment_invalid_id(self):
        import pytest

        with pytest.raises(ValueError, match="Invalid comment_id"):
            tools_write.edit_comment({"issue_key": "PROJ-1", "comment_id": "abc", "comment": "X"})


# --- jira_transition_issue ---


class TestTransitionIssue:
    def test_dry_run(self):
        result = tools_write.transition_issue({"issue_key": "PROJ-1", "transition_id": 5, "dry_run": True})
        assert result["dry_run"] is True
        assert result["transition_id"] == 5

    def test_dry_run_with_resolution(self):
        result = tools_write.transition_issue(
            {
                "issue_key": "PROJ-1",
                "transition_id": 5,
                "resolution": "Done",
                "dry_run": True,
            }
        )
        assert result["resolution"] == "Done"

    def test_transition_success(self):
        with _mock_http(204, {}):
            result = tools_write.transition_issue({"issue_key": "PROJ-1", "transition_id": 5, "dry_run": False})
            assert result["success"] is True


# --- jira_assign_issue ---


class TestAssignIssue:
    def test_dry_run(self):
        result = tools_write.assign_issue({"issue_key": "PROJ-1", "assignee": "jdoe", "dry_run": True})
        assert result["dry_run"] is True
        assert result["assignee"] == "jdoe"

    def test_assign_cloud(self):
        with patch.object(handler, "is_cloud", True), _mock_http(204, {}) as mock_http:
            tools_write.assign_issue({"issue_key": "PROJ-1", "assignee": "abc123", "dry_run": False})
            call_body = mock_http.call_args[1].get("body") or mock_http.call_args[0][3]
            assert call_body == {"accountId": "abc123"}

    def test_assign_server(self):
        with patch.object(handler, "is_cloud", False), _mock_http(204, {}) as mock_http:
            tools_write.assign_issue({"issue_key": "PROJ-1", "assignee": "jdoe", "dry_run": False})
            call_body = mock_http.call_args[1].get("body") or mock_http.call_args[0][3]
            assert call_body == {"name": "jdoe"}


# --- jira_set_priority ---


class TestSetPriority:
    def test_dry_run(self):
        result = tools_write.set_priority({"issue_key": "PROJ-1", "priority": "High", "dry_run": True})
        assert result["dry_run"] is True
        assert result["priority"] == "High"

    def test_success(self):
        with _mock_http(204, {}):
            result = tools_write.set_priority({"issue_key": "PROJ-1", "priority": "High", "dry_run": False})
            assert result["success"] is True


# --- jira_set_labels / add_labels / remove_labels ---


class TestLabels:
    def test_set_labels_dry_run(self):
        result = tools_write.set_labels({"issue_key": "PROJ-1", "labels": ["a", "b"], "dry_run": True})
        assert result["labels"] == ["a", "b"]

    def test_add_labels_dry_run(self):
        result = tools_write.add_labels({"issue_key": "PROJ-1", "labels": ["new"], "dry_run": True})
        assert result["labels_to_add"] == ["new"]

    def test_add_labels_uses_update_operation(self):
        with _mock_http(204, {}) as mock_http:
            tools_write.add_labels({"issue_key": "PROJ-1", "labels": ["a", "b"], "dry_run": False})
            call_body = mock_http.call_args[1].get("body") or mock_http.call_args[0][3]
            assert call_body == {"update": {"labels": [{"add": "a"}, {"add": "b"}]}}

    def test_remove_labels_uses_update_operation(self):
        with _mock_http(204, {}) as mock_http:
            tools_write.remove_labels({"issue_key": "PROJ-1", "labels": ["old"], "dry_run": False})
            call_body = mock_http.call_args[1].get("body") or mock_http.call_args[0][3]
            assert call_body == {"update": {"labels": [{"remove": "old"}]}}


# --- jira_set_text_field ---


class TestSetTextField:
    def test_dry_run(self):
        result = tools_write.set_text_field(
            {
                "issue_key": "PROJ-1",
                "field_name": "summary",
                "value": "New title",
                "dry_run": True,
            }
        )
        assert result["value_preview"] == "New title"

    def test_cloud_description_uses_adf(self):
        with patch.object(handler, "is_cloud", True), _mock_http(204, {}) as mock_http:
            tools_write.set_text_field(
                {
                    "issue_key": "PROJ-1",
                    "field_name": "description",
                    "value": "Hello",
                    "dry_run": False,
                }
            )
            call_body = mock_http.call_args[1].get("body") or mock_http.call_args[0][3]
            assert call_body["fields"]["description"]["type"] == "doc"


# --- jira_set_custom_field ---


class TestSetCustomField:
    def test_dry_run(self):
        result = tools_write.set_custom_field(
            {
                "issue_key": "PROJ-1",
                "field_id": "customfield_123",
                "value": "hello",
                "dry_run": True,
            }
        )
        assert result["dry_run"] is True
        assert result["field_id"] == "customfield_123"

    def test_auto_number(self):
        result = tools_write.set_custom_field(
            {
                "issue_key": "PROJ-1",
                "field_id": "cf_1",
                "value": 42,
                "dry_run": True,
            }
        )
        assert result["field_type"] == "number"

    def test_select_type(self):
        with _mock_http(204, {}) as mock_http:
            tools_write.set_custom_field(
                {
                    "issue_key": "PROJ-1",
                    "field_id": "cf_1",
                    "value": "Option A",
                    "field_type": "select",
                    "dry_run": False,
                }
            )
            call_body = mock_http.call_args[1].get("body") or mock_http.call_args[0][3]
            assert call_body["fields"]["cf_1"] == {"value": "Option A"}


# --- jira_set_story_points ---


class TestSetStoryPoints:
    def test_dry_run(self):
        result = tools_write.set_story_points({"issue_key": "PROJ-1", "points": 5, "dry_run": True})
        assert result["story_points"] == 5


# --- jira_set_components ---


class TestSetComponents:
    def test_dry_run(self):
        result = tools_write.set_components({"issue_key": "PROJ-1", "components": ["Web"], "dry_run": True})
        assert result["components"] == [{"name": "Web"}]

    def test_success(self):
        with _mock_http(204, {}):
            result = tools_write.set_components({"issue_key": "PROJ-1", "components": ["Web"], "dry_run": False})
            assert result["success"] is True


# --- jira_add_issue_link / jira_delete_issue_link ---


class TestIssueLinks:
    def test_add_link_dry_run(self):
        result = tools_write.add_issue_link(
            {
                "link_type": "Blocks",
                "inward_issue_key": "PROJ-1",
                "outward_issue_key": "PROJ-2",
                "dry_run": True,
            }
        )
        assert result["payload"]["type"] == {"name": "Blocks"}

    def test_delete_link_success(self):
        with _mock_http(204, {}):
            result = tools_write.delete_issue_link({"link_id": "12345"})
            assert result["success"] is True


# --- jira_issue_worklog ---


class TestIssueWorklog:
    def test_dry_run(self):
        result = tools_write.issue_worklog(
            {
                "issue_key": "PROJ-1",
                "time_spent": "2h 30m",
                "dry_run": True,
            }
        )
        assert result["time_spent"] == "2h 30m"

    def test_success(self):
        with _mock_http(201, {"id": "w1"}):
            result = tools_write.issue_worklog({"issue_key": "PROJ-1", "time_spent": "1h", "dry_run": False})
            assert result["success"] is True


# --- jira_add_issues_to_sprint / jira_add_issues_to_backlog ---


class TestSprintOps:
    def test_add_to_sprint_dry_run(self):
        result = tools_write.add_issues_to_sprint(
            {
                "sprint_id": 42,
                "issue_keys": "PROJ-1,PROJ-2",
                "dry_run": True,
            }
        )
        assert result["sprint_id"] == 42
        assert result["issue_keys"] == ["PROJ-1", "PROJ-2"]

    def test_add_to_backlog_dry_run(self):
        result = tools_write.add_issues_to_backlog({"issue_keys": "PROJ-1,PROJ-2", "dry_run": True})
        assert result["issue_keys"] == ["PROJ-1", "PROJ-2"]

    def test_add_to_sprint_success(self):
        with _mock_http(204, {}):
            result = tools_write.add_issues_to_sprint(
                {
                    "sprint_id": 42,
                    "issue_keys": "PROJ-1",
                    "dry_run": False,
                }
            )
            assert result["success"] is True
