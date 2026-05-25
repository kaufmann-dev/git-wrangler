---
title: "Introduction"
description: "Learn what Git Wrangler is and why it was built."
category: "General"
order: 1
---

# Introduction

**Git Wrangler** is a command-line orchestrator that broadcasts Git operations — from simple pulls to complex history rewrites — across an entire directory of repositories simultaneously.

If you manage more than a handful of repositories, you know the pain: running `git pull` in twelve directories, committing the same change across five projects, or trying to scrub a secret that leaked into six repos. Git Wrangler solves that.

## What it does

Instead of writing brittle shell loops or reaching for heavyweight tools, Git Wrangler gives you a clean, Git-like interface:

```bash
git-wrangler <command> [options]
```

Every subcommand is a purpose-built script that targets all `.git` repositories it can find in the current directory — automatically, without configuration files or setup steps.

## Why it exists

Git Wrangler was built on three principles:

1. **Zero friction** — One install command. No config files. No daemons. Works anywhere Git does.
2. **Safety first** — Every repository operation runs in an isolated subshell. Variables and directory changes never leak between iterations.
3. **Extensibility** — Adding a new command is just adding a new file to `libexec/`. The help system discovers it automatically.

## Command categories

| Category              | What it covers                                                |
| --------------------- | ------------------------------------------------------------- |
| **Remote Operations** | Cloning, pulling, pushing, GitHub renaming                    |
| **Local Operations**  | Committing, reviewing, license management, `.gitignore` fixes |
| **History Rewriting** | Author/date/message rewrites, secret purging                  |
| **Utility**           | Status dashboard, environment diagnostics, update, uninstall  |

## Next steps

- [Install Git Wrangler](/docs/installation) — the one-liner
- [Understand the architecture](/docs/architecture) — how it all fits together
- [Browse command reference](/docs/status) — every command documented
