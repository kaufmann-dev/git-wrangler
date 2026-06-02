---
title: "doctor"
description: "Checks Git Wrangler runtime dependencies."
category: "Utility"
order: 3
usage: "git-wrangler doctor"
---

# doctor

Checks the local tools Git Wrangler needs for its workflows.

## Usage

```bash
git-wrangler doctor
```

This command takes no arguments.

## What it does

Prints the Git Wrangler version, the current platform, the executable path, and local dependency checks for `git`, `gh`, and `git-filter-repo`.

Missing `git` is reported as an error because most Git Wrangler commands need it, and `doctor` exits nonzero. Missing `gh` or `git-filter-repo` is reported as a warning because those tools are only needed for specific workflows.

`doctor` does not scan repositories, check GitHub authentication, make network requests, or check API keys.

## Example output

```text
Git Wrangler Doctor

Version:    git-wrangler dev
Platform:   linux/amd64
Executable: /usr/local/bin/git-wrangler

Dependencies:
  OK    git              /usr/bin/git (git version 2.50.0)
  WARN  gh               not found; needed for clone and rename-repo
  WARN  git-filter-repo  not found; needed for history rewrite commands

Source installs do not include runtime dependencies. Install missing tools yourself or use an official bundled install.
```
