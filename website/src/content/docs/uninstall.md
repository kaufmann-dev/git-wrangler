---
title: "uninstall"
description: "Uninstalls Git Wrangler from the system."
category: "Utility"
order: 4
usage: "git-wrangler uninstall [--confirm]"
---

# uninstall

Uninstalls Git Wrangler from the system.

## Usage

```bash
git-wrangler uninstall [--confirm]
```

## What it does

Removes the Git Wrangler installation directory (`~/.git-wrangler`) and the `git-wrangler` symlink from the bin directory. Prompts for confirmation before proceeding unless `--confirm` is supplied.

## Options

| Flag | Required | Description |
|---|---|---|
| `--confirm` | Optional | Skip the interactive confirmation prompt. |

## Examples

```bash
# Interactive uninstall (prompts for confirmation)
git-wrangler uninstall

# Non-interactive
git-wrangler uninstall --confirm
```

## Notes

- Only removes the installation directory and symlink — no other files are touched
- If the symlink target is not a symlink (e.g. you copied the binary), it is skipped with a warning
