"""Tests for Confluence handler tool functions.

Mocks the http() function to test tool logic without a real
Confluence server.
"""

from unittest import mock

import handler


def _mock_http(status, body):
    """Create a mock http() that returns a fixed response."""
    return mock.patch.object(handler, "http", return_value=(status, body, {}))


class TestGetPage:
    def test_returns_page(self):
        page = {"id": "123", "title": "Test", "body": {"storage": {"value": "<p>hi</p>"}}}
        with _mock_http(200, page):
            result = handler.confluence_get_page({"page_id": "123"})
        assert result["id"] == "123"
        assert result["title"] == "Test"

    def test_missing_page_id(self):
        try:
            handler.confluence_get_page({})
            assert False, "should raise"
        except ValueError as e:
            assert "page_id" in str(e)

    def test_http_error(self):
        with _mock_http(404, {"message": "not found"}):
            result = handler.confluence_get_page({"page_id": "999"})
        assert "error" in result
        assert "404" in result["error"]


class TestGetPageByTitle:
    def test_returns_first_match(self):
        body = {"results": [{"id": "1", "title": "A"}, {"id": "2", "title": "B"}]}
        with _mock_http(200, body):
            result = handler.confluence_get_page_by_title({"title": "A", "space_key": "ENG"})
        assert result["id"] == "1"

    def test_no_results(self):
        with _mock_http(200, {"results": []}):
            result = handler.confluence_get_page_by_title({"title": "Missing", "space_key": "ENG"})
        assert "error" in result
        assert "No page found" in result["error"]

    def test_missing_params(self):
        try:
            handler.confluence_get_page_by_title({"title": "A"})
            assert False, "should raise"
        except ValueError:
            pass


class TestSearch:
    def test_returns_results(self):
        body = {"results": [{"id": "1"}], "size": 1}
        with _mock_http(200, body):
            result = handler.confluence_search({"cql": "type=page"})
        assert "results" in result

    def test_missing_cql(self):
        try:
            handler.confluence_search({})
            assert False, "should raise"
        except ValueError as e:
            assert "cql" in str(e)


class TestGetPagesByTitle:
    def test_returns_list(self):
        body = {"results": [{"id": "1"}, {"id": "2"}]}
        with _mock_http(200, body):
            result = handler.confluence_get_pages_by_title({"title": "Test", "space_key": "ENG"})
        assert result["count"] == 2
        assert len(result["results"]) == 2


class TestGetSpaces:
    def test_returns_spaces(self):
        body = {"results": [{"key": "ENG", "name": "Engineering"}]}
        with _mock_http(200, body):
            result = handler.confluence_get_spaces({"limit": 10})
        assert "results" in result


class TestGetSpace:
    def test_returns_space(self):
        body = {"key": "ENG", "name": "Engineering"}
        with _mock_http(200, body):
            result = handler.confluence_get_space({"space_key": "ENG"})
        assert result["key"] == "ENG"

    def test_missing_space_key(self):
        try:
            handler.confluence_get_space({})
            assert False, "should raise"
        except ValueError:
            pass


class TestCreatePage:
    def test_dry_run_default(self):
        result = handler.confluence_create_page(
            {
                "space_key": "ENG",
                "title": "New Page",
                "body": "<p>Content</p>",
            }
        )
        assert result["dry_run"] is True
        assert result["action"] == "confluence_create_page"
        assert result["title"] == "New Page"

    def test_dry_run_with_parent(self):
        result = handler.confluence_create_page(
            {
                "space_key": "ENG",
                "title": "Child",
                "body": "<p>Content</p>",
                "parent_id": "42",
            }
        )
        assert result["parent_id"] == "42"

    def test_creates_with_ancestors(self):
        created = {"id": "100", "title": "New Page"}
        with _mock_http(200, created) as mock_fn:
            result = handler.confluence_create_page(
                {
                    "space_key": "ENG",
                    "title": "New Page",
                    "body": "<p>Content</p>",
                    "parent_id": "42",
                    "dry_run": False,
                }
            )
        assert result["id"] == "100"
        call_body = mock_fn.call_args[1].get("body") or mock_fn.call_args[0][2]
        # Verify ancestors array was sent
        if isinstance(call_body, dict):
            assert call_body["ancestors"] == [{"id": "42"}]

    def test_body_preview_truncation(self):
        long_body = "<p>" + "x" * 300 + "</p>"
        result = handler.confluence_create_page(
            {
                "space_key": "ENG",
                "title": "Long",
                "body": long_body,
            }
        )
        assert result["body_preview"].endswith("...")
        assert len(result["body_preview"]) <= 203  # 200 + "..."


class TestUpdatePage:
    def test_dry_run_default(self):
        result = handler.confluence_update_page(
            {
                "page_id": "123",
                "title": "Updated",
                "body": "<p>New content</p>",
            }
        )
        assert result["dry_run"] is True
        assert result["page_id"] == "123"

    def test_fetches_version_and_increments(self):
        current = {"version": {"number": 5}}
        updated = {"id": "123", "version": {"number": 6}}

        call_count = [0]
        responses = [(200, current, {}), (200, updated, {})]

        def mock_http(*args, **kwargs):
            resp = responses[call_count[0]]
            call_count[0] += 1
            return resp

        with mock.patch.object(handler, "http", side_effect=mock_http):
            result = handler.confluence_update_page(
                {
                    "page_id": "123",
                    "title": "Updated",
                    "body": "<p>New</p>",
                    "dry_run": False,
                }
            )
        assert result["version"]["number"] == 6
        assert call_count[0] == 2  # GET version + PUT update


class TestAddComment:
    def test_dry_run_default(self):
        result = handler.confluence_add_comment(
            {
                "page_id": "123",
                "comment": "<p>Nice work!</p>",
            }
        )
        assert result["dry_run"] is True
        assert result["page_id"] == "123"

    def test_posts_comment(self):
        created = {"id": "456", "type": "comment"}
        with _mock_http(200, created):
            result = handler.confluence_add_comment(
                {
                    "page_id": "123",
                    "comment": "<p>Note</p>",
                    "dry_run": False,
                }
            )
        assert result["id"] == "456"


class TestGetPageChildren:
    def test_dict_response(self):
        body = {"results": [{"id": "1"}, {"id": "2"}]}
        with _mock_http(200, body):
            result = handler.confluence_get_page_children({"page_id": "123"})
        assert result["count"] == 2

    def test_list_response(self):
        body = [{"id": "1"}]
        with _mock_http(200, body):
            result = handler.confluence_get_page_children({"page_id": "123"})
        assert result["count"] == 1

    def test_missing_page_id(self):
        try:
            handler.confluence_get_page_children({})
            assert False, "should raise"
        except ValueError:
            pass


class TestGetPageHistory:
    def test_returns_history(self):
        body = {"lastUpdated": {"number": 5, "by": {"displayName": "Alice"}}}
        with _mock_http(200, body):
            result = handler.confluence_get_page_history({"page_id": "123"})
        assert "lastUpdated" in result
