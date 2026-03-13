package main

import (
	"encoding/json"
	"fmt"
	"time"

	"google.golang.org/api/calendar/v3"
	googleapi "google.golang.org/api/googleapi"
)

const (
	eventFields     googleapi.Field = "id,summary,start,end,location,description,attendees/email,htmlLink,status"
	eventListFields googleapi.Field = "items(id,summary,start,end,location,description,attendees/email,htmlLink,status),nextPageToken"
	calendarFields  googleapi.Field = "items(id,summary,description,primary,timeZone,accessRole)"
	freeBusyFields  googleapi.Field = "calendars,timeMin,timeMax"
)

type getEventsParams struct {
	CalendarID string `json:"calendar_id"`
	MaxResults int    `json:"max_results"`
	TimeMin    string `json:"time_min"`
	TimeMax    string `json:"time_max"`
}

func toolGetEvents(params, _ json.RawMessage) (any, error) {
	var p getEventsParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, fmt.Errorf("parse params: %w", err)
	}
	if p.CalendarID == "" {
		p.CalendarID = "primary"
	}
	if p.MaxResults == 0 {
		p.MaxResults = 10
	}
	if p.TimeMin == "" {
		p.TimeMin = time.Now().UTC().Format(time.RFC3339)
	}

	call := calendarSvc.Events.List(p.CalendarID).
		Fields(eventListFields).
		TimeMin(p.TimeMin).
		MaxResults(int64(p.MaxResults)).
		SingleEvents(true).
		OrderBy("startTime")
	if p.TimeMax != "" {
		call = call.TimeMax(p.TimeMax)
	}

	res, err := call.Do()
	if err != nil {
		return nil, fmt.Errorf("list events: %w", err)
	}
	return res, nil
}

type getEventParams struct {
	EventID    string `json:"event_id"`
	CalendarID string `json:"calendar_id"`
}

func toolGetEvent(params, _ json.RawMessage) (any, error) {
	var p getEventParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, fmt.Errorf("parse params: %w", err)
	}
	if p.EventID == "" {
		return nil, fmt.Errorf("event_id is required")
	}
	if p.CalendarID == "" {
		p.CalendarID = "primary"
	}

	res, err := calendarSvc.Events.Get(p.CalendarID, p.EventID).Fields(eventFields).Do()
	if err != nil {
		return nil, fmt.Errorf("get event: %w", err)
	}
	return res, nil
}

type createEventParams struct {
	Summary   string   `json:"summary"`
	Start     string   `json:"start_datetime"`
	End       string   `json:"end_datetime"`
	Calendar  string   `json:"calendar_id"`
	Desc      string   `json:"description"`
	Location  string   `json:"location"`
	Attendees []string `json:"attendees"`
	AllDay    bool     `json:"all_day"`
	DryRun    bool     `json:"dry_run"`
}

func toolCreateEvent(params, _ json.RawMessage) (any, error) {
	p := createEventParams{DryRun: true}
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, fmt.Errorf("parse params: %w", err)
	}
	if p.Summary == "" {
		return nil, fmt.Errorf("summary is required")
	}
	if p.Start == "" || p.End == "" {
		return nil, fmt.Errorf("start_datetime and end_datetime are required")
	}
	if p.Calendar == "" {
		p.Calendar = "primary"
	}

	event := buildEvent(p)

	if p.DryRun {
		return map[string]any{
			"dry_run":     true,
			"action":      "calendar_create_event",
			"calendar_id": p.Calendar,
			"event":       event,
		}, nil
	}

	res, err := calendarSvc.Events.Insert(p.Calendar, event).Fields(eventFields).Do()
	if err != nil {
		return nil, fmt.Errorf("create event: %w", err)
	}
	return res, nil
}

type updateEventParams struct {
	EventID  string `json:"event_id"`
	Calendar string `json:"calendar_id"`
	Summary  string `json:"summary"`
	Start    string `json:"start_datetime"`
	End      string `json:"end_datetime"`
	Desc     string `json:"description"`
	Location string `json:"location"`
	AllDay   bool   `json:"all_day"`
	DryRun   bool   `json:"dry_run"`
}

func toolUpdateEvent(params, _ json.RawMessage) (any, error) {
	p := updateEventParams{DryRun: true}
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, fmt.Errorf("parse params: %w", err)
	}
	if p.EventID == "" {
		return nil, fmt.Errorf("event_id is required")
	}
	if p.Calendar == "" {
		p.Calendar = "primary"
	}

	// Fetch existing event
	event, err := calendarSvc.Events.Get(p.Calendar, p.EventID).Do()
	if err != nil {
		return nil, fmt.Errorf("get event for update: %w", err)
	}

	changes := applyEventUpdates(event, p)

	if p.DryRun {
		return map[string]any{
			"dry_run":     true,
			"action":      "calendar_update_event",
			"calendar_id": p.Calendar,
			"event_id":    p.EventID,
			"changes":     changes,
		}, nil
	}

	res, err := calendarSvc.Events.Update(p.Calendar, p.EventID, event).Fields(eventFields).Do()
	if err != nil {
		return nil, fmt.Errorf("update event: %w", err)
	}
	return res, nil
}

type deleteEventParams struct {
	EventID  string `json:"event_id"`
	Calendar string `json:"calendar_id"`
	DryRun   bool   `json:"dry_run"`
}

func toolDeleteEvent(params, _ json.RawMessage) (any, error) {
	p := deleteEventParams{DryRun: true}
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, fmt.Errorf("parse params: %w", err)
	}
	if p.EventID == "" {
		return nil, fmt.Errorf("event_id is required")
	}
	if p.Calendar == "" {
		p.Calendar = "primary"
	}

	if p.DryRun {
		return map[string]any{
			"dry_run":     true,
			"action":      "calendar_delete_event",
			"calendar_id": p.Calendar,
			"event_id":    p.EventID,
		}, nil
	}

	if err := calendarSvc.Events.Delete(p.Calendar, p.EventID).Do(); err != nil {
		return nil, fmt.Errorf("delete event: %w", err)
	}
	return map[string]any{
		"success": true,
		"message": fmt.Sprintf("Event %s deleted", p.EventID),
	}, nil
}

func toolGetCalendars(_, _ json.RawMessage) (any, error) {
	res, err := calendarSvc.CalendarList.List().Fields(calendarFields).Do()
	if err != nil {
		return nil, fmt.Errorf("list calendars: %w", err)
	}
	return res, nil
}

type searchEventsParams struct {
	Query      string `json:"query"`
	CalendarID string `json:"calendar_id"`
	MaxResults int    `json:"max_results"`
}

func toolSearchEvents(params, _ json.RawMessage) (any, error) {
	var p searchEventsParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, fmt.Errorf("parse params: %w", err)
	}
	if p.Query == "" {
		return nil, fmt.Errorf("query is required")
	}
	if p.CalendarID == "" {
		p.CalendarID = "primary"
	}
	if p.MaxResults == 0 {
		p.MaxResults = 10
	}

	res, err := calendarSvc.Events.List(p.CalendarID).
		Fields(eventListFields).
		Q(p.Query).
		MaxResults(int64(p.MaxResults)).
		SingleEvents(true).
		Do()
	if err != nil {
		return nil, fmt.Errorf("search events: %w", err)
	}
	return res, nil
}

type freeBusyParams struct {
	TimeMin     string   `json:"time_min"`
	TimeMax     string   `json:"time_max"`
	CalendarIDs []string `json:"calendar_ids"`
}

func toolGetFreeBusy(params, _ json.RawMessage) (any, error) {
	var p freeBusyParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, fmt.Errorf("parse params: %w", err)
	}
	if p.TimeMin == "" || p.TimeMax == "" {
		return nil, fmt.Errorf("time_min and time_max are required")
	}
	if len(p.CalendarIDs) == 0 {
		p.CalendarIDs = []string{"primary"}
	}

	items := make([]*calendar.FreeBusyRequestItem, len(p.CalendarIDs))
	for i, id := range p.CalendarIDs {
		items[i] = &calendar.FreeBusyRequestItem{Id: id}
	}

	res, err := calendarSvc.Freebusy.Query(&calendar.FreeBusyRequest{
		TimeMin: p.TimeMin,
		TimeMax: p.TimeMax,
		Items:   items,
	}).Fields(freeBusyFields).Do()
	if err != nil {
		return nil, fmt.Errorf("freebusy query: %w", err)
	}
	return res, nil
}

// buildEvent constructs a calendar.Event from creation parameters.
// Pure function — no service access.
func buildEvent(p createEventParams) *calendar.Event {
	event := &calendar.Event{Summary: p.Summary}

	if p.AllDay {
		event.Start = &calendar.EventDateTime{Date: p.Start}
		event.End = &calendar.EventDateTime{Date: p.End}
	} else {
		event.Start = &calendar.EventDateTime{DateTime: p.Start}
		event.End = &calendar.EventDateTime{DateTime: p.End}
	}

	if p.Desc != "" {
		event.Description = p.Desc
	}
	if p.Location != "" {
		event.Location = p.Location
	}
	for _, email := range p.Attendees {
		event.Attendees = append(event.Attendees, &calendar.EventAttendee{Email: email})
	}

	return event
}

// applyEventUpdates applies partial updates to an existing event
// and returns a map of changed fields. Pure function — modifies
// the event in place but does not access any service.
func applyEventUpdates(event *calendar.Event, p updateEventParams) map[string]any {
	changes := map[string]any{}

	if p.Summary != "" {
		event.Summary = p.Summary
		changes["summary"] = p.Summary
	}
	if p.Desc != "" {
		event.Description = p.Desc
		changes["description"] = p.Desc
	}
	if p.Location != "" {
		event.Location = p.Location
		changes["location"] = p.Location
	}
	if p.Start != "" {
		if p.AllDay {
			event.Start = &calendar.EventDateTime{Date: p.Start}
		} else {
			event.Start = &calendar.EventDateTime{DateTime: p.Start}
		}
		changes["start"] = p.Start
	}
	if p.End != "" {
		if p.AllDay {
			event.End = &calendar.EventDateTime{Date: p.End}
		} else {
			event.End = &calendar.EventDateTime{DateTime: p.End}
		}
		changes["end"] = p.End
	}

	return changes
}
