---
title: "doctor"
description: "Checks dependencies, install guidance, and update status."
category: "Utility"
order: 3
usage: "git-wrangler doctor"
---

# doctor

Checks whether Git Wrangler's command dependencies are installed, prints install instructions for your detected package manager, and reports whether your installed Git Wrangler checkout is up to date.

## Usage

```bash
git-wrangler doctor
```

## What it does

`doctor` checks for `git`, `gh`, `git-filter-repo`, and Python 3. If anything is missing, it detects the available package manager and prints commands for installing the dependencies.

It also compares the local Git Wrangler commit with the remote branch. If a newer version is available, it tells you to run `git-wrangler update`.

## Examples

```bash
git-wrangler doctor
```

## Notes

- `doctor` only prints install instructions; it never installs dependencies.
- If the remote repository cannot be reached, the update check reports a warning and the dependency check still completes.
