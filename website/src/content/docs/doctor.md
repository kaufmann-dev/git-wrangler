---
title: "doctor"
description: "Checks Git Wrangler runtime dependencies and local setup."
category: "Utility"
order: 3
usage: "git-wrangler doctor"
---

# doctor

Checks the local tools and setup Git Wrangler needs for its workflows.

## Usage

```bash
git-wrangler doctor
```

This command takes no arguments.

## What it does

Prints the Git Wrangler version, the current platform, the executable path, local dependency checks for `git`, `gh`, and `git-filter-repo`, and local config/auth state.

Missing `git` is reported as an error because most Git Wrangler commands need it, and `doctor` exits nonzero. Missing `gh` or `git-filter-repo` is reported as a warning because those tools are only needed for specific workflows.

`doctor` does not scan repositories, make network requests, or validate tokens. It reports local config validity, keyring availability, credential source, and AI provider/model/base URL presence.

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

Config and Auth:
  OK    config           /home/user/.config/git-wrangler/config.json
  WARN  keyring          unavailable
  WARN  github.auth      missing
  OK    github.host      github.com
  WARN  ai.api-key       missing
  OK    ai.provider      openai
  OK    ai.base-url      https://api.openai.com/v1
  WARN  ai.model         missing
```
