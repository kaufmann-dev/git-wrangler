---
title: "update"
description: "Updates Git Wrangler to the latest version."
category: "Utility"
order: 3
usage: "git-wrangler update [--confirm]"
---

# update

Updates Git Wrangler to the latest version.

## Usage

```bash
git-wrangler update [--confirm]
```

## What it does

Checks if a newer version of Git Wrangler is available on GitHub by comparing the local commit hash against the remote. If an update is available, it shows the difference and prompts for confirmation before applying.

## Options

| Flag | Required | Description |
|---|---|---|
| `--confirm` | Optional | Skip the interactive confirmation prompt. |

## Examples

```bash
# Interactive update (prompts before applying)
git-wrangler update

# Non-interactive (e.g. in scripts/CI)
git-wrangler update --confirm
```

## Notes

- Requires an internet connection to reach GitHub
- Uses `git fetch` + `git reset --hard` — local modifications to the install directory will be overwritten
- Script permissions (`chmod +x`) are restored automatically after the update
