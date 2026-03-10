# Google Calendar Plugin

Provides 8 tools for Google Calendar event management.

## Tools

- **calendar_get_events** — List upcoming events (defaults to now, primary calendar)
- **calendar_get_event** — Fetch a single event by ID
- **calendar_create_event** — Create an event (dry_run by default)
- **calendar_update_event** — Update event fields (dry_run by default)
- **calendar_delete_event** — Delete an event (dry_run by default)
- **calendar_get_calendars** — List all accessible calendars
- **calendar_search_events** — Search events by text
- **calendar_get_free_busy** — Check free/busy status

## Usage Notes

- Times are in RFC3339 format (e.g., `2024-01-15T09:00:00-05:00`)
- Write operations (create, update, delete) default to `dry_run: true`
  Set `dry_run: false` to actually perform the action
- Calendar ID defaults to `"primary"` (the user's main calendar)
- Use `calendar_get_calendars` to discover available calendar IDs
