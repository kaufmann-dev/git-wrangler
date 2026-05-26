---
title: "rewrite-dates"
description: "Redistributes commit timestamps to mimic natural human activity."
category: "History Rewriting"
order: 4
usage: "git-wrangler rewrite-dates [--start-date YYYY-MM-DD] [--end-date YYYY-MM-DD] [--yes]"
---

# rewrite-dates

Redistributes commit timestamps across a date range and applies the rewrite with `git-filter-repo`.

## Usage

```bash
git-wrangler rewrite-dates [--start-date YYYY-MM-DD] [--end-date YYYY-MM-DD] [--yes]
```

## Options

- `--start-date YYYY-MM-DD` sets the earliest date. Defaults to the oldest commit timestamp.
- `--end-date YYYY-MM-DD` sets the latest date. Defaults to the newest commit timestamp.
- `--yes` skips the interactive confirmation.

## Prerequisites

- `git`
- `git-filter-repo`

## Example

```bash
git-wrangler rewrite-dates --start-date 2024-01-01 --end-date 2024-06-30
```

> **Warning:** This rewrites Git history. You will need to force-push to update remotes.
