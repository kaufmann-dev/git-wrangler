---
title: "rename-branch"
description: "Renames a specified branch to a new name across repositories."
category: "Local Operations"
order: 6
usage: "wrangler rename-branch --oldbranch <old_name> --newbranch <new_name>"
---

# rename-branch

Renames a specified branch to a new name across repositories.

## Usage

```bash
wrangler rename-branch --oldbranch <old_name> --newbranch <new_name>
```

## What it does

Renames a specified branch to a new name across all managed Git repositories. Repositories where the old branch does not exist or the new branch name is already taken are skipped automatically.

## Options

| Flag | Required | Description |
|---|---|---|
| `--oldbranch <name>` | **Required** | The name of the existing branch to rename. |
| `--newbranch <name>` | **Required** | The new name for the branch. |

## Example

```bash
# Rename 'master' to 'main' across all repos
wrangler rename-branch --oldbranch master --newbranch main
```

## Notes

- Repositories where the old branch doesn't exist are skipped with a yellow warning
- Repositories where the new branch name already exists are also skipped
- This only renames the **local** branch — you'll need `wrangler push --force` to update the remote
