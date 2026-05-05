---
title: "rewrite-dates"
description: "Redistributes commit timestamps to mimic natural human activity."
category: "History Rewriting"
order: 3
usage: "wrangler rewrite-dates [--start-date <YYYY-MM-DD>] [--end-date <YYYY-MM-DD>] [--confirm]"
---

# rewrite-dates

Redistributes commit timestamps to mimic natural human activity.

## Usage

```bash
wrangler rewrite-dates [--start-date <YYYY-MM-DD>] [--end-date <YYYY-MM-DD>] [--confirm]
```

## What it does

Rewrites commit author and committer dates across all managed repositories using a statistically natural redistribution algorithm. Timestamps are spread across the given date range with:

- **Stratified jitter** — commits are evenly distributed across time slots, then randomly offset within each slot
- **Bimodal time-of-day weighting** — peaks at 10am and 3pm, matching real developer activity
- **Weekend dampening** — 65% of weekend commits are moved to adjacent Friday evening or Monday morning
- **Micro-clustering** — same-day commits are spaced 25–90 minutes apart

## Options

| Flag | Required | Description |
|---|---|---|
| `--start-date <YYYY-MM-DD>` | Optional | Earliest date for redistributed commits. Defaults to the first commit date. |
| `--end-date <YYYY-MM-DD>` | Optional | Latest date for redistributed commits. Defaults to the last commit date. |
| `--confirm` | Optional | Skip the interactive confirmation prompt. |

## Prerequisites

- `git-filter-repo` must be installed
- Python 3 must be available

## Example

```bash
wrangler rewrite-dates --start-date 2024-01-01 --end-date 2024-12-31 --confirm
```

## Notes

> **Warning:** This rewrites Git history. You will need to force-push to update remotes.

- Repositories with fewer than 2 commits are skipped
- A before/after summary is printed for review before the rewrite is applied
- The repository's original timezone offset is inferred and preserved
