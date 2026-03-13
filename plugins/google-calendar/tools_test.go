package main

import (
	"encoding/json"
	"testing"
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
