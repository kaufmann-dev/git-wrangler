---
title: "rewrite-authors"
description: "Rewrites author and committer names and emails across repositories."
category: "History Rewriting"
order: 1
usage: "git-wrangler rewrite-authors --name <new_name> --email <new_email> --confirm [--force] [--repo <repository_name>]"
---

# rewrite-authors

Rewrites author and committer names and emails across repositories.

## Usage

```bash
git-wrangler rewrite-authors --name <new_name> --email <new_email> --confirm [--force] [--repo <repository_name>]
```

`--confirm` is required before any destructive history rewrite is performed.

## What it does

Iterates through Git repositories found under the current directory, and rewrites **all** author and committer information across the entire history using `git-filter-repo`. The remote origin URL is automatically restored after the rewrite.

## Options

| Flag              | Required     | Description                                                              |
| ----------------- | ------------ | ------------------------------------------------------------------------ |
| `--name <name>`   | **Required** | The new name to set as author and committer.                             |
| `--email <email>` | **Required** | The new email to set as author and committer.                            |
| `--confirm`       | **Required** | Confirm noninteractive history rewriting.                                |
| `--force`         | Optional     | Allow rewriting even if the repository does not look like a fresh clone. |
| `--repo <name>`   | Optional     | Target a single repository instead of all in the current directory.      |

## Prerequisites

- `git-filter-repo` must be installed

## Example

```bash
git-wrangler rewrite-authors --name "Jane Doe" --email "jane@example.com" --confirm --force
```

## Notes

> **Warning:** This rewrites Git history. Use `git-wrangler push --force` for a lease-safe remote update, or `git-wrangler push --force-unsafe` only when a raw force push is intentional.

- The remote `origin` URL is preserved and restored automatically after the rewrite
- `--force` is required when running on a non-fresh clone (which is the typical use case)
