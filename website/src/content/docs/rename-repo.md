---
title: "rename-repo"
description: "Bulk renames GitHub repositories and optionally updates their descriptions."
category: "Remote Operations"
order: 4
usage: "git-wrangler rename-repo [--description]"
---

# rename-repo

Bulk renames GitHub repositories and optionally updates their descriptions.

## Usage

```bash
git-wrangler rename-repo [--description]
```

## What it does

Iterates through Git repositories found under the current directory. For each repository, it retrieves the current name from GitHub and prompts for a new name. If `--description` is provided, it also retrieves the current description and prompts for a new one.

## Options

| Flag            | Required | Description                                       |
| --------------- | -------- | ------------------------------------------------- |
| `--description` | Optional | Also prompt to update the repository description. |

## Prerequisites

- `gh` (GitHub CLI) must be installed
- Git Wrangler GitHub auth must be configured with `git-wrangler init` or `git-wrangler config set github.auth`
- Each local repository must have a GitHub remote set as `origin`

## Examples

```bash
# Rename repos only
git-wrangler rename-repo

# Rename repos and update descriptions
git-wrangler rename-repo --description
```

## Notes

- Leave the prompt blank to skip renaming/updating a specific repository
- Repositories without a GitHub remote are skipped with a warning
