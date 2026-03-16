"""Integration tests for Cloud vs Server API path selection."""

from unittest.mock import patch

import handler
import tools_cache
import tools_read
import tools_write


class TestSearchPaths:
    """Verify search endpoint uses correct API version."""

    def test_search_server_uses_v2(self):
        """Server mode: search calls /rest/api/2/search."""
        with (
            patch.object(handler, "is_cloud", False),
            patch.object(handler, "cache_get", return_value=None),
            patch.object(handler, "cache_set"),
            patch.object(handler, "_send") as mock_send,
            patch.object(
                handler, "_recv", return_value={"status": 200, "body": {"total": 0, "issues": []}, "headers": {}}
            ),
        ):
            tools_read.search({"jql": "project=PROJ"})
            sent_msg = mock_send.call_args[0][0]
            assert sent_msg["path"] == "/rest/api/2/search"

    def test_search_cloud_uses_v3(self):
        """Cloud mode: search calls /rest/api/3/search/jql."""
        with (
            patch.object(handler, "is_cloud", True),
            patch.object(handler, "cache_get", return_value=None),
            patch.object(handler, "cache_set"),
            patch.object(handler, "_send") as mock_send,
            patch.object(
                handler, "_recv", return_value={"status": 200, "body": {"total": 0, "issues": []}, "headers": {}}
            ),
        ):
            tools_read.search({"jql": "project=PROJ"})
            sent_msg = mock_send.call_args[0][0]
            assert sent_msg["path"] == "/rest/api/3/search/jql"


class TestReadToolPaths:
    """Verify read tools use correct API version."""

    def test_get_myself_paths(self):
        """get_myself uses v2 on Server, v3 on Cloud."""
        mock_response = {"status": 200, "body": {"accountId": "123", "displayName": "Test User"}, "headers": {}}

        # Server
        with (
            patch.object(handler, "is_cloud", False),
            patch.object(handler, "cache_get", return_value=None),
            patch.object(handler, "cache_set"),
            patch.object(handler, "_send") as mock_send,
            patch.object(handler, "_recv", return_value=mock_response),
        ):
            tools_read.get_myself({})
            assert mock_send.call_args[0][0]["path"] == "/rest/api/2/myself"

        # Cloud
        with (
            patch.object(handler, "is_cloud", True),
            patch.object(handler, "cache_get", return_value=None),
            patch.object(handler, "cache_set"),
            patch.object(handler, "_send") as mock_send,
            patch.object(handler, "_recv", return_value=mock_response),
        ):
            tools_read.get_myself({})
            assert mock_send.call_args[0][0]["path"] == "/rest/api/3/myself"

    def test_get_user_paths(self):
        """get_user uses v2 on Server, v3 on Cloud."""
        mock_response = {"status": 200, "body": [{"accountId": "123", "displayName": "Test User"}], "headers": {}}

        # Server
        with (
            patch.object(handler, "is_cloud", False),
            patch.object(handler, "_send") as mock_send,
            patch.object(handler, "_recv", return_value=mock_response),
        ):
            tools_read.get_user({"username": "testuser"})
            assert mock_send.call_args[0][0]["path"] == "/rest/api/2/user/search"

        # Cloud
        with (
            patch.object(handler, "is_cloud", True),
            patch.object(handler, "_send") as mock_send,
            patch.object(handler, "_recv", return_value=mock_response),
        ):
            tools_read.get_user({"username": "testuser"})
            assert mock_send.call_args[0][0]["path"] == "/rest/api/3/user/search"

    def test_get_transitions_paths(self):
        """get_transitions uses v2 on Server, v3 on Cloud."""
        mock_response = {"status": 200, "body": {"transitions": []}, "headers": {}}

        # Server
        with (
            patch.object(handler, "is_cloud", False),
            patch.object(handler, "_send") as mock_send,
            patch.object(handler, "_recv", return_value=mock_response),
        ):
            tools_read.get_transitions({"issue_key": "PROJ-1"})
            assert mock_send.call_args[0][0]["path"] == "/rest/api/2/issue/PROJ-1/transitions"

        # Cloud
        with (
            patch.object(handler, "is_cloud", True),
            patch.object(handler, "_send") as mock_send,
            patch.object(handler, "_recv", return_value=mock_response),
        ):
            tools_read.get_transitions({"issue_key": "PROJ-1"})
            assert mock_send.call_args[0][0]["path"] == "/rest/api/3/issue/PROJ-1/transitions"


class TestWriteToolPaths:
    """Verify write tools use correct API version."""

    def test_create_issue_paths(self):
        """create_issue uses v2 on Server, v3 on Cloud."""
        params = {"project": "PROJ", "issue_type": "Task", "summary": "Test", "dry_run": False}
        mock_response = {"status": 201, "body": {"key": "PROJ-1", "id": "10001"}, "headers": {}}

        # Server
        with (
            patch.object(handler, "is_cloud", False),
            patch.object(handler, "_send") as mock_send,
            patch.object(handler, "_recv", return_value=mock_response),
        ):
            tools_write.create_issue(params)
            assert mock_send.call_args[0][0]["path"] == "/rest/api/2/issue"

        # Cloud
        with (
            patch.object(handler, "is_cloud", True),
            patch.object(handler, "_send") as mock_send,
            patch.object(handler, "_recv", return_value=mock_response),
        ):
            tools_write.create_issue(params)
            assert mock_send.call_args[0][0]["path"] == "/rest/api/3/issue"

    def test_add_comment_paths(self):
        """add_comment uses v2 on Server, v3 on Cloud."""
        params = {"issue_key": "PROJ-1", "comment": "Test comment", "dry_run": False}
        mock_response = {"status": 201, "body": {"id": "10001"}, "headers": {}}

        # Server
        with (
            patch.object(handler, "is_cloud", False),
            patch.object(handler, "_send") as mock_send,
            patch.object(handler, "_recv", return_value=mock_response),
        ):
            tools_write.add_comment(params)
            assert mock_send.call_args[0][0]["path"] == "/rest/api/2/issue/PROJ-1/comment"

        # Cloud
        with (
            patch.object(handler, "is_cloud", True),
            patch.object(handler, "_send") as mock_send,
            patch.object(handler, "_recv", return_value=mock_response),
        ):
            tools_write.add_comment(params)
            assert mock_send.call_args[0][0]["path"] == "/rest/api/3/issue/PROJ-1/comment"

    def test_worklog_paths(self):
        """issue_worklog uses v2 on Server, v3 on Cloud."""
        params = {"issue_key": "PROJ-1", "time_spent": "2h", "dry_run": False}
        mock_response = {"status": 201, "body": {"id": "w1"}, "headers": {}}

        # Server
        with (
            patch.object(handler, "is_cloud", False),
            patch.object(handler, "_send") as mock_send,
            patch.object(handler, "_recv", return_value=mock_response),
        ):
            tools_write.issue_worklog(params)
            assert mock_send.call_args[0][0]["path"] == "/rest/api/2/issue/PROJ-1/worklog"

        # Cloud
        with (
            patch.object(handler, "is_cloud", True),
            patch.object(handler, "_send") as mock_send,
            patch.object(handler, "_recv", return_value=mock_response),
        ):
            tools_write.issue_worklog(params)
            assert mock_send.call_args[0][0]["path"] == "/rest/api/3/issue/PROJ-1/worklog"


class TestCacheToolPaths:
    """Verify cache/attachment tools use correct API version."""

    def test_debug_fields_paths(self):
        """debug_fields uses v2 on Server, v3 on Cloud."""
        mock_response = {"status": 200, "body": [], "headers": {}}

        # Server
        with (
            patch.object(handler, "is_cloud", False),
            patch.object(handler, "_send") as mock_send,
            patch.object(handler, "_recv", return_value=mock_response),
        ):
            tools_cache.debug_fields({})
            assert mock_send.call_args[0][0]["path"] == "/rest/api/2/field"

        # Cloud
        with (
            patch.object(handler, "is_cloud", True),
            patch.object(handler, "_send") as mock_send,
            patch.object(handler, "_recv", return_value=mock_response),
        ):
            tools_cache.debug_fields({})
            assert mock_send.call_args[0][0]["path"] == "/rest/api/3/field"
