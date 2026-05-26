---
title: "reset"
description: "Resets the current branch to exactly match its remote counterpart."
category: "Local Operations"
order: 7
usage: "git-wrangler reset [--yes]"
---

# reset

Resets the current branch to exactly match its remote counterpart.

## Usage

```bash
git-wrangler reset [--yes]
```

## What it does

Fetches the latest remote state and hard-resets the current branch to `origin/<current-branch>`, discarding any local commits and uncommitted changes. Before resetting, the command shows divergence status (ahead/behind counts) and prompts for confirmation unless `--yes` is supplied.

## Options

| Flag    | Required | Description                               |
| ------- | -------- | ----------------------------------------- |
| `--yes` | Optional | Skip the interactive confirmation prompt. |

## Examples

```bash
# Interactive reset (shows divergence + prompts)
git-wrangler reset

# Non-interactive (use in scripts)
git-wrangler reset --yes
```

## Notes

> **Warning:** This is a destructive operation. All local commits and uncommitted changes will be discarded permanently.

- Repositories already in sync with their remote are skipped
- Repositories in detached HEAD state are skipped with a warning
- Repositories with no upstream tracking branch are skipped
