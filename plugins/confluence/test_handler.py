"""Tests for Confluence handler tool functions.

Mocks the http() and cache functions to test tool logic without
a real Confluence server.
"""

from unittest import mock

import handler


def _mock_http(status, body):
    """Create a mock http() that returns a fixed response."""
    return mock.patch.object(handler, "http", return_value=(status, body, {}))


def _mock_cache_miss():
    """Mock cache_get to always miss, cache_set to no-op."""
    return [
        mock.patch.object(handler, "cache_get", return_value=None),
        mock.patch.object(handler, "cache_set"),
    ]


def _raw_page(page_id="123", title="Test", body_content="<p>hi</p>"):
    """Build a raw Confluence page response with typical bloat."""
    return {
        "id": page_id,
        "title": title,
        "status": "current",
        "space": {"key": "ENG", "name": "Engineering", "_links": {"self": "/spaces/ENG"}, "_expandable": {}},
        "version": {"number": 5, "when": "2026-03-10T10:00:00Z", "by": {"displayName": "Alice", "accountId": "abc"}},
        "body": {"storage": {"value": body_content, "_expandable": {"view": ""}}, "_expandable": {"export_view": ""}},
        "_links": {"webui": "/pages/123", "base": "https://confluence.example.com", "self": "/rest/api/content/123"},
        "_expandable": {"children": "", "ancestors": ""},
    }


class TestStripMetadata:
    def test_removes_links(self):
        data = {"id": "1", "_links": {"self": "/foo"}, "title": "Test"}
        result = handler._strip(data)
        assert "_links" not in result
        assert result["id"] == "1"

    def test_removes_expandable(self):
        data = {"id": "1", "_expandable": {"foo": ""}}
        result = handler._strip(data)
        assert "_expandable" not in result

    def test_recursive(self):
        data = {"a": {"_links": {}, "b": "keep"}, "c": [{"_expandable": {}, "d": 1}]}
        result = handler._strip(data)
        assert "_links" not in result["a"]
        assert result["a"]["b"] == "keep"
        assert "_expandable" not in result["c"][0]
        assert result["c"][0]["d"] == 1


class TestExtractPage:
    def test_extracts_fields(self):
        page = _raw_page()
        result = handler._extract_page(page)
        assert result["id"] == "123"
        assert result["title"] == "Test"
        assert result["space_key"] == "ENG"
        assert result["version"] == 5
        assert result["modified_by"] == "Alice"
        assert result["url"] == "https://confluence.example.com/pages/123"
        assert result["body"] == "<p>hi</p>"
        assert "_links" not in result
        assert "_expandable" not in result

    def test_without_body(self):
        page = _raw_page()
        result = handler._extract_page(page, include_body=False)
        assert "body" not in result
        assert result["title"] == "Test"


class TestGetPage:
    def test_returns_extracted(self):
        with _mock_http(200, _raw_page()):
            result = handler.confluence_get_page({"page_id": "123"})
        assert result["id"] == "123"
        assert result["space_key"] == "ENG"
        assert "_links" not in result

    def test_include_body_false(self):
        page = _raw_page()
        del page["body"]  # simulate no body expand
        with _mock_http(200, page):
            result = handler.confluence_get_page({"page_id": "123", "include_body": False})
        assert "body" not in result

    def test_missing_page_id(self):
        try:
            handler.confluence_get_page({})
            assert False, "should raise"
        except ValueError as e:
            assert "page_id" in str(e)

    def test_http_error(self):
        with _mock_http(404, {"message": "not found", "_links": {"self": "/foo"}}):
            result = handler.confluence_get_page({"page_id": "999"})
        assert "error" in result
        assert "_links" not in result.get("details", {})


class TestGetPageByTitle:
    def test_returns_extracted(self):
        body = {"results": [_raw_page()]}
        with _mock_http(200, body):
            result = handler.confluence_get_page_by_title({"title": "Test", "space_key": "ENG"})
        assert result["id"] == "123"
        assert "_links" not in result

    def test_no_results(self):
        with _mock_http(200, {"results": []}):
            result = handler.confluence_get_page_by_title({"title": "Missing", "space_key": "ENG"})
        assert "error" in result


class TestSearch:
    def test_returns_brief_results(self):
        raw = {
            "results": [
                {
                    "content": {
                        "id": "1",
                        "title": "Page A",
                        "type": "page",
                        "space": {"key": "ENG"},
                        "_links": {"webui": "/pages/1"},
                    },
                    "excerpt": "some text here",
                    "_links": {"base": "https://confluence.example.com"},
                }
            ],
            "totalSize": 1,
        }
        patches = _mock_cache_miss()
        with patches[0], patches[1], _mock_http(200, raw):
            result = handler.confluence_search({"cql": "type=page"})
        assert result["count"] == 1
        assert result["results"][0]["id"] == "1"
        assert result["results"][0]["space_key"] == "ENG"
        assert "_links" not in result["results"][0]

    def test_caches_results(self):
        patches = _mock_cache_miss()
        raw = {"results": [], "totalSize": 0}
        with patches[0], patches[1] as mock_set, _mock_http(200, raw):
            handler.confluence_search({"cql": "type=page"})
        mock_set.assert_called_once()
        assert mock_set.call_args[1]["ttl"] == 300


class TestGetPagesByTitle:
    def test_returns_without_body(self):
        body = {"results": [_raw_page(), _raw_page(page_id="456", title="Test 2")]}
        with _mock_http(200, body):
            result = handler.confluence_get_pages_by_title({"title": "Test", "space_key": "ENG"})
        assert result["count"] == 2
        assert "body" not in result["results"][0]


class TestGetSpaces:
    def test_returns_extracted(self):
        raw = {
            "results": [
                {
                    "key": "ENG",
                    "name": "Engineering",
                    "type": "global",
                    "description": {"plain": {"value": "Engineering space"}},
                    "_links": {"self": "/spaces/ENG"},
                    "_expandable": {},
                }
            ]
        }
        patches = _mock_cache_miss()
        with patches[0], patches[1], _mock_http(200, raw):
            result = handler.confluence_get_spaces({})
        assert result["spaces"][0]["key"] == "ENG"
        assert "_links" not in result

    def test_caches_result(self):
        patches = _mock_cache_miss()
        raw = {"results": []}
        with patches[0], patches[1] as mock_set, _mock_http(200, raw):
            handler.confluence_get_spaces({})
        mock_set.assert_called_once()
        assert mock_set.call_args[1]["ttl"] == 1800

    def test_returns_cached(self):
        cached = {"spaces": [{"key": "ENG"}], "count": 1}
        with mock.patch.object(handler, "cache_get", return_value=cached):
            result = handler.confluence_get_spaces({})
        assert result == cached


class TestGetSpace:
    def test_returns_extracted(self):
        raw = {
            "key": "ENG",
            "name": "Engineering",
            "type": "global",
            "description": {"plain": {"value": "Full description here"}},
            "homepage": {"id": "100", "title": "Home"},
            "_links": {},
        }
        patches = _mock_cache_miss()
        with patches[0], patches[1], _mock_http(200, raw):
            result = handler.confluence_get_space({"space_key": "ENG"})
        assert result["key"] == "ENG"
        assert result["description"] == "Full description here"
        assert result["homepage_id"] == "100"


class TestCreatePage:
    def test_dry_run(self):
        result = handler.confluence_create_page({"space_key": "ENG", "title": "New", "body": "<p>Content</p>"})
        assert result["dry_run"] is True

    def test_returns_slim(self):
        raw = _raw_page(page_id="100", title="New")
        with _mock_http(200, raw):
            result = handler.confluence_create_page(
                {"space_key": "ENG", "title": "New", "body": "<p>Content</p>", "dry_run": False}
            )
        assert result["id"] == "100"
        assert "body" not in result
        assert "_links" not in result


class TestUpdatePage:
    def test_dry_run(self):
        result = handler.confluence_update_page({"page_id": "123", "title": "Updated", "body": "<p>New</p>"})
        assert result["dry_run"] is True

    def test_returns_slim(self):
        current = {"version": {"number": 5}}
        updated = _raw_page(page_id="123", title="Updated")
        call_count = [0]
        responses = [(200, current, {}), (200, updated, {})]

        def mock_http(*args, **kwargs):
            resp = responses[call_count[0]]
            call_count[0] += 1
            return resp

        with mock.patch.object(handler, "http", side_effect=mock_http):
            result = handler.confluence_update_page(
                {"page_id": "123", "title": "Updated", "body": "<p>New</p>", "dry_run": False}
            )
        assert result["id"] == "123"
        assert "body" not in result


class TestAddComment:
    def test_dry_run(self):
        result = handler.confluence_add_comment({"page_id": "123", "comment": "<p>Nice!</p>"})
        assert result["dry_run"] is True

    def test_returns_slim(self):
        raw = {
            "id": "456",
            "title": "Re: Page",
            "version": {"number": 1},
            "_links": {"webui": "/comment", "base": "https://example.com"},
        }
        with _mock_http(200, raw):
            result = handler.confluence_add_comment({"page_id": "123", "comment": "<p>Note</p>", "dry_run": False})
        assert result["id"] == "456"
        assert "_links" not in result


class TestGetPageChildren:
    def test_returns_slim(self):
        raw = {"results": [{"id": "1", "title": "Child A", "status": "current", "_links": {}, "_expandable": {}}]}
        patches = _mock_cache_miss()
        with patches[0], patches[1], _mock_http(200, raw):
            result = handler.confluence_get_page_children({"page_id": "123"})
        assert result["count"] == 1
        assert result["results"][0] == {"id": "1", "title": "Child A", "status": "current"}

    def test_caches_result(self):
        patches = _mock_cache_miss()
        raw = {"results": []}
        with patches[0], patches[1] as mock_set, _mock_http(200, raw):
            handler.confluence_get_page_children({"page_id": "123"})
        mock_set.assert_called_once()
        assert mock_set.call_args[0][0] == "children:123"


class TestGetPageHistory:
    def test_strips_metadata(self):
        raw = {"lastUpdated": {"number": 5, "by": {"displayName": "Alice"}}, "_links": {"self": "/history"}}
        with _mock_http(200, raw):
            result = handler.confluence_get_page_history({"page_id": "123"})
        assert "lastUpdated" in result
        assert "_links" not in result
