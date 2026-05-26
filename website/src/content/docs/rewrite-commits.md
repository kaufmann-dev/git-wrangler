---
title: "rewrite-commits"
description: "Rewrites commit messages to adhere to the Conventional Commits standard."
category: "History Rewriting"
order: 2
usage: "git-wrangler rewrite-commits"
---

# rewrite-commits

Rewrites commit messages to adhere to the Conventional Commits standard.

## Usage

```bash
git-wrangler rewrite-commits
```

This command takes no arguments.

## What it does

Rewrites the commit messages of Git repositories to adhere to the [Conventional Commits](https://www.conventionalcommits.org/) standard. It categorizes commits based on file paths and statuses to automatically determine the type (e.g., `feat`, `fix`, `docs`, `chore`) and scope. Commits that already conform to the standard are left unchanged.

## Type detection logic

| Condition                                               | Assigned type |
| ------------------------------------------------------- | ------------- |
| Only doc files changed (`.md`, `.txt`, `docs/`)         | `docs`        |
| Only test files changed (`test/`, `spec/`, `*.test.*`)  | `test`        |
| Only config files changed (`.yml`, `.json`, `Makefile`) | `chore`       |
| Source files added (no deletions)                       | `feat`        |
| Source files modified/mixed                             | `fix`         |
| Everything else                                         | `chore`       |

## Prerequisites

- `git-filter-repo` must be installed

## Example

```bash
git-wrangler rewrite-commits
```

## Notes

> **Warning:** This rewrites Git history. You will need to force-push to update remotes.

- Commits that already match the `type(scope): message` pattern are untouched
- Empty commits (no file changes) are left unchanged
- The remote `origin` URL is restored automatically
