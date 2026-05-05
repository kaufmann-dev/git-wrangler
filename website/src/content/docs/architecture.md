---
title: "Architecture"
description: "How Git Wrangler is built — the dispatcher, scripts, and isolation model."
category: "General"
order: 3
---

# Architecture

Git Wrangler is a modular, decentralized bash architecture built for extensibility, safety, and zero-maintenance scaling.

## Dispatcher model

The root `wrangler` script is a thin router. It receives `wrangler <subcommand>` and delegates to `libexec/wrangler-<subcommand>` via `exec bash`:

```
wrangler clone --user myname
      │
      ▼
libexec/wrangler-clone --user myname
```

There is no registry, no central config, no lookup table. The dispatcher just translates the subcommand name to a file path.

## Subcommand structure

Every script in `libexec/` follows this structure:

```bash
#!/bin/bash

# ====
# Usage: wrangler <subcommand> [--flag <value>]
# Description: One-line description used in the help menu.
# Category: Remote Operations | Local Operations | History Rewriting | Utility
#
# Longer description paragraph...
#
# Options:
#   --flag <value>  (required/optional) Description.
#
# Example:
#     wrangler <subcommand> --flag value
# ====

# 1. Default variables + argument parsing
# 2. Prerequisite checks (git, gh, git-filter-repo)
# 3. Repository discovery via find
# 4. Execution loop with subshell isolation
```

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

The `wrangler help` command scans every file in `libexec/` and reads its structured metadata header. No registration step is needed — add a file, get a help entry automatically.

## Error handling

All errors are written to **stderr** (`>&2`) so that piped commands still receive clean stdout. For example, `wrangler status | grep "dirty"` works correctly even if one repo fails.

## Adding a new command

1. Create `libexec/wrangler-mycommand` with the standard header block
2. Make it executable: `chmod +x libexec/wrangler-mycommand`
3. That's it — `wrangler help` discovers it immediately
