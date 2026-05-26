---
title: "Architecture"
description: "How Git Wrangler is built — the dispatcher, scripts, and isolation model."
category: "General"
order: 3
---

# Architecture

Git Wrangler is a modular, decentralized bash architecture built for extensibility, safety, and zero-maintenance scaling.

## Dispatcher model

The root `git-wrangler` script is a thin router. It receives `git-wrangler <subcommand>` and delegates to `libexec/git-wrangler-<subcommand>` via `exec bash`:

```
git-wrangler clone --user myname
      │
      ▼
libexec/git-wrangler-clone --user myname
```

There is no registry, no central config, no lookup table. The dispatcher just translates the subcommand name to a file path.

## Subcommand structure

Every script in `libexec/` follows this structure:

```bash
#!/bin/bash

# ====
# Usage: git-wrangler <subcommand> [--flag <value>]
# Description: One-line description used in the help menu.
# Category: Remote Operations | Local Operations | History Rewriting | Utility
#
# Longer description paragraph...
#
# Options:
#   --flag <value>  (required/optional) Description.
#
# Example:
#     git-wrangler <subcommand> --flag value
# ====

# 1. Default variables + argument parsing
# 2. Shared UI helper source
# 3. Prerequisite checks (git, gh, git-filter-repo, python3 as needed)
# 4. Repository discovery via find
# 5. Execution loop with subshell isolation
```

## Terminal UI helper

Every subcommand sources `libexec/git-wrangler-ui` after its metadata header. The helper centralizes colors, status labels, repository headers, and confirmation prompts.

The helper follows common CLI conventions:

- Color is enabled only for TTY output.
- `NO_COLOR=1`, `CLICOLOR=0`, and `TERM=dumb` disable styling.
- `CLICOLOR_FORCE=1` forces color unless one of the disabling controls is set.
- Unicode symbols are used only for capable interactive terminals; plain output uses ASCII labels.

## Repository discovery

All multi-repo commands locate `.git` directories using `find`:

```bash
git_repositories=$(find . -maxdepth 2 -type d -name '.git')
```

This searches the current directory and one level of subdirectories — matching the natural layout of a "mono-workspace" where multiple repos sit side by side.

## Subshell isolation

This is the key safety feature. Every repository operation runs inside a subshell `( ... )`:

```bash
while IFS= read -r git_dir; do
    (
        repo_dir=$(dirname "$git_dir")
        cd "$repo_dir" || exit
        # ... operations ...
    )
done <<< "$git_repositories"
```

**Why this matters:**
- A `cd` inside a subshell never affects the parent shell
- Variables set inside never leak to subsequent iterations
- A `exit` inside a subshell only exits that subshell — the loop continues

## Dynamic help system

The `git-wrangler help` command scans command files in `libexec/` and reads their structured metadata headers. Helper files without command metadata are ignored. No registration step is needed — add a command file with a valid header, get a help entry automatically.

## Error handling

All errors are written to **stderr** (`>&2`) so that piped commands still receive clean stdout. For example, `git-wrangler status | grep "dirty"` works correctly even if one repo fails.

## Adding a new command

1. Create `libexec/git-wrangler-mycommand` with the standard header block
2. Source `libexec/git-wrangler-ui`
3. Make it executable: `chmod +x libexec/git-wrangler-mycommand`
4. That's it — `git-wrangler help` discovers it immediately
