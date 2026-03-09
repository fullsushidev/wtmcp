# Jira Plugin Context

## Tool Usage Guidelines

### Efficient Querying

- Use JQL to get exactly what's needed in a single call
- `resolution = EMPTY` finds all open tickets regardless of status
- Combine conditions: `assignee = currentUser() AND resolution = EMPTY ORDER BY priority DESC`
- Don't re-query the same data — use cached results from previous calls

### Write Operations

All write tools default to `dry_run=true`. Always preview before
executing writes:

1. Call the tool with `dry_run: true` (default)
2. Show the preview to the user
3. Only set `dry_run: false` after explicit approval

This applies to: create_issue, add_comment, edit_comment,
transition_issue, assign_issue, set_priority, set_labels,
add_labels, remove_labels, set_text_field, set_custom_field,
set_story_points, set_components, add_issue_link, issue_worklog,
add_issues_to_sprint, add_issues_to_backlog, add_attachment,
delete_attachment.

### Assignee Aliases

Use `"me"`, `"myself"`, or `"currentUser"` for self-assignment in
jira_get_user, jira_assign_issue, and JQL queries.

### Search Patterns

**My open tickets:**
```
jql: "assignee = currentUser() AND resolution = EMPTY ORDER BY priority DESC, updated DESC"
```

**Current sprint tickets:**
```
jql: "assignee = currentUser() AND sprint in openSprints() ORDER BY status ASC"
```

**Recent activity:**
```
jql: "assignee = currentUser() AND updated >= -7d ORDER BY updated DESC"
```

**Team sprint:**
```
jql: "sprint = 'Sprint Name' ORDER BY assignee ASC, status ASC"
```

### Sprint Operations

To find sprint names, use `jira_list_available_sprints` first, then
reference the exact sprint name in `jira_get_sprint_issues` or
`jira_search_by_sprint`.

For sprint filtering with JQL, use `sprint = 'Exact Sprint Name'`
or `sprint in openSprints()`.

### Export and Local Analysis

For large datasets, use the export/cache workflow:

1. `jira_export_sprint_data` — save sprint data to a local file
2. `jira_query_local_sprint_data` — filter locally without API calls
3. `jira_sprint_metrics_summary` — calculate completion metrics
4. `jira_compare_sprints` — compare metrics across sprint files

Use `jira_read_cache_summary` and `jira_get_issue_from_cache` to
read exported data efficiently instead of reading raw JSON files.

### Custom Fields

Use `jira_debug_fields` with a search term to find field IDs:
```
jira_debug_fields(search="sprint")    → find sprint field ID
jira_debug_fields(search="story")     → find story points field
jira_debug_fields(search="team")      → find team field
```

Then use `jira_set_custom_field` with the discovered field ID.

### Cloud vs Server

The plugin auto-detects Cloud vs Server at startup. Differences
handled automatically:

- Comments and descriptions use ADF format on Cloud, plain text on Server
- User assignment uses accountId on Cloud, username on Server
- Sprint report (Greenhopper API) is only available on Server/DC

### Attachments

Use `file_path` with an **absolute path** when attaching files.
Relative paths won't resolve correctly since the plugin runs
from a different working directory:
```
jira_add_attachment(issue_key="PROJ-123",
                    file_path="/home/user/path/to/file.png")
```

### Brief Mode

Search and sprint tools return compact summaries by default
(`brief: true`): key, summary, status, assignee, priority.
Set `brief: false` for full Jira issue data when needed.
