---
title: "push"
description: "Pushes local commits to remote for all tracked repositories."
category: "Remote Operations"
order: 3
usage: "git-wrangler push [--force] [--force-unsafe] [--yes]"
---

# push

Pushes local commits to remote for all tracked repositories.

## Usage

```bash
git-wrangler push [--force] [--force-unsafe] [--yes]
```

## What it does

Iterates through Git repositories found under the current directory, checks if there are changes to push, and performs a `git push origin HEAD`. `--force` uses `--force-with-lease`; `--force-unsafe` performs a raw force push only after confirmation. Repositories that are already up to date are reported and skipped.

## Options

| Flag             | Required | Description                                         |
| ---------------- | -------- | --------------------------------------------------- |
| `--force`        | Optional | Forcefully push changes using `--force-with-lease`. |
| `--force-unsafe` | Optional | Perform a raw `--force` push after confirmation.    |
| `--yes`          | Optional | Skip the raw force-push confirmation prompt.        |

## Examples

```bash
# Standard push
git-wrangler push

# Lease-safe force push
git-wrangler push --force

# Raw force push after explicit confirmation
git-wrangler push --force-unsafe --yes
```

## Notes

> **Warning:** `--force-unsafe` rewrites remote branch history without the lease protection used by `--force`. Only use it after a deliberate history-rewriting operation and when you are certain no remote work will be overwritten.
