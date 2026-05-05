---
title: "info"
description: "Displays detailed information about tracked repositories."
category: "Utility"
order: 2
usage: "wrangler info [--repo <repository_name>]"
---

# info

Displays detailed information about tracked repositories.

## Usage

```bash
wrangler info [--repo <repository_name>]
```

## What it does

Iterates through Git repositories found in the current directory and its immediate subdirectories, and provides detailed information about each repository including name, status, license, branches, remotes, commits, top authors, and largest files in history.

## Options

| Flag | Required | Description |
|---|---|---|
| `--repo <name>` | Optional | Analyze a single repository instead of all in the current directory. |

## Example output

```
Repository:         my-api
Status:             Clean
License:            'MIT License'
Current branch:     main
Ahead/behind:       0 ahead, 0 behind remote
Branches (2):       main
                    develop
Remotes:            https://github.com/username/my-api.git
Initial commit:     2024-01-15 10:23:44 +0000 - feat: initial project setup
Total commits:      142
Commits last month: 18
Last commit:        2024-05-01 14:30:00 +0000 - chore: update dependencies
Top authors:        142 - Jane Doe <jane@example.com>
Largest files:      2.45 MB - assets/bundle.js
                    450.00 KB - assets/styles.css
```

## Examples

```bash
# Analyze all repositories in the current directory
wrangler info

# Analyze a single repository
wrangler info --repo my-project
```
