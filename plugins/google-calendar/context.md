# Google Calendar Plugin

## Tool Usage Guidelines

### Write Safety

All write tools default to `dry_run: true`. Always preview first:

1. Call the tool with `dry_run: true` (default)
2. Show the preview to the user
3. Only set `dry_run: false` after explicit approval

This applies to: calendar_create_event, calendar_update_event,
calendar_delete_event.

### Time Format

All times use RFC3339 with timezone offset or Z for UTC:
- `2026-03-10T09:00:00-05:00` (with offset)
- `2026-03-10T14:00:00Z` (UTC)

For all-day events, use date only: `2026-03-10`
(set `all_day: true` in create/update).

### Discovering Calendars

Always use `calendar_get_calendars` first to discover available
calendars and their IDs. The `"primary"` calendar is the user's
main calendar and is the default for all tools.

Holiday and shared calendars have long IDs like
`en.portuguese#holiday@group.v.calendar.google.com` — use
`calendar_get_calendars` to find them rather than guessing.

### Common Patterns

**Today's events:**
```
calendar_get_events()
```
Events from now onwards are returned by default.

**Events in a date range:**
```
calendar_get_events(time_min="2026-03-10T00:00:00Z",
                    time_max="2026-03-14T23:59:59Z",
                    max_results=50)
```

**Find a meeting by name:**
```
calendar_search_events(query="standup")
```
Searches summaries and descriptions.

**Check availability before scheduling:**
```
calendar_get_free_busy(time_min="2026-03-11T08:00:00Z",
                       time_max="2026-03-11T18:00:00Z")
```
Pass multiple `calendar_ids` to check team availability.

**Events from a specific calendar:**
```
calendar_get_events(calendar_id="team@group.calendar.google.com")
```

### Creating Events

Minimum required: `summary`, `start_datetime`, `end_datetime`:
```
calendar_create_event(
    summary="Design review",
    start_datetime="2026-03-12T14:00:00Z",
    end_datetime="2026-03-12T15:00:00Z")
```

With attendees and location:
```
calendar_create_event(
    summary="Design review",
    start_datetime="2026-03-12T14:00:00Z",
    end_datetime="2026-03-12T15:00:00Z",
    location="Room 42",
    attendees=["alice@example.com", "bob@example.com"])
```

All-day event:
```
calendar_create_event(
    summary="Team offsite",
    start_datetime="2026-03-15",
    end_datetime="2026-03-16",
    all_day=true)
```

### Updating Events

You need the `event_id` from a previous get/list/search call.
Only specify fields you want to change:
```
calendar_update_event(
    event_id="abc123",
    summary="Updated title",
    location="New room")
```

### Event IDs

Event IDs come from `calendar_get_events`, `calendar_search_events`,
or `calendar_get_event`. Don't fabricate IDs — always get them from
a previous call.

### Response Fields

Responses are trimmed to essential fields: `id`, `summary`, `start`,
`end`, `location`, `description`, `attendees` (email only),
`htmlLink`, `status`. Full Google Calendar metadata is not returned.
