package main

import (
	"testing"
	"time"

	gogitlab "gitlab.com/gitlab-org/api/client-go"
)

func TestTimeStrNil(t *testing.T) {
	got := timeStr(nil)
	if got != "" {
		t.Errorf("timeStr(nil) = %q, want empty", got)
	}
}

func TestTimeStrValid(t *testing.T) {
	ts := time.Date(2026, 3, 10, 14, 30, 0, 0, time.UTC)
	got := timeStr(&ts)
	want := "2026-03-10T14:30:00Z"
	if got != want {
		t.Errorf("timeStr() = %q, want %q", got, want)
	}
}

func TestIssueToMapFull(t *testing.T) {
	created := time.Date(2026, 1, 15, 10, 0, 0, 0, time.UTC)
	updated := time.Date(2026, 3, 10, 14, 0, 0, 0, time.UTC)
	due := gogitlab.ISOTime(time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC))
	issue := &gogitlab.Issue{
		IID:         42,
		Title:       "Fix login bug",
		Description: "The login page crashes on mobile",
		State:       "opened",
		CreatedAt:   &created,
		UpdatedAt:   &updated,
		WebURL:      "https://gitlab.example.com/proj/-/issues/42",
		Labels:      gogitlab.Labels{"bug", "critical"},
		Author:      &gogitlab.IssueAuthor{Username: "alice"},
		Assignees: []*gogitlab.IssueAssignee{
			{Username: "bob"},
			{Username: "carol"},
		},
		Milestone: &gogitlab.Milestone{Title: "v2.0"},
		DueDate:   &due,
	}

	m := issueToMap(issue)

	if m["iid"] != int64(42) {
		t.Errorf("iid = %v", m["iid"])
	}
	if m["title"] != "Fix login bug" {
		t.Errorf("title = %v", m["title"])
	}
	if m["state"] != "opened" {
		t.Errorf("state = %v", m["state"])
	}
	if m["author"] != "alice" {
		t.Errorf("author = %v", m["author"])
	}
	assignees, ok := m["assignees"].([]string)
	if !ok {
		t.Fatalf("assignees type = %T", m["assignees"])
	}
	if len(assignees) != 2 || assignees[0] != "bob" {
		t.Errorf("assignees = %v", assignees)
	}
	if m["milestone"] != "v2.0" {
		t.Errorf("milestone = %v", m["milestone"])
	}
	if _, ok := m["due_date"]; !ok {
		t.Error("expected due_date key")
	}
	labels, ok := m["labels"].(gogitlab.Labels)
	if !ok {
		t.Fatalf("labels type = %T", m["labels"])
	}
	if len(labels) != 2 {
		t.Errorf("labels = %v", labels)
	}
}

func TestIssueToMapNilFields(t *testing.T) {
	issue := &gogitlab.Issue{
		IID:   1,
		Title: "Minimal",
		State: "closed",
	}

	m := issueToMap(issue)

	if m["iid"] != int64(1) {
		t.Errorf("iid = %v", m["iid"])
	}
	// Author, Milestone, DueDate should not panic
	if _, ok := m["author"]; ok {
		t.Error("author should not be set for nil Author")
	}
	if _, ok := m["milestone"]; ok {
		t.Error("milestone should not be set for nil Milestone")
	}
	if _, ok := m["due_date"]; ok {
		t.Error("due_date should not be set for nil DueDate")
	}
}

func TestIssueToMapNoAssignees(t *testing.T) {
	issue := &gogitlab.Issue{
		IID:   2,
		Title: "Unassigned",
		State: "opened",
	}

	m := issueToMap(issue)

	assignees, ok := m["assignees"].([]string)
	if ok && len(assignees) > 0 {
		t.Errorf("expected nil/empty assignees, got %v", assignees)
	}
}

func TestMRListResultMultiple(t *testing.T) {
	created := time.Date(2026, 3, 1, 10, 0, 0, 0, time.UTC)
	updated := time.Date(2026, 3, 10, 14, 0, 0, 0, time.UTC)
	mrs := []*gogitlab.BasicMergeRequest{
		{
			IID:          10,
			Title:        "Add feature X",
			State:        "merged",
			WebURL:       "https://gitlab.example.com/proj/-/merge_requests/10",
			SourceBranch: "feature-x",
			TargetBranch: "main",
			Author:       &gogitlab.BasicUser{Username: "alice"},
			CreatedAt:    &created,
			UpdatedAt:    &updated,
			Draft:        false,
			Labels:       gogitlab.Labels{"enhancement"},
		},
		{
			IID:          11,
			Title:        "Fix bug Y",
			State:        "opened",
			WebURL:       "https://gitlab.example.com/proj/-/merge_requests/11",
			SourceBranch: "fix-y",
			TargetBranch: "main",
			Author:       &gogitlab.BasicUser{Username: "bob"},
			CreatedAt:    &created,
			UpdatedAt:    &updated,
			Draft:        true,
			Labels:       gogitlab.Labels{},
		},
	}

	result := mrListResult(mrs)

	if result["total"] != 2 {
		t.Errorf("total = %v, want 2", result["total"])
	}
	items, ok := result["merge_requests"].([]map[string]any)
	if !ok {
		t.Fatalf("merge_requests type = %T", result["merge_requests"])
	}
	if len(items) != 2 {
		t.Fatalf("got %d items, want 2", len(items))
	}
	if items[0]["iid"] != int64(10) {
		t.Errorf("first MR iid = %v", items[0]["iid"])
	}
	if items[0]["author"] != "alice" {
		t.Errorf("first MR author = %v", items[0]["author"])
	}
	if items[0]["draft"] != false {
		t.Errorf("first MR draft = %v", items[0]["draft"])
	}
	if items[1]["draft"] != true {
		t.Errorf("second MR draft = %v", items[1]["draft"])
	}
}

func TestMRListResultEmpty(t *testing.T) {
	result := mrListResult(nil)

	if result["total"] != 0 {
		t.Errorf("total = %v, want 0", result["total"])
	}
}

func TestMRListResultNilAuthor(t *testing.T) {
	mrs := []*gogitlab.BasicMergeRequest{
		{
			IID:   1,
			Title: "No author",
			State: "opened",
		},
	}

	result := mrListResult(mrs)
	items := result["merge_requests"].([]map[string]any)
	if items[0]["author"] != "" {
		t.Errorf("author should be empty for nil Author, got %v", items[0]["author"])
	}
}
