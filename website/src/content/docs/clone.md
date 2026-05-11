---
title: "clone"
description: "Clones multiple GitHub repositories for a given user."
category: "Remote Operations"
order: 1
usage: "git-wrangler clone --user <username> [--visibility <all|public|private>] [--limit <number>] [--into <directory>]"
---

# clone

Clones multiple GitHub repositories for a given user.

## Usage

```bash
git-wrangler clone --user <username> [--visibility <all|public|private>] [--limit <number>] [--into <directory>]
```

## What it does

Clones GitHub repositories based on specified criteria (visibility, user, limit) and organizes them into a designated directory. Checks for existing repositories and skips them automatically. Requires `gh` (GitHub CLI) to be installed and authenticated.

## Options

| Flag | Required | Description |
|---|---|---|
| `--user <username>` | **Required** | GitHub username whose repositories to clone. |
| `--visibility <all\|public\|private>` | Optional | Visibility filter. Default: `all`. |
| `--limit <number>` | Optional | Maximum number of repositories to clone. Default: `100`. |
| `--into <directory>` | Optional | Target directory for cloned repositories. Default: the username. |

## Prerequisites

- `gh` (GitHub CLI) must be installed and authenticated
- For private repos, you must be authenticated as the target user

## Examples

```bash
# Clone all public repos into ./repos
git-wrangler clone --user myusername --visibility public --into ./repos

# Clone up to 10 repos into a directory named after the user
git-wrangler clone --user myusername --limit 10

# Clone all repos (public + private — requires gh auth)
git-wrangler clone --user myusername --visibility all
```

## Notes

- Existing directories are skipped automatically (the command is idempotent)
- The `--into` directory is created if it does not exist
