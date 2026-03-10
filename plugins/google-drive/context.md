# Google Drive Plugin

Provides 9 tools for Google Drive file access, search, and export.

## Read-Only Access

This plugin has read-only access to Google Drive. It cannot create,
modify, or delete files.

## Search Patterns

**Find files by name:**
```
drive_search_text(text="quarterly report")
```

**Find my recent documents:**
```
drive_search_text(text="design", owners=["me"])
```

**Find Google Docs only:**
```
drive_search_text(text="meeting notes",
                  mime_types=["application/vnd.google-apps.document"])
```

**Raw Drive query (advanced):**
```
drive_search_files(query="name contains 'design' and trashed = false and modifiedTime > '2026-01-01'")
```

## File ID Extraction

Several tools accept Google Drive/Docs URLs directly:
- `drive_get_file_by_url` — get metadata from a URL
- `drive_extract_and_get_from_text` — scan text for Drive links
- `drive_export_google_doc_markdown` — accepts URL or file ID

Supported URL formats:
- `https://docs.google.com/document/d/FILE_ID/edit`
- `https://docs.google.com/spreadsheets/d/FILE_ID/edit`
- `https://drive.google.com/file/d/FILE_ID/view`

## Export Tools

| Tool | Source | Output |
|------|--------|--------|
| `drive_export_google_doc_text` | Google Doc | Plain text (UTF-8) |
| `drive_export_google_doc_markdown` | Google Doc | Text saved to local file |
| `drive_export_google_sheet_csv` | Google Sheet | CSV (first worksheet) |
| `drive_export_slides_pdf` | Google Slides | PDF (base64 encoded) |

## Saving Exports Locally

`drive_export_google_doc_markdown` saves to `./drive/<title>.md` by
default. This avoids consuming context tokens with large documents.
Set `save_to_file: false` to get content directly (not recommended
for large documents).

## Common MIME Types

- `application/vnd.google-apps.document` — Google Docs
- `application/vnd.google-apps.spreadsheet` — Google Sheets
- `application/vnd.google-apps.presentation` — Google Slides
- `application/vnd.google-apps.folder` — Folder
- `application/pdf` — PDF files
