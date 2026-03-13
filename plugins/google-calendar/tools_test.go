package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"google.golang.org/api/calendar/v3"
	"google.golang.org/api/option"
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

// --- toolUpdateEvent dry_run tests ---

func TestUpdateEventDryRun(t *testing.T) {
	params := mustJSON(t, map[string]any{
		"event_id": "evt-789",
		"summary":  "Updated meeting",
		"location": "Room B",
	})

	result, err := toolUpdateEvent(params, nil)
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
	if m["action"] != "calendar_update_event" {
		t.Errorf("action = %v", m["action"])
	}
	if m["event_id"] != "evt-789" {
		t.Errorf("event_id = %v", m["event_id"])
	}
	changes, ok := m["changes"].(map[string]any)
	if !ok {
		t.Fatalf("changes type = %T", m["changes"])
	}
	if changes["summary"] != "Updated meeting" {
		t.Errorf("changes[summary] = %v", changes["summary"])
	}
	if changes["location"] != "Room B" {
		t.Errorf("changes[location] = %v", changes["location"])
	}
}

func TestUpdateEventDryRunNoServiceAccess(t *testing.T) {
	// calendarSvc is nil — this test verifies that dry_run does
	// NOT access the service (would panic if it did).
	params := mustJSON(t, map[string]any{
		"event_id": "evt-999",
		"summary":  "Safe update",
	})

	result, err := toolUpdateEvent(params, nil)
	if err != nil {
		t.Fatalf("dry_run should not access service: %v", err)
	}
	m := result.(map[string]any)
	if m["dry_run"] != true {
		t.Error("expected dry_run=true")
	}
}

func TestUpdateEventMissingEventID(t *testing.T) {
	params := mustJSON(t, map[string]any{
		"summary": "No event ID",
	})

	_, err := toolUpdateEvent(params, nil)
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

// --- httptest integration tests ---

func setupCalendarTest(t *testing.T, handler http.Handler) {
	t.Helper()
	ts := httptest.NewServer(handler)
	t.Cleanup(ts.Close)

	svc, err := calendar.NewService(context.Background(),
		option.WithHTTPClient(ts.Client()),
		option.WithEndpoint(ts.URL))
	if err != nil {
		t.Fatal(err)
	}
	calendarSvc = svc
}

func TestToolGetEvents(t *testing.T) {
	setupCalendarTest(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"items":[{"id":"evt1","summary":"Standup","start":{"dateTime":"2026-03-12T09:00:00Z"},"end":{"dateTime":"2026-03-12T09:15:00Z"}}]}`)
	}))

	result, err := toolGetEvents(mustJSON(t, map[string]any{}), nil)
	if err != nil {
		t.Fatalf("toolGetEvents: %v", err)
	}

	events, ok := result.(*calendar.Events)
	if !ok {
		t.Fatalf("result type = %T", result)
	}
	if len(events.Items) != 1 {
		t.Fatalf("got %d events, want 1", len(events.Items))
	}
	if events.Items[0].Summary != "Standup" {
		t.Errorf("summary = %q", events.Items[0].Summary)
	}
}

func TestToolGetEvent(t *testing.T) {
	setupCalendarTest(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"id":"evt1","summary":"Meeting","start":{"dateTime":"2026-03-12T14:00:00Z"}}`)
	}))

	result, err := toolGetEvent(mustJSON(t, map[string]any{
		"event_id": "evt1",
	}), nil)
	if err != nil {
		t.Fatalf("toolGetEvent: %v", err)
	}

	event, ok := result.(*calendar.Event)
	if !ok {
		t.Fatalf("result type = %T", result)
	}
	if event.Summary != "Meeting" {
		t.Errorf("summary = %q", event.Summary)
	}
}

func TestToolGetEventMissingID(t *testing.T) {
	_, err := toolGetEvent(mustJSON(t, map[string]any{}), nil)
	if err == nil {
		t.Fatal("expected error for missing event_id")
	}
}

func TestToolCreateEventFull(t *testing.T) {
	setupCalendarTest(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"id":"new-evt","summary":"Design review","status":"confirmed"}`)
	}))

	result, err := toolCreateEvent(mustJSON(t, map[string]any{
		"summary":        "Design review",
		"start_datetime": "2026-03-12T14:00:00Z",
		"end_datetime":   "2026-03-12T15:00:00Z",
		"dry_run":        false,
	}), nil)
	if err != nil {
		t.Fatalf("toolCreateEvent: %v", err)
	}

	event, ok := result.(*calendar.Event)
	if !ok {
		t.Fatalf("result type = %T", result)
	}
	if event.Id != "new-evt" {
		t.Errorf("id = %q", event.Id)
	}
}

func TestToolDeleteEventFull(t *testing.T) {
	setupCalendarTest(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	result, err := toolDeleteEvent(mustJSON(t, map[string]any{
		"event_id": "evt-del",
		"dry_run":  false,
	}), nil)
	if err != nil {
		t.Fatalf("toolDeleteEvent: %v", err)
	}

	m, ok := result.(map[string]any)
	if !ok {
		t.Fatalf("result type = %T", result)
	}
	if m["success"] != true {
		t.Errorf("success = %v", m["success"])
	}
}

func TestToolSearchEvents(t *testing.T) {
	setupCalendarTest(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"items":[{"id":"evt1","summary":"Standup"}]}`)
	}))

	result, err := toolSearchEvents(mustJSON(t, map[string]any{
		"query": "standup",
	}), nil)
	if err != nil {
		t.Fatalf("toolSearchEvents: %v", err)
	}

	events, ok := result.(*calendar.Events)
	if !ok {
		t.Fatalf("result type = %T", result)
	}
	if len(events.Items) != 1 {
		t.Fatalf("got %d events, want 1", len(events.Items))
	}
}

func TestToolSearchEventsMissingQuery(t *testing.T) {
	_, err := toolSearchEvents(mustJSON(t, map[string]any{}), nil)
	if err == nil {
		t.Fatal("expected error for missing query")
	}
}

func TestToolGetCalendars(t *testing.T) {
	setupCalendarTest(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"items":[{"id":"primary","summary":"My Calendar","primary":true}]}`)
	}))

	result, err := toolGetCalendars(nil, nil)
	if err != nil {
		t.Fatalf("toolGetCalendars: %v", err)
	}

	list, ok := result.(*calendar.CalendarList)
	if !ok {
		t.Fatalf("result type = %T", result)
	}
	if len(list.Items) != 1 {
		t.Fatalf("got %d calendars, want 1", len(list.Items))
	}
	if !list.Items[0].Primary {
		t.Error("expected primary calendar")
	}
}
