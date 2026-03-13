package main

import (
	"encoding/json"
	"testing"

	"google.golang.org/api/calendar/v3"
)

func mustJSON(t *testing.T, v any) json.RawMessage {
	t.Helper()
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

// --- toolCreateEvent dry_run tests ---

func TestCreateEventDryRunMinimal(t *testing.T) {
	params := mustJSON(t, map[string]any{
		"summary":        "Design review",
		"start_datetime": "2026-03-12T14:00:00Z",
		"end_datetime":   "2026-03-12T15:00:00Z",
	})

	result, err := toolCreateEvent(params, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	m, ok := result.(map[string]any)
	if !ok {
		t.Fatalf("expected map, got %T", result)
	}
	if m["dry_run"] != true {
		t.Error("expected dry_run=true")
	}
	if m["action"] != "calendar_create_event" {
		t.Errorf("action = %v", m["action"])
	}
	if m["calendar_id"] != "primary" {
		t.Errorf("calendar_id = %v, want primary", m["calendar_id"])
	}
}

func TestCreateEventDryRunAllFields(t *testing.T) {
	params := mustJSON(t, map[string]any{
		"summary":        "Team offsite",
		"start_datetime": "2026-03-15T09:00:00Z",
		"end_datetime":   "2026-03-15T17:00:00Z",
		"calendar_id":    "team@group.calendar.google.com",
		"description":    "Annual planning",
		"location":       "Room 42",
		"attendees":      []string{"alice@example.com", "bob@example.com"},
	})

	result, err := toolCreateEvent(params, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	m := result.(map[string]any)
	if m["calendar_id"] != "team@group.calendar.google.com" {
		t.Errorf("calendar_id = %v", m["calendar_id"])
	}
}

func TestCreateEventDryRunAllDay(t *testing.T) {
	params := mustJSON(t, map[string]any{
		"summary":        "Holiday",
		"start_datetime": "2026-12-25",
		"end_datetime":   "2026-12-26",
		"all_day":        true,
	})

	result, err := toolCreateEvent(params, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	m := result.(map[string]any)
	if m["dry_run"] != true {
		t.Error("expected dry_run=true")
	}
}

func TestCreateEventMissingSummary(t *testing.T) {
	params := mustJSON(t, map[string]any{
		"start_datetime": "2026-03-12T14:00:00Z",
		"end_datetime":   "2026-03-12T15:00:00Z",
	})

	_, err := toolCreateEvent(params, nil)
	if err == nil {
		t.Fatal("expected error for missing summary")
	}
}

func TestCreateEventMissingTimes(t *testing.T) {
	params := mustJSON(t, map[string]any{
		"summary": "Meeting",
	})

	_, err := toolCreateEvent(params, nil)
	if err == nil {
		t.Fatal("expected error for missing start/end times")
	}
}

func TestCreateEventDryRunDefault(t *testing.T) {
	// dry_run should default to true when omitted
	params := mustJSON(t, map[string]any{
		"summary":        "Meeting",
		"start_datetime": "2026-03-12T14:00:00Z",
		"end_datetime":   "2026-03-12T15:00:00Z",
	})

	result, err := toolCreateEvent(params, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	m := result.(map[string]any)
	if m["dry_run"] != true {
		t.Error("dry_run should default to true")
	}
}

// --- toolDeleteEvent dry_run tests ---

func TestDeleteEventDryRun(t *testing.T) {
	params := mustJSON(t, map[string]any{
		"event_id": "evt-123",
	})

	result, err := toolDeleteEvent(params, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	m, ok := result.(map[string]any)
	if !ok {
		t.Fatalf("expected map, got %T", result)
	}
	if m["dry_run"] != true {
		t.Error("expected dry_run=true")
	}
	if m["action"] != "calendar_delete_event" {
		t.Errorf("action = %v", m["action"])
	}
	if m["event_id"] != "evt-123" {
		t.Errorf("event_id = %v", m["event_id"])
	}
	if m["calendar_id"] != "primary" {
		t.Errorf("calendar_id = %v, want primary (default)", m["calendar_id"])
	}
}

func TestDeleteEventDryRunCustomCalendar(t *testing.T) {
	params := mustJSON(t, map[string]any{
		"event_id":    "evt-456",
		"calendar_id": "work@group.calendar.google.com",
	})

	result, err := toolDeleteEvent(params, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	m := result.(map[string]any)
	if m["calendar_id"] != "work@group.calendar.google.com" {
		t.Errorf("calendar_id = %v", m["calendar_id"])
	}
}

func TestDeleteEventMissingEventID(t *testing.T) {
	params := mustJSON(t, map[string]any{})

	_, err := toolDeleteEvent(params, nil)
	if err == nil {
		t.Fatal("expected error for missing event_id")
	}
}

// --- buildEvent tests ---

func TestBuildEventMinimal(t *testing.T) {
	event := buildEvent(createEventParams{
		Summary: "Standup",
		Start:   "2026-03-12T09:00:00Z",
		End:     "2026-03-12T09:15:00Z",
	})

	if event.Summary != "Standup" {
		t.Errorf("Summary = %q", event.Summary)
	}
	if event.Start.DateTime != "2026-03-12T09:00:00Z" {
		t.Errorf("Start.DateTime = %q", event.Start.DateTime)
	}
	if event.End.DateTime != "2026-03-12T09:15:00Z" {
		t.Errorf("End.DateTime = %q", event.End.DateTime)
	}
	if event.Description != "" {
		t.Errorf("Description should be empty, got %q", event.Description)
	}
}

func TestBuildEventAllDay(t *testing.T) {
	event := buildEvent(createEventParams{
		Summary: "Holiday",
		Start:   "2026-12-25",
		End:     "2026-12-26",
		AllDay:  true,
	})

	if event.Start.Date != "2026-12-25" {
		t.Errorf("Start.Date = %q", event.Start.Date)
	}
	if event.Start.DateTime != "" {
		t.Errorf("Start.DateTime should be empty for all-day, got %q", event.Start.DateTime)
	}
	if event.End.Date != "2026-12-26" {
		t.Errorf("End.Date = %q", event.End.Date)
	}
}

func TestBuildEventAllFields(t *testing.T) {
	event := buildEvent(createEventParams{
		Summary:   "Team meeting",
		Start:     "2026-03-12T14:00:00Z",
		End:       "2026-03-12T15:00:00Z",
		Desc:      "Weekly sync",
		Location:  "Room 42",
		Attendees: []string{"alice@example.com", "bob@example.com"},
	})

	if event.Description != "Weekly sync" {
		t.Errorf("Description = %q", event.Description)
	}
	if event.Location != "Room 42" {
		t.Errorf("Location = %q", event.Location)
	}
	if len(event.Attendees) != 2 {
		t.Fatalf("Attendees count = %d, want 2", len(event.Attendees))
	}
	if event.Attendees[0].Email != "alice@example.com" {
		t.Errorf("Attendees[0].Email = %q", event.Attendees[0].Email)
	}
}

// --- applyEventUpdates tests ---

func TestApplyEventUpdatesSummaryOnly(t *testing.T) {
	event := &calendar.Event{
		Summary:     "Old title",
		Description: "Keep this",
	}
	changes := applyEventUpdates(event, updateEventParams{
		Summary: "New title",
	})

	if event.Summary != "New title" {
		t.Errorf("Summary = %q", event.Summary)
	}
	if event.Description != "Keep this" {
		t.Errorf("Description should be unchanged, got %q", event.Description)
	}
	if changes["summary"] != "New title" {
		t.Errorf("changes[summary] = %v", changes["summary"])
	}
	if _, ok := changes["description"]; ok {
		t.Error("description should not be in changes")
	}
}

func TestApplyEventUpdatesMultipleFields(t *testing.T) {
	event := &calendar.Event{Summary: "Meeting"}
	changes := applyEventUpdates(event, updateEventParams{
		Desc:     "Updated description",
		Location: "New room",
	})

	if event.Description != "Updated description" {
		t.Errorf("Description = %q", event.Description)
	}
	if event.Location != "New room" {
		t.Errorf("Location = %q", event.Location)
	}
	if len(changes) != 2 {
		t.Errorf("expected 2 changes, got %d", len(changes))
	}
}

func TestApplyEventUpdatesTimeFields(t *testing.T) {
	event := &calendar.Event{
		Summary: "Meeting",
		Start:   &calendar.EventDateTime{DateTime: "2026-03-12T14:00:00Z"},
		End:     &calendar.EventDateTime{DateTime: "2026-03-12T15:00:00Z"},
	}
	changes := applyEventUpdates(event, updateEventParams{
		Start: "2026-03-12T16:00:00Z",
		End:   "2026-03-12T17:00:00Z",
	})

	if event.Start.DateTime != "2026-03-12T16:00:00Z" {
		t.Errorf("Start.DateTime = %q", event.Start.DateTime)
	}
	if event.End.DateTime != "2026-03-12T17:00:00Z" {
		t.Errorf("End.DateTime = %q", event.End.DateTime)
	}
	if changes["start"] != "2026-03-12T16:00:00Z" {
		t.Errorf("changes[start] = %v", changes["start"])
	}
}

func TestApplyEventUpdatesAllDayToggle(t *testing.T) {
	event := &calendar.Event{
		Summary: "Meeting",
		Start:   &calendar.EventDateTime{DateTime: "2026-03-12T14:00:00Z"},
	}
	changes := applyEventUpdates(event, updateEventParams{
		Start:  "2026-03-12",
		End:    "2026-03-13",
		AllDay: true,
	})

	if event.Start.Date != "2026-03-12" {
		t.Errorf("Start.Date = %q", event.Start.Date)
	}
	if event.Start.DateTime != "" {
		t.Errorf("Start.DateTime should be empty for all-day, got %q", event.Start.DateTime)
	}
	if len(changes) != 2 {
		t.Errorf("expected 2 changes, got %d", len(changes))
	}
}

func TestApplyEventUpdatesNoChanges(t *testing.T) {
	event := &calendar.Event{Summary: "Meeting"}
	changes := applyEventUpdates(event, updateEventParams{})

	if len(changes) != 0 {
		t.Errorf("expected 0 changes, got %d: %v", len(changes), changes)
	}
}
