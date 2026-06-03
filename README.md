# Git Wrangler

[![CI](https://github.com/kaufmann-dev/git-wrangler/actions/workflows/ci.yml/badge.svg)](https://github.com/kaufmann-dev/git-wrangler/actions/workflows/ci.yml)
[![Release](https://github.com/kaufmann-dev/git-wrangler/actions/workflows/release.yml/badge.svg)](https://github.com/kaufmann-dev/git-wrangler/actions/workflows/release.yml)
[![License: MIT](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)
[![Documentation](https://img.shields.io/badge/docs-wrangler.kaufmann.dev-blue)](https://wrangler.kaufmann.dev/docs/introduction)
[![GitHub Releases](https://img.shields.io/badge/releases-GitHub-blue)](https://github.com/kaufmann-dev/git-wrangler/releases)

**Git operations, orchestrated at scale.**

Cross-platform Go CLI for coordinating dozens of Git repositories in parallel, eliminating the manual overhead of managing large developer workspaces.

---

## Navigation

- [Why Git Wrangler?](#why-git-wrangler)
- [Installation](#installation)
- [Quick Start](#quick-start)
- [Commands](#commands)
- [AI-Powered Workflows](#ai-powered-workflows)
- [Safety & Guardrails](#safety--guardrails)
- [Runtime Dependencies](#runtime-dependencies)
- [Shell Completions](#shell-completions)

## Why Git Wrangler?

Managing many related Git repositories by hand is repetitive, slow, and easy to
mess up. Git Wrangler turns that workspace into one coordinated unit.

It finds repositories below your current directory and runs Git workflows across
them in one pass, with parallel execution, stable output, and safe defaults.

### What it gives you

- **One command, many repositories** — run common Git workflows across every repo.
- **Parallel execution, stable output** — fast runs without chaotic terminal output.
- **AI-assisted commits** — generate Conventional Commit messages from diffs.
- **Safer history rewrites** — rewrite metadata or remove secrets with confirmations.
- **GitHub workflows** — clone, rename, and manage repositories through `gh`.
- **Single binary** — portable Go executable with no runtime dependencies beyond `git`.

## Installation & Maintenance

### Install

```bash
# macOS & Linux
brew install kaufmann-dev/tap/git-wrangler

# Windows
scoop bucket add kaufmann-dev https://github.com/kaufmann-dev/scoop-bucket.git
scoop install kaufmann-dev/git-wrangler
```

> [!TIP]
> You can also install manually by downloading a standalone binary from
> GitHub Releases, extracting it, and adding it to your `PATH`.
> You will need to manually install runtime dependencies and set up shell
> completions, as these are not automatically handled like with Scoop or Homebrew
> (see [Shell Completions](#shell-completions) and [Runtime Dependencies](#runtime-dependencies)).

### Update

```bash
# Homebrew
brew update
brew upgrade git-wrangler

# Scoop
scoop update
scoop update git-wrangler
```

### Uninstall

```bash
# Homebrew
brew uninstall git-wrangler
brew untap kaufmann-dev/tap

# Scoop
scoop uninstall git-wrangler
scoop bucket rm kaufmann-dev
```

## Quick Start

```bash
# 1. Set up GitHub auth and AI credentials
git-wrangler init

# 2. Verify your setup
git-wrangler doctor

# 3. Check state across all repos in the current directory
git-wrangler status

# 4. See the full command list to decide what to do next
git-wrangler help
```

## Commands

### Remote Operations

| Command       | What it does                                              |
| ------------- | --------------------------------------------------------- |
| `clone`       | Clone multiple GitHub repositories for a user or org.     |
| `pull`        | Pull latest changes for every discovered repository.      |
| `push`        | Push local commits to origin. `--force` uses lease-based. |
| `rename-repo` | Rename GitHub repositories through `gh`.                  |

### Local Operations

| Command         | What it does                                                |
| --------------- | ----------------------------------------------------------- |
| `commit`        | Stage all changes and create a commit in each dirty repo.   |
| `fix-gitignore` | Add missing common generated-file patterns to `.gitignore`. |
| `license`       | Add or replace MIT license files.                           |
| `rename-branch` | Rename a branch across repositories.                        |
| `reset`         | Reset current branches to their origin counterparts.        |
| `review`        | Review unpushed changes across repositories.                |
| `untrack`       | Stop tracking files already covered by `.gitignore`.        |

### AI Commands

| Command              | What it does                                                        |
| -------------------- | ------------------------------------------------------------------- |
| `commit-ai`          | Generate and create one AI Conventional Commit per changed repo.    |
| `rewrite-commits-ai` | Generate AI Conventional Commit messages, then rewrite history.     |

### History Rewriting

| Command           | What it does                                     |
| ----------------- | ------------------------------------------------ |
| `remove-secrets`  | Purge sensitive files from Git history.          |
| `rewrite-authors` | Rewrite author and committer identity.           |
| `rewrite-commits` | Rewrite commit messages to Conventional Commits. |
| `rewrite-dates`   | Redistribute commit timestamps.                  |

### Utility

| Command      | What it does                                        |
| ------------ | --------------------------------------------------- |
| `config`     | Show and edit Git Wrangler configuration.           |
| `doctor`     | Check runtime dependencies and local configuration. |
| `info`       | Show detailed repository information.               |
| `init`       | Set up GitHub and AI credentials.                   |
| `status`     | Show clean, dirty, ahead, behind, and remote state. |
| `version`    | Print version metadata.                             |
| `completion` | Generate shell completion scripts.                  |
| `help`       | Show help for Git Wrangler or a specific command.   |

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

## Safety & Guardrails

Git Wrangler is built for bulk Git operations, where small mistakes can affect
many repositories at once. Destructive actions are therefore explicit, guarded,
and designed to fail safely.

- **Privacy by default** — AI commands redact diff content before sending it to
  the API, including sensitive file contents and common secret patterns. Old
  commit messages are not sent as context.
- **Confirmation before mutation** — history rewrite commands ask before making
  destructive changes. Use `--yes` only for intentional noninteractive runs.
- **Safer force pushes** — `push --force` uses `--force-with-lease`. Raw force
  push requires the separate `--force-unsafe` flag.
- **Fail-safe bulk runs** — per-repository failures do not stop the whole run.
  Git Wrangler reports all failures and exits nonzero if anything failed.
- **Origin preservation** — history rewrite commands restore the `origin` remote
  after `git-filter-repo` removes it.
- **Warnings on stderr** — destructive operations warn clearly without polluting
  normal command output.

## Runtime Dependencies

| Tool               | Required for                                     |
| ------------------ | ------------------------------------------------ |
| `git`              | All repository operations (required).            |
| `gh`               | GitHub operations: `clone`, `rename-repo`.       |
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
