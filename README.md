# Git Wrangler

Git Wrangler is a command-line orchestrator that broadcasts Git operations from simple pulls to complex history rewrites across an entire directory of repositories simultaneously.

## Table of Contents
- [Features](#features)
- [Prerequisites](#prerequisites)
- [Installation](#installation)
- [Quick Start](#quick-start)
- [Command Reference](#command-reference)
- [Architecture](#architecture)

## Features
- **Multi-Repo Management:** Execute operations across multiple `.git` repositories in a single command.
- **History Rewriting:** Safely rewrite commit history, authors, dates, and remove secrets across repositories.
- **Unified CLI:** A clean, Git-like interface (`wrangler <command>`) with built-in help and isolated execution environments.
- **Cross-Platform:** Works natively on macOS, Linux, and Windows (via Git Bash / WSL).

## Prerequisites

Before installing or using Git Wrangler, ensure you have the following dependencies:
- **[`git`](https://git-scm.com/)**: Required for all core operations.
- **[`gh`](https://cli.github.com/)**: Required for `clone` and `rename-repo`.
- **[`git-filter-repo`](https://github.com/newren/git-filter-repo)**: Required for history rewriting (`rewrite-authors`, `rewrite-commits`, `rewrite-dates`, `remove-secrets`).

## Installation

Run the following command to securely download and install Git Wrangler:

```bash
curl -fsSL https://raw.githubusercontent.com/kaufmann-dev/git-wrangler/main/install | bash
```

This will clone the repository to `~/.wrangler` and symlink the executable to `~/.local/bin`. 

- **Update:** Run `wrangler update` to fetch the latest version.
- **Uninstall:** Run `wrangler uninstall` to safely remove the tool from your system.

## Quick Start

Here are a few common workflows to get you started:

**Clone multiple repositories for a specific user:**
```bash
wrangler clone --user myusername --visibility public --into ./repos
```

**Check the status of all tracked repositories:**
```bash
wrangler status
```

**Pull the latest changes across all repositories (with rebase):**
```bash
wrangler pull --rebase
```

**Stage and commit across all repositories:**
```bash
wrangler commit --message "chore: update dependencies"
```

*For more detailed help, run `wrangler help` or `wrangler help <command>`.*

## Command Reference

### Remote Operations
| Command | Description |
|---|---|
| `wrangler clone` | Clones multiple GitHub repositories for a given user |
| `wrangler rename-repo` | Bulk renames GitHub repositories and optionally their descriptions |
| `wrangler pull` | Pulls the latest changes for all tracked repositories |
| `wrangler push` | Pushes local commits to remote for all tracked repositories |

### Local Operations
| Command | Description |
|---|---|
| `wrangler commit` | Stages all changes and creates a commit across multiple repositories |
| `wrangler review` | Reviews committed changes before pushing across repositories |
| `wrangler license` | Adds or replaces a LICENSE file across repositories |
| `wrangler untrack` | Removes tracked files that match `.gitignore` exclusion rules |
| `wrangler fix-gitignore` | Audits and fixes `.gitignore` files by adding missing entries |
| `wrangler rename-branch` | Renames a specified branch to a new name across repositories |
| `wrangler reset` | Resets the current branch to exactly match its remote counterpart |

### History Rewriting
| Command | Description |
|---|---|
| `wrangler rewrite-authors` | Rewrites author and committer names and emails across repositories |
| `wrangler rewrite-commits` | Rewrites commit messages to adhere to the Conventional Commits standard |
| `wrangler rewrite-dates` | Redistributes commit timestamps to mimic natural human activity |
| `wrangler remove-secrets` | Permanently purges sensitive files from the entire Git history |

### Utility
| Command | Description |
|---|---|
| `wrangler status` | Shows dirty/clean and ahead/behind status of tracked repositories |
| `wrangler info` | Displays detailed information about tracked repositories |
| `wrangler update` | Updates Git Wrangler to the latest version |
| `wrangler uninstall` | Uninstalls Git Wrangler from the system |
| `wrangler help` | Displays help information for wrangler and its subcommands |

## Architecture

Git Wrangler is built on a modular, decentralized bash architecture designed for extensibility and safety:

- **Thin Dispatcher:** The root `wrangler` script acts purely as a router, delegating `wrangler <command>` invocations to standalone executable scripts in the `libexec/` directory.
- **Dynamic Help System:** There is no central registry for commands. The help menu is generated dynamically by parsing structured metadata headers embedded at the top of each script.
- **State Isolation:** When iterating over multiple repositories, operations are heavily sandboxed within subshells to guarantee that directory changes and variables never leak between iterations.
