---
title: "push"
description: "Pushes local commits to remote for all tracked repositories."
category: "Remote Operations"
order: 3
usage: "git-wrangler push [--force]"
---

# push

Pushes local commits to remote for all tracked repositories.

## Usage

```bash
git-wrangler push [--force]
```

## What it does

Iterates through Git repositories found in the current directory and its immediate subdirectories, checks if there are changes to push, and performs a `git push origin HEAD`. Repositories that are already up to date are reported and skipped.

## Options

| Flag | Required | Description |
|---|---|---|
| `--force` | Optional | Forcefully push changes, overwriting remote branches if necessary. |

## Examples

```bash
# Standard push
git-wrangler push

# Force push (use with caution — rewrites remote history)
git-wrangler push --force
```

## Notes

> **Warning:** `--force` rewrites remote branch history. Only use this after a deliberate history-rewriting operation (e.g. `git-wrangler rewrite-authors`).
