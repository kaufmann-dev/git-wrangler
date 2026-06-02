# Git Wrangler

[![CI](https://github.com/kaufmann-dev/git-wrangler/actions/workflows/ci.yml/badge.svg)](https://github.com/kaufmann-dev/git-wrangler/actions/workflows/ci.yml)
[![Release](https://github.com/kaufmann-dev/git-wrangler/actions/workflows/release.yml/badge.svg)](https://github.com/kaufmann-dev/git-wrangler/actions/workflows/release.yml)
[![License: MIT](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)

**Orchestrate Git operations across many repositories.**

Run status checks, pulls, pushes, commits, branch renames, GitHub repo operations,
AI-powered commit generation, and guarded history rewrites — all from one directory,
without stepping through every project by hand.

📖 [Documentation](https://git-wrangler.kaufmann.dev) · 📦 [Releases](https://github.com/kaufmann-dev/git-wrangler/releases)

---

## Navigation

- [Why Git Wrangler?](#why-git-wrangler)
- [Install](#install)
- [Quick Start](#quick-start)
- [Commands](#commands)
- [AI-Powered Workflows](#ai-powered-workflows)
- [Safety & Guardrails](#safety--guardrails)
- [Shell Completions](#shell-completions)
- [License](#license)

## Why Git Wrangler?

If you maintain a workspace full of related repositories, routine Git operations
become tedious fast. Git Wrangler discovers every repository below your current
directory and runs operations across all of them in one pass:

- **One command, many repos** — `status`, `pull`, `push`, `commit`, and more work
  across every discovered repository.
- **AI commit messages** — generate Conventional Commit messages from diffs using
  any OpenAI-compatible endpoint.
- **Safe history rewrites** — rewrite authors, dates, commit messages, or purge
  secrets with confirmation prompts and automatic origin restoration.
- **GitHub integration** — clone all repos for a user or org, rename repos, and
  manage visibility through `gh`.
- **Stable, ordered output** — repositories are always processed and printed in
  the same deterministic order, even when running in parallel.
- **Single binary** — compiled Go with zero runtime dependencies beyond `git`.

## Installation

### Package Managers

```bash
# macOS & Linux
brew install kaufmann-dev/tap/git-wrangler

# Windows
scoop bucket add kaufmann-dev https://github.com/kaufmann-dev/scoop-bucket.git
scoop install kaufmann-dev/git-wrangler
```

### Standalone Binary

Download a release archive from
[GitHub Releases](https://github.com/kaufmann-dev/git-wrangler/releases), extract
the `git-wrangler` binary, and place it on your `PATH`. You'll need to install
`git`, `gh`, and `git-filter-repo` yourself for the commands that require them.

### Updating

```bash
# Homebrew
brew update
brew upgrade git-wrangler

# Scoop
scoop update
scoop update git-wrangler
```

## Quick Start

```bash
# 1. Verify your setup
git-wrangler doctor

# 2. Set up GitHub auth and AI credentials (if needed)
git-wrangler init

# 3. Check state across all repos in the current directory
git-wrangler status

# 4. Pull latest changes everywhere
git-wrangler pull --rebase

# 5. Review what you haven't pushed yet
git-wrangler review

# 6. Generate AI commit messages for every dirty repo
git-wrangler commit-ai
```

Run `git-wrangler help` for the full command list, or
`git-wrangler help <command>` for command-specific flags.

## Commands

### Remote Operations

| Command       | What it does                                              |
| ------------- | --------------------------------------------------------- |
| `clone`       | Clone multiple GitHub repositories for a user or org.     |
| `pull`        | Pull latest changes for every discovered repository.      |
| `push`        | Push local commits to origin. `--force` uses lease-based. |
| `rename-repo` | Rename GitHub repositories through `gh`.                  |

### Local Operations

| Command         | What it does                                              |
| --------------- | --------------------------------------------------------- |
| `commit`        | Stage all changes and create a commit in each dirty repo. |
| `fix-gitignore` | Add missing common generated-file patterns to `.gitignore`. |
| `license`       | Add or replace MIT license files.                         |
| `rename-branch` | Rename a branch across repositories.                      |
| `reset`         | Reset current branches to their origin counterparts.      |
| `review`        | Review unpushed changes across repositories.              |
| `untrack`       | Stop tracking files already covered by `.gitignore`.      |

### AI Commands

| Command              | What it does                                                        |
| -------------------- | ------------------------------------------------------------------- |
| `commit-ai`          | Generate and create one AI Conventional Commit per changed repo.    |
| `rewrite-commits-ai` | Generate AI Conventional Commit messages, then rewrite history.     |

### History Rewriting

| Command           | What it does                                   |
| ----------------- | ---------------------------------------------- |
| `remove-secrets`  | Purge sensitive files from Git history.         |
| `rewrite-authors` | Rewrite author and committer identity.          |
| `rewrite-commits` | Rewrite commit messages to Conventional Commits. |
| `rewrite-dates`   | Redistribute commit timestamps.                |

### Utility

| Command      | What it does                                        |
| ------------ | --------------------------------------------------- |
| `config`     | Show and edit Git Wrangler configuration.            |
| `doctor`     | Check runtime dependencies and local configuration. |
| `info`       | Show detailed repository information.               |
| `init`       | Set up GitHub and AI credentials.                   |
| `status`     | Show clean, dirty, ahead, behind, and remote state. |
| `version`    | Print version metadata.                             |
| `completion` | Generate shell completion scripts.                  |

## AI-Powered Workflows

`commit-ai` and `rewrite-commits-ai` use any OpenAI-compatible chat completions
API to generate Conventional Commit messages from your diffs.

```bash
# Set up credentials
git-wrangler init

# Or configure directly
git-wrangler config set ai-base-url https://api.openai.com/v1
git-wrangler config set ai-model gpt-4o
git-wrangler config set ai-api-key
```

**Privacy by default** — diff content is redacted to remove sensitive file
contents and common secret patterns before being sent to the API. Old commit
messages are never sent as context.

## Safety & Guardrails

Git Wrangler is built for bulk operations, so risky actions are always explicit:

- **Confirmation required** — history rewrite commands prompt before mutating.
  Pass `--yes` for noninteractive use.
- **Safe force push** — `push --force` uses `--force-with-lease`. Raw force push
  is a separate `--force-unsafe` flag.
- **Fail-safe bulk runs** — bulk commands continue after per-repo failures, then
  exit nonzero if anything failed. No-op skips stay successful.
- **Origin preservation** — history rewrite commands restore the `origin` remote
  after `git-filter-repo` removes it.
- **Warnings on stderr** — destructive operations always warn before proceeding.

### Runtime dependencies

| Tool               | Required for                                    |
| ------------------ | ----------------------------------------------- |
| `git`              | All repository operations (required).           |
| `gh`               | GitHub operations: `clone`, `rename-repo`.      |
| `git-filter-repo`  | History rewrites: `remove-secrets`, `rewrite-*`. |

Run `git-wrangler doctor` to check what's available on your system.

## Shell Completions

Homebrew and Scoop install completions automatically. For manual installs:

```bash
# Bash
git-wrangler completion bash > /etc/bash_completion.d/git-wrangler

# Zsh
git-wrangler completion zsh > "${fpath[1]}/_git-wrangler"

# Fish
git-wrangler completion fish > ~/.config/fish/completions/git-wrangler.fish

# PowerShell
git-wrangler completion powershell > git-wrangler.ps1
```
