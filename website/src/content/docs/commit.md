---
title: "commit"
description: "Stages all changes and creates a commit across multiple repositories."
category: "Local Operations"
order: 1
usage: "git-wrangler commit --message <commit_message>"
---

# commit

Stages all changes and creates a commit across multiple repositories.

## Usage

```bash
git-wrangler commit --message <commit_message>
```

## What it does

Iterates through Git repositories found in the current directory and its immediate subdirectories, stages all changes (`git add -A`), and creates a commit with the provided message. Repositories with no staged changes are skipped automatically.

## Options

| Flag | Required | Description |
|---|---|---|
| `--message <text>` | **Required** | The commit message to use for all repositories. |

## Example

```bash
git-wrangler commit --message "chore: update dependencies"
```

## Notes

- Uses `git add -A` to stage all changes, including new files, modifications, and deletions
- Repositories where there is nothing to commit are skipped with a yellow message
- For repositories with nothing staged after `git add -A`, the command exits cleanly without error
