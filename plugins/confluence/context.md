# Confluence Plugin

## Tool Usage Guidelines

### Write Safety

All write tools default to `dry_run: true`. Always preview first:

1. Call the tool with `dry_run: true` (default)
2. Show the preview to the user
3. Only set `dry_run: false` after explicit approval

This applies to: confluence_create_page, confluence_update_page,
confluence_add_comment.

### Discovering Spaces

Use `confluence_get_spaces` first to find available spaces and
their keys. Space keys are short identifiers like `ENG`, `HR`,
`TEAM` used in all space-scoped operations.

### Finding Pages

**By title (exact match, single page):**
```
confluence_get_page_by_title(title="Meeting Notes", space_key="ENG")
```

**By title (all matches):**
```
confluence_get_pages_by_title(title="Meeting Notes", space_key="ENG")
```

**By ID (when you have it from a previous call):**
```
confluence_get_page(page_id="12345")
```

**By search (CQL):**
```
confluence_search(cql="type=page AND space=ENG AND title~'meeting'")
```

### CQL Search Patterns

CQL (Confluence Query Language) is used with `confluence_search`:

**Find pages in a space:**
```
cql: "type=page AND space=ENG"
```

**Full-text search:**
```
cql: "type=page AND text~'deployment guide'"
```

**Title search (fuzzy):**
```
cql: "type=page AND title~'architecture'"
```

**Recently modified:**
```
cql: "type=page AND space=ENG AND lastModified > now('-7d')"
```

**By label:**
```
cql: "type=page AND label='meeting-notes'"
```

**By author:**
```
cql: "type=page AND creator='jsmith'"
```

**Combining conditions:**
```
cql: "type=page AND space=ENG AND label='design' AND lastModified > now('-30d') ORDER BY lastModified DESC"
```

### Page Hierarchy

Confluence pages are organized in a tree structure:

- Use `confluence_get_page_children(page_id="...")` to list child pages
- Use `parent_id` in `confluence_create_page` to create under a parent
- Use `confluence_get_page_history` to see version history

### Content Format

Confluence uses **storage format** (a subset of XHTML) for page
content. When creating or updating pages:

```html
<p>Regular paragraph</p>
<h1>Heading</h1>
<ul><li>List item</li></ul>
<ac:structured-macro ac:name="code">
  <ac:plain-text-body><![CDATA[code here]]></ac:plain-text-body>
</ac:structured-macro>
```

The body returned by get/search tools includes the storage format
in `body.storage.value`.

### Updating Pages

`confluence_update_page` automatically fetches the current page
version and increments it. You must provide both `title` and
`body` — even if only changing one, pass the current value for
the other.

### Page IDs

Page IDs come from search results, get_page_by_title, or
get_page_children. Don't fabricate IDs — always get them from
a previous call.
