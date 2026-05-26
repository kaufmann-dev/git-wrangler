---
title: "Introduction"
description: "Learn what Git Wrangler is and why it was built."
category: "General"
order: 1
---

# Introduction

Git Wrangler is a compiled Go command-line tool for running Git workflows across many repositories from one directory.

It is built for maintainers who regularly need to pull, inspect, commit, push, rename, or rewrite history across a group of related repositories.

## What it does

Git Wrangler provides a Git-like command surface:

```bash
git-wrangler <command> [flags]
```

Commands discover regular `.git` directories and linked worktree `.git` files from the filesystem, process repositories in deterministic order, and print a compact status-oriented summary.

Bulk commands keep going after a per-repository failure, then exit nonzero if any repository operation failed. Repositories skipped because there was nothing to do still count as successful no-ops.

## Why it exists

Git Wrangler keeps repetitive multi-repository work in one focused CLI:

1. Install through Homebrew or GitHub Release binaries.
2. Use `gh` for GitHub operations so authentication stays familiar.
3. Use `git-filter-repo` for history rewrite commands.
4. Use Cobra-native help, version output, and shell completions.

## Command categories

Remote operations cover cloning, pulling, pushing, and GitHub repository renames.

Local operations cover status-adjacent repository maintenance such as commits, reviews, license files, branch renames, resets, and ignore cleanup.

History rewriting covers author, date, message, AI-assisted message, and secret-removal rewrites.

Utility commands cover repository information, version metadata, and completions.

## Next steps

- [Install Git Wrangler](/docs/installation)
- [Understand the architecture](/docs/architecture)
- [Browse command reference](/docs/status)
