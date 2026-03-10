# Gmail Plugin

Provides 8 tools for Gmail email management with built-in
safeguards against token explosion.

## Write Safety

All write tools default to `dry_run: true`. Always preview first:

1. Call the tool with `dry_run: true` (default)
2. Show the preview to the user
3. Only set `dry_run: false` after explicit approval

This applies to: gmail_send_message, gmail_create_draft,
gmail_modify_labels.

## Reading Email — Recommended Workflow

1. **List message IDs** with `gmail_list_messages`
2. **Get summaries** with `gmail_get_messages_summary` (lightweight)
3. **Cache if needed** with `gmail_fetch_and_cache` for bulk processing

Avoid `gmail_get_messages` — it returns full payloads and can cause
token explosion. Use `gmail_get_messages_summary` instead.

## Search Queries

Gmail search syntax works in the `query` parameter:

```
from:alice@example.com
to:me subject:report
has:attachment filename:pdf
after:2026/01/01 before:2026/03/01
is:unread
label:important
in:sent
newer_than:7d
```

Combine with spaces (AND) or `OR`:
```
from:alice OR from:bob subject:meeting
```

## Hard Limits

- `gmail_list_messages`: max 100 messages
- `gmail_get_messages_summary`: max 100 messages
- `gmail_get_messages`: max 20 messages (deprecated)
- `gmail_fetch_and_cache`: max 200 messages

## Caching

`gmail_fetch_and_cache` saves full message data to a local file
and returns only summaries (first 10). Use file operations to
process the cached data locally without consuming context tokens.

## Labels

Use `gmail_list_labels` to discover label IDs, then:
- Add labels: `gmail_modify_labels(message_id="...", add_labels=["STARRED"])`
- Archive: `gmail_modify_labels(message_id="...", remove_labels=["INBOX"])`
- Mark read: `gmail_modify_labels(message_id="...", remove_labels=["UNREAD"])`

Common system labels: INBOX, SENT, DRAFT, SPAM, TRASH, UNREAD,
STARRED, IMPORTANT.
