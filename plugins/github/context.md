# GitHub Plugin

Provides task discovery and investigation tools for GitHub.

## Quick Start

- **What do I need to work on?** → `github_my_work` (unified view)
- **PRs waiting for my review?** → `github_my_prs_to_review`
- **Issues assigned to me?** → `github_my_issues`
- **Notifications?** → `github_my_notifications`

## Investigation Flow

Once you find an item of interest:

1. Get PR details: `github_get_pr` (stats, reviewers, merge status)
2. See what changed: `github_get_pr_files` (diffs per file)
3. Read reviews: `github_get_pr_reviews` (approval status)
4. Read inline comments: `github_get_pr_review_comments`
5. Read conversation: `github_get_comments`
6. See commits: `github_get_pr_commits`

For issues, use `github_get_issue` then `github_get_comments`.

## Search Syntax

`github_search` accepts GitHub search qualifiers:

- `is:pr is:open review-requested:username` — PRs needing review
- `is:issue is:open assignee:username` — assigned issues
- `is:pr author:username` — your PRs
- `involves:username` — everything involving you
- `repo:owner/name` — scope to a repo
- `org:orgname` — scope to an org
- `label:bug` — filter by label
- `created:>2024-01-01` — date filters

## Pagination

Tools that return lists support `max_results` and `start_at`.
GitHub's search API is limited to 1000 results total (10 pages
of 100).

## Limitations

- `github_my_notifications` requires a **classic PAT** with the
  `notifications` scope. Fine-grained PATs do not support
  the notifications API.
- All tools are **read-only**. Commenting, approving, and merging
  are not yet supported.
- Rate limit: 5000 requests/hour for authenticated users. The
  plugin includes rate limit warnings when remaining calls drop
  below 100.
