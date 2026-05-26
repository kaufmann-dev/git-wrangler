---
title: "doctor"
description: "Checks runtime dependencies and GitHub authentication guidance."
category: "Utility"
order: 3
usage: "git-wrangler doctor [--summary]"
---

# doctor

Checks whether the normal runtime dependencies are available.

## Usage

```bash
git-wrangler doctor [--summary]
```

## What it checks

`doctor` checks for:

- `git`
- `gh`
- `git-filter-repo` or `git filter-repo`

It also reminds users to run `gh auth login` before private or all-repository GitHub operations.

## Options

- `--summary` prints only dependency status.

## Examples

```bash
git-wrangler doctor
git-wrangler doctor --summary
```
