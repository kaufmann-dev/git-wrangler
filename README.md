# Git Wrangler

A unified CLI tool for managing multiple Git repositories at once. Wrangler provides a single entry point — `wrangler` — that dispatches to a collection of subcommands for cloning, pushing, pulling, committing, and rewriting history across all repositories in your working directory.

## Prerequisites

For all subcommands to work, make sure you have the following installed:
- `git`
- `gh` (GitHub CLI) — required for `wrangler clone`
- `git-filter-repo` — required for `wrangler rewrite-authors`, `wrangler rewrite-commits`, and `wrangler remove-secrets`

## Usage

```
wrangler <subcommand> [options]
```

Run `wrangler help` to see all available subcommands, or `wrangler help <subcommand>` for detailed documentation on a specific command.

## Subcommands

### Remote Operations

| Subcommand | Description |
|---|---|
| `wrangler clone` | Clones multiple GitHub repositories for a given user |
| `wrangler pull` | Pulls the latest changes for all tracked repositories |
| `wrangler push` | Pushes local commits to remote for all tracked repositories |

### Local Operations

| Subcommand | Description |
|---|---|
| `wrangler commit` | Stages all changes and creates a commit across multiple repositories |
| `wrangler license` | Adds or replaces a LICENSE file across repositories |
| `wrangler untrack` | Removes tracked files that match .gitignore exclusion rules |
| `wrangler fix-gitignore` | Audits and fixes .gitignore files by adding missing entries |
| `wrangler rename-branch` | Renames a specified branch to a new name across repositories |

### History Rewriting

| Subcommand | Description |
|---|---|
| `wrangler rewrite-authors` | Rewrites author and committer names and emails across repositories |
| `wrangler rewrite-commits` | Rewrites commit messages to adhere to the Conventional Commits standard |
| `wrangler remove-secrets` | Permanently purges sensitive files from the entire Git history |

### Utility

| Subcommand | Description |
|---|---|
| `wrangler info` | Displays detailed information about tracked repositories |
| `wrangler help` | Displays help information for wrangler and its subcommands |

## Examples

Clone all repositories for a GitHub user:
```bash
wrangler clone --user myusername --visibility public --into ./repos
```

Pull latest changes across all repositories:
```bash
wrangler pull --rebase
```

Stage and commit across all repositories:
```bash
wrangler commit --message "chore: update dependencies"
```

Add an MIT license to all repositories:
```bash
wrangler license --name "Your Name"
```

Rewrite author information:
```bash
wrangler rewrite-authors --name "New Name" --email "new@email.com" --force
```

View repository details:
```bash
wrangler info --repo my-project
```

Get help for a specific subcommand:
```bash
wrangler help clone
```

## Architecture

The `wrangler` script in the repository root is a thin dispatcher. It resolves the requested subcommand and hands off execution to the corresponding script in `libexec/`:

```
wrangler                  # Dispatcher (repository root)
libexec/
  wrangler-clone          # wrangler clone
  wrangler-pull           # wrangler pull
  wrangler-push           # wrangler push
  wrangler-commit         # wrangler commit
  wrangler-license        # wrangler license
  wrangler-info           # wrangler info
  wrangler-help           # wrangler help
  wrangler-untrack        # wrangler untrack
  wrangler-fix-gitignore  # wrangler fix-gitignore
  wrangler-rename-branch  # wrangler rename-branch
  wrangler-rewrite-authors  # wrangler rewrite-authors
  wrangler-rewrite-commits  # wrangler rewrite-commits
  wrangler-remove-secrets   # wrangler remove-secrets
```

Each subcommand script in `libexec/` includes a structured header block with `Usage`, `Description`, and `Category` fields that the help system parses dynamically. Per-subcommand documentation is accessed via `wrangler help <subcommand>`.
