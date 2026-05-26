---
title: "status"
description: "Shows dirty/clean and ahead/behind status of tracked repositories."
category: "Utility"
order: 1
usage: "git-wrangler status"
---

# status

Shows dirty/clean and ahead/behind status of tracked repositories.

## Usage

```bash
git-wrangler status
```

This command takes no arguments.

## What it does

Iterates through Git repositories found under the current directory and displays a unified dashboard of their real-time sync state, dirty/clean working tree status, and commit metrics.

## Example output

```
REPOSITORY                     | STATE | TRACKING
-------------------------------+-------+------------------------
my-api                         | clean | up to date
frontend-app                   | dirty | behind 3
design-system                  | clean | ahead 2
old-project                    | clean | no remote
-------------------------------+-------+------------------------
Summary: 1 dirty, 1 behind, 1 no remote
```

## Example

```bash
git-wrangler status
```

## Notes

- **dirty** — the working tree has uncommitted changes or untracked files
- **ahead N** — you have N local commits not yet pushed to the remote
- **behind N** — the remote has N commits you haven't pulled yet
- **no remote** — no upstream tracking branch is configured
- The summary line is written to stderr so it doesn't interfere with piping:

```bash
# Filter for only dirty repos
git-wrangler status | grep "dirty"
```
