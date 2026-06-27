# Git Wrangler

![Git Wrangler Logo](docs/logo.png)

[![CI](https://github.com/kaufmann-dev/git-wrangler/actions/workflows/ci.yml/badge.svg)](https://github.com/kaufmann-dev/git-wrangler/actions/workflows/ci.yml)
[![Release](https://github.com/kaufmann-dev/git-wrangler/actions/workflows/release.yml/badge.svg)](https://github.com/kaufmann-dev/git-wrangler/actions/workflows/release.yml)
[![License: MIT](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)
[![Documentation](https://img.shields.io/badge/docs-online-blue)](https://wrangler.kaufmann.dev/docs/introduction)
[![GitHub Releases](https://img.shields.io/badge/releases-GitHub-blue)](https://github.com/kaufmann-dev/git-wrangler/releases)

**Git operations, orchestrated at scale.**

Cross-platform Go CLI for coordinating dozens of Git repositories in parallel, eliminating the manual overhead of managing large developer workspaces.

---

## Navigation

- [Why Git Wrangler?](#why-git-wrangler)
- [Installation & Maintenance](#installation--maintenance)
- [Quick Start](#quick-start)
- [Commands](#commands)
- [AI-Powered Workflows](#ai-powered-workflows)
- [Safety & Guardrails](#safety--guardrails)
- [Runtime Dependencies](#runtime-dependencies)
- [Shell Completions](#shell-completions)
- [Development](#development)

## Why Git Wrangler?

Managing many related Git repositories by hand is repetitive, slow, and easy to
mess up. Git Wrangler turns that workspace into one coordinated unit.

It finds repositories below your current directory and runs Git workflows across
them in one pass, with parallel execution, stable output, and safe defaults.
Use `--repo PATH` on repository commands when you want to target exactly one
repository instead of discovering everything below the current directory.
Remote-aware commands such as `status`, `info`, `review`, and history rewrite
planning refresh `origin` by default before inspecting remote-tracking refs.
Use `--no-fetch` on those commands for offline or local-only runs.

### What it gives you

- **One command, many repositories** — run common Git workflows across every repo.
- **Parallel execution, stable output** — fast runs without chaotic terminal output.
- **AI-assisted commits** — generate Conventional Commit messages from diffs.
- **Safer history rewrites** — rewrite metadata or remove secrets with confirmations.
- **GitHub workflows** — clone, rename, and manage repositories through `gh`.
- **Single binary** — portable Go executable; GitHub and history workflows use standard tools like `gh` and `git-filter-repo`.

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
> [GitHub Releases](https://github.com/kaufmann-dev/git-wrangler/releases),
> extracting it, and adding it to your `PATH`.
> You will need to manually install runtime dependencies and set up shell
> completions, as these are not automatically handled like with Scoop or Homebrew
> (see [Runtime Dependencies](#runtime-dependencies) and [Shell Completions](#shell-completions)).

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

Most repository workflow commands support `--guided` to collect their
command-specific options interactively before execution:

```bash
git-wrangler push --guided
git-wrangler rewrite-dates --guided
```

Guided setup requires both stdin and stderr to be terminals, prints the selected
configuration to stderr, and cannot be combined with `--json`. Missing required
values are prompted for in a terminal and fail in noninteractive runs.
Pressing `Ctrl+C` or sending EOF with `Ctrl+D` cancels any active prompt
immediately, stops the command before further work, and exits nonzero.

On browserless machines, `init` prints the GitHub device code and verification
URL so authorization can be completed in a browser on another device. On
machines without an available keyring, configure credentials through environment
variables before running Git Wrangler:

```bash
export GIT_WRANGLER_GITHUB_TOKEN=...
export GIT_WRANGLER_AI_API_KEY=...

# OpenAI API keys may also use the provider-specific fallback:
export OPENAI_API_KEY=...
```

Git Wrangler does not resolve GitHub credentials from an inbound `GH_TOKEN`.
It sets `GH_TOKEN` only when passing its own resolved credential to child `gh`
processes.

## Commands

### Remote Operations

| Command                                                         | What it does                                              |
| --------------------------------------------------------------- | --------------------------------------------------------- |
| [`clone`](https://wrangler.kaufmann.dev/docs/clone)             | Clone multiple GitHub repositories for a user or org.     |
| [`fetch`](https://wrangler.kaufmann.dev/docs/fetch)             | Fetch origin updates. Use `--prune` to prune stale refs.  |
| [`pull`](https://wrangler.kaufmann.dev/docs/pull)               | Pull latest changes for every discovered repository.      |
| [`push`](https://wrangler.kaufmann.dev/docs/push)               | Push local commits to origin. `--force` uses lease-based. |
| [`rename-repo`](https://wrangler.kaufmann.dev/docs/rename-repo) | Rename GitHub repositories through `gh`.                  |

### Local Operations

| Command                                                             | What it does                                                |
| ------------------------------------------------------------------- | ----------------------------------------------------------- |
| [`commit`](https://wrangler.kaufmann.dev/docs/commit)               | Generate and create one AI Conventional Commit per repo.    |
| [`fix-gitignore`](https://wrangler.kaufmann.dev/docs/fix-gitignore) | Add missing common generated-file patterns to `.gitignore`. |
| [`license`](https://wrangler.kaufmann.dev/docs/license)             | Add or replace MIT license files.                           |
| [`rename-branch`](https://wrangler.kaufmann.dev/docs/rename-branch) | Rename a branch across repositories.                        |
| [`reset`](https://wrangler.kaufmann.dev/docs/reset)                 | Reset current branches to their origin counterparts.        |
| [`review`](https://wrangler.kaufmann.dev/docs/review)               | Review unpushed changes across repositories.                |
| [`untrack`](https://wrangler.kaufmann.dev/docs/untrack)             | Stop tracking files already covered by `.gitignore`.        |

### History Rewriting

| Command                                                                 | What it does                                                    |
| ----------------------------------------------------------------------- | --------------------------------------------------------------- |
| [`remove-secrets`](https://wrangler.kaufmann.dev/docs/remove-secrets)   | Purge sensitive files from Git history.                         |
| [`rewrite-authors`](https://wrangler.kaufmann.dev/docs/rewrite-authors) | Rewrite author and committer identity.                          |
| [`rewrite-commits`](https://wrangler.kaufmann.dev/docs/rewrite-commits) | Generate AI Conventional Commit messages, then rewrite history. |
| [`rewrite-dates`](https://wrangler.kaufmann.dev/docs/rewrite-dates)     | Redistribute commit timestamps or roll back rewritten history.  |

### Utility

| Command                                                       | What it does                                        |
| ------------------------------------------------------------- | --------------------------------------------------- |
| [`activity`](https://wrangler.kaufmann.dev/docs/activity)     | Show an aggregated commit activity calendar.        |
| [`config`](https://wrangler.kaufmann.dev/docs/config)         | Show and edit Git Wrangler configuration.           |
| [`doctor`](https://wrangler.kaufmann.dev/docs/doctor)         | Check runtime dependencies and local configuration. |
| [`info`](https://wrangler.kaufmann.dev/docs/info)             | Show detailed repository information.               |
| [`init`](https://wrangler.kaufmann.dev/docs/init)             | Set up GitHub and AI credentials.                   |
| [`status`](https://wrangler.kaufmann.dev/docs/status)         | Show clean, dirty, ahead, behind, and remote state. |
| [`version`](https://wrangler.kaufmann.dev/docs/version)       | Print version metadata.                             |
| [`completion`](https://wrangler.kaufmann.dev/docs/completion) | Generate shell completion scripts.                  |
| [`help`](https://wrangler.kaufmann.dev/docs/help)             | Show help for Git Wrangler or a specific command.   |

## AI-Powered Workflows

`commit` and `rewrite-commits` use any OpenAI-compatible chat completions
API to generate Conventional Commit messages from your diffs.

AI context stays privacy-first: Git Wrangler sends file paths, stats, a compact
change summary, and redacted diff snippets, but not old commit messages. For
large or cross-cutting changes, use `--body` for rationale and prefer a stronger
model when a cheap/fast model still produces file-level summaries. Context is
collected and packed automatically with internal bounds.

```bash
# Set up credentials
git-wrangler init

# Or configure directly
git-wrangler config set ai.base-url https://api.openai.com/v1
git-wrangler config set ai.model gpt-4o
git-wrangler config set ai.api-key

# Optional gateway headers
git-wrangler config set ai.headers.X-Project-ID corp-dev-99
git-wrangler config set ai.headers.api-key
```

## Safety & Guardrails

Git Wrangler is built for bulk Git operations, where small mistakes can affect
many repositories at once. Destructive actions are therefore explicit, guarded,
and designed to fail safely.

- **Privacy by default** — AI commands redact diff content before sending it to
  the API, including sensitive file contents and common secret patterns. Old
  commit messages are not sent as context.
- **AI confirmation before staging** — `commit` prepares context with a
  temporary index and stages the real index only after valid AI messages are
  available.
- **Confirmation before mutation** — history rewrite commands ask before making
  destructive changes. Noninteractive runs that reach a confirmation fail with
  guidance to pass `--yes`. Use `--yes` or `-y` only for intentional
  noninteractive runs; they skip confirmations but never fill required values.
- **Safer force pushes** — `push --force` uses `--force-with-lease`. Raw force
  push requires the separate `--force-unsafe` flag.
- **Fail-safe bulk runs** — per-repository failures do not stop the whole run.
  Git Wrangler reports all failures and exits nonzero if anything failed.
- **Fresh remote-tracking refs** — remote-aware reports and history rewrite
  planning run `git fetch --prune origin` by default, with `--no-fetch` for
  explicit offline/local-only runs.
- **Origin preservation** — history rewrite commands that use `git-filter-repo`
  restore the `origin` remote after it is removed.
- **Exact date rollback** — `rewrite-dates --rollback` restores the first
  stored original baseline from backup refs, even after repeated rewrites,
  replaying only new commits made after that baseline.
- **Warnings on stderr** — destructive operations warn clearly without polluting
  normal command output.

## Runtime Dependencies

| Tool              | Required for                                                                                          |
| ----------------- | ----------------------------------------------------------------------------------------------------- |
| `git`             | All repository operations (required).                                                                 |
| `gh`              | GitHub operations: `clone`, `rename-repo`.                                                            |
| `git-filter-repo` | History rewrites: `remove-secrets`, `rewrite-authors`, `rewrite-commits`, and normal `rewrite-dates`. |

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

## Development

Install the local development build without using Homebrew:

```bash
scripts/install-dev
```

The script installs `git-wrangler` with `go install`, defaults `GOBIN` to
`~/.local/bin`, and writes bash completion to
`${XDG_DATA_HOME:-$HOME/.local/share}/bash-completion/completions/git-wrangler`.
It does not edit shell startup files. Put the binary directory before Homebrew
on your `PATH`:

```bash
export PATH="$HOME/.local/bin:$PATH"
```

New bash sessions should load completion automatically when `bash-completion`
is installed and loaded. To enable completion in the current shell immediately,
source the file printed by `scripts/install-dev`.

Run local checks before opening changes:

```bash
scripts/test
scripts/check
```

After making changes, run the script again and test the command normally:

```bash
scripts/install-dev
git-wrangler version
scripts/test
```
