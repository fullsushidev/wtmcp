# Google Calendar Plugin

Provides 8 tools for Google Calendar event management.

## Write Safety

All write tools default to `dry_run: true`. Always preview first:

1. Call the tool with `dry_run: true` (default)
2. Show the preview to the user
3. Only set `dry_run: false` after explicit approval

This applies to: calendar_create_event, calendar_update_event,
calendar_delete_event.

## Time Format

All times use RFC3339 with timezone:
- `2026-03-10T09:00:00-05:00` (US Eastern)
- `2026-03-10T14:00:00Z` (UTC)
- `2026-03-10T14:00:00+01:00` (CET)

For all-day events, use date only: `2026-03-10`
(set `all_day: true` in create/update).

## Common Patterns

**Upcoming events this week:**
```
calendar_get_events(time_min="2026-03-10T00:00:00Z",
                    time_max="2026-03-16T23:59:59Z")
```

**Find a meeting by name:**
```
calendar_search_events(query="standup")
```

**Check availability before scheduling:**
```
calendar_get_free_busy(time_min="2026-03-11T08:00:00Z",
                       time_max="2026-03-11T18:00:00Z")
```

**List all calendars to find IDs:**
```
calendar_get_calendars()
```

Then use a specific calendar:
```
calendar_get_events(calendar_id="team@group.calendar.google.com")
```

## Calendar IDs

- `"primary"` — the user's main calendar (default for all tools)
- Use `calendar_get_calendars` to discover shared/team calendars
- Holiday calendars like `en.portuguese#holiday@group.v.calendar.google.com`
  are read-only

## Tips

- `calendar_get_events` defaults to events from now onwards
- Search with `calendar_search_events` matches text in summaries
  and descriptions
- When creating events with attendees, pass a list of email addresses
- The `calendar_get_free_busy` tool checks multiple calendars at once
  when given a list of `calendar_ids`
