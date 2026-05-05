---
title: "license"
description: "Adds or replaces a LICENSE file across repositories."
category: "Local Operations"
order: 3
usage: "wrangler license --name <copyright_holder> [--overwrite] [--repo <repository_name>]"
---

# license

Adds or replaces a LICENSE file across repositories.

## Usage

```bash
wrangler license --name <copyright_holder> [--overwrite] [--repo <repository_name>]
```

## What it does

Iterates through Git repositories found in the current directory and creates or overwrites a license file with a given copyright holder's name. Uses the MIT license by default. Existing LICENSE files are skipped unless `--overwrite` is provided.

## Options

| Flag | Required | Description |
|---|---|---|
| `--name <name>` | **Required** | The copyright holder's name to embed in the license. |
| `--overwrite` | Optional | Replace existing LICENSE files instead of skipping them. |
| `--repo <name>` | Optional | Target a single repository instead of all in the current directory. |

## Examples

```bash
# Add a LICENSE to all repos that don't have one
wrangler license --name "Jane Doe"

# Replace all existing LICENSE files
wrangler license --name "Jane Doe" --overwrite

# Target a single repo
wrangler license --name "Jane Doe" --repo my-project
```

## Notes

- The generated license is always MIT format
- The current year is embedded automatically
