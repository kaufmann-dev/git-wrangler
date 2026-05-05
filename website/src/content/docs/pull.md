---
title: "pull"
description: "Pulls the latest changes for all tracked repositories."
category: "Remote Operations"
order: 2
usage: "wrangler pull [--rebase] [--force]"
---

# pull

Pulls the latest changes for all tracked repositories.

## Usage

```bash
wrangler pull [--rebase] [--force]
```

## What it does

Iterates through Git repositories found in the current directory and its immediate subdirectories, and performs a Git pull operation to fetch and integrate changes from the remote repository. Repositories that are already up to date are reported and skipped.

## Options

| Flag | Required | Description |
|---|---|---|
| `--rebase` | Optional | Rebase local commits on top of the fetched remote branch instead of merging. |
| `--force` | Optional | Forcefully pull changes, overwriting local changes if necessary. |

## Examples

```bash
# Standard pull (merge strategy)
wrangler pull

# Pull with rebase
wrangler pull --rebase

# Force pull (overwrites local changes)
wrangler pull --force
```
