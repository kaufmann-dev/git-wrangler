---
title: "untrack"
description: "Removes tracked files that match .gitignore exclusion rules."
category: "Local Operations"
order: 4
usage: "git-wrangler untrack [--confirm]"
---

# untrack

Removes tracked files that match `.gitignore` exclusion rules.

## Usage

```bash
git-wrangler untrack [--confirm]
```

The command previews the number of tracked ignored files before modifying each repository. Use `--confirm` to skip the prompt for noninteractive runs.

## What it does

Removes files from the Git index that are actively tracked but match exclusion rules in `.gitignore`. It untracks the files safely while leaving them on the local disk, and commits the removals automatically.

This is typically needed when you:
1. Added an entry to `.gitignore` after a file was already committed
2. Want to enforce ignoring without deleting the local file

## Example

```bash
git-wrangler untrack --confirm
```

## Notes

- Files are **not** deleted from disk — they are only removed from the Git index
- The untracking is committed with the message `"Stop tracking files defined in .gitignore"` after confirmation
- Repositories without a `.gitignore` are skipped
- Repositories where no tracked files match `.gitignore` are skipped cleanly
