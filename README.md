# Git Wrangler

Git Wrangler is a command-line orchestrator that broadcasts Git operations from simple pulls to complex history rewrites across an entire directory of repositories simultaneously.

## Table of Contents
- [Features](#features)
- [Prerequisites](#prerequisites)
- [Installation](#installation)
- [Quick Start](#quick-start)
- [Terminal Output](#terminal-output)
- [Command Reference](#command-reference)
- [Architecture](#architecture)
- [Contributor Commands](#contributor-commands)

## Features
- **Multi-Repo Management:** Execute operations across multiple `.git` repositories in a single command.
- **History Rewriting:** Safely rewrite commit history, authors, dates, and remove secrets across repositories.
- **Unified CLI:** A clean, Git-like interface (`git-wrangler <command>`) with built-in help and isolated execution environments.
- **Cross-Platform:** Works natively on macOS, Linux, and Windows (via Git Bash / WSL).

## Prerequisites

Before installing or using Git Wrangler, ensure you have the following dependencies:
- **[`git`](https://git-scm.com/)**: Required for all core operations.
- **[`gh`](https://cli.github.com/)**: Required for `clone` and `rename-repo`.
- **[`git-filter-repo`](https://github.com/newren/git-filter-repo)**: Required for history rewriting (`rewrite-authors`, `rewrite-commits`, `rewrite-commits-ai`, `rewrite-dates`, `remove-secrets`) as either the `git-filter-repo` executable or the `git filter-repo` Git subcommand.
- **Go**: Required to build and run the Go CLI from a source checkout.
- **Python 3**: Required for AI-assisted commit rewrites and date redistribution.
- **OpenAI-compatible API access**: Required for `rewrite-commits-ai` (`--base-url`, `--model`, and an API key).

Run `git-wrangler doctor` after installation to check your environment, see package-manager-specific install instructions, and verify whether Git Wrangler is up to date.

## Installation

Run the following command to securely download and install Git Wrangler:

```bash
curl -fsSL https://raw.githubusercontent.com/kaufmann-dev/git-wrangler/main/install | bash
```

This will clone the repository to `~/.git-wrangler`, symlink the executable to `~/.local/bin`, and show a short dependency summary.

- **Update:** Run `git-wrangler update` to fetch the latest version.
- **Uninstall:** Run `git-wrangler uninstall` to safely remove the tool from your system.

## Quick Start

Here are a few common workflows to get you started:

**Clone multiple repositories for a specific user:**
```bash
git-wrangler clone --user myusername --visibility public --into ./repos
```

**Check the status of all tracked repositories:**
```bash
git-wrangler status
```

**Pull the latest changes across all repositories (with rebase):**
```bash
git-wrangler pull --rebase
```

**Stage and commit across all repositories:**
```bash
git-wrangler commit --message "chore: update dependencies"
```

**Run local checks before changing the Bash CLI:**
```bash
scripts/check
scripts/test
```

*For more detailed help, run `git-wrangler help` or `git-wrangler help <command>`.*

## Terminal Output

Git Wrangler uses one terminal vocabulary across the dispatcher, installer, and subcommands. Interactive terminals get colored status output and Unicode symbols; plain output uses ASCII labels and no ANSI styling.

The following environment variables control presentation:

- `NO_COLOR=1` disables color and styling.
- `CLICOLOR=0` disables color and styling.
- `CLICOLOR_FORCE=1` forces color unless `NO_COLOR`, `CLICOLOR=0`, or `TERM=dumb` is set.
- `TERM=dumb` disables color and Unicode symbols.

Color is also disabled automatically when output is not a TTY, so piped commands stay machine-readable.

## Command Reference

### Remote Operations
| Command                    | Description                                                        |
| -------------------------- | ------------------------------------------------------------------ |
| `git-wrangler clone`       | Clones multiple GitHub repositories for a given user               |
| `git-wrangler rename-repo` | Bulk renames GitHub repositories and optionally their descriptions |
| `git-wrangler pull`        | Pulls the latest changes for all tracked repositories              |
| `git-wrangler push`        | Pushes local commits to remote, with a lease-safe force mode       |

### Local Operations
| Command                      | Description                                                          |
| ---------------------------- | -------------------------------------------------------------------- |
| `git-wrangler commit`        | Stages all changes and creates a commit across multiple repositories |
| `git-wrangler review`        | Reviews committed changes before pushing across repositories         |
| `git-wrangler license`       | Adds or replaces a LICENSE file across repositories                  |
| `git-wrangler untrack`       | Removes tracked files that match `.gitignore` exclusion rules        |
| `git-wrangler fix-gitignore` | Audits and fixes `.gitignore` files by adding missing entries        |
| `git-wrangler rename-branch` | Renames a specified branch to a new name across repositories         |
| `git-wrangler reset`         | Resets the current branch to exactly match its remote counterpart    |

### History Rewriting
| Command                           | Description                                                             |
| --------------------------------- | ----------------------------------------------------------------------- |
| `git-wrangler rewrite-authors`    | Rewrites author and committer names and emails across repositories      |
| `git-wrangler rewrite-commits`    | Rewrites commit messages to adhere to the Conventional Commits standard |
| `git-wrangler rewrite-commits-ai` | Rewrites commit messages with an OpenAI-compatible AI endpoint          |
| `git-wrangler rewrite-dates`      | Redistributes commit timestamps to mimic natural human activity         |
| `git-wrangler remove-secrets`     | Permanently purges sensitive files from history after confirmation      |

### Utility
| Command                  | Description                                                       |
| ------------------------ | ----------------------------------------------------------------- |
| `git-wrangler status`    | Shows dirty/clean and ahead/behind status of tracked repositories |
| `git-wrangler info`      | Displays detailed information about tracked repositories          |
| `git-wrangler doctor`    | Checks dependencies, install guidance, and update status          |
| `git-wrangler update`    | Updates Git Wrangler to the latest version                        |
| `git-wrangler uninstall` | Uninstalls Git Wrangler from the system                           |
| `git-wrangler help`      | Displays help information for git-wrangler and its subcommands    |

## Architecture

Git Wrangler is implemented as a Go CLI with the original Bash command files retained as reference documentation and help metadata:

- **Thin Launcher:** The root `git-wrangler` script resolves the install directory, builds a cached Go binary when needed, and executes it.
- **Go Command Registry:** Public subcommands are implemented in `cmd/git-wrangler`.
- **Dynamic Help Metadata:** The help menu still parses structured headers from `libexec/git-wrangler-*`, so the Bash files remain the help/reference oracle.
- **Shared Go Helpers:** Terminal UI, parser validation, repo discovery, display names, confirmations, command capture, and destructive safeguards are centralized in Go.
- **Sequential Repository Processing:** Mutating and destructive commands process repositories one at a time and use temporary repositories in tests.

## Contributor Commands

| Command         | Description                                                                            |
| --------------- | -------------------------------------------------------------------------------------- |
| `scripts/check` | Runs Bash syntax checks, Go tests, optional ShellCheck/shfmt, and website build checks |
| `scripts/test`  | Runs temp-directory integration tests for core Git Wrangler behavior                   |
| `scripts/bench` | Creates temporary repositories and times read-only status checks                       |

The Bash-to-Go compatibility contract is documented in `GO_REWRITE_CONTRACT.md`. Keep it passing with `scripts/test` when changing Go command behavior or Bash reference metadata.
