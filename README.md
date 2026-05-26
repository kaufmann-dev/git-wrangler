# Git Wrangler

Git Wrangler is a compiled Go CLI that orchestrates Git operations across every repository in a directory.

## Install

Primary install path:

```bash
brew install --cask kaufmann-dev/tap/git-wrangler
```

Upgrade with Homebrew:

```bash
brew upgrade --cask git-wrangler
```

Homebrew installs shell completions automatically. Linux and Windows users can download binaries from GitHub Releases.

## Runtime Dependencies

`git` is required for normal repository operations.

`gh` is required for GitHub repository operations such as `clone` and `rename-repo`. Run `gh auth login` before private or all-repository GitHub workflows.

`git-filter-repo` is required for history rewrite commands: `remove-secrets`, `rewrite-authors`, `rewrite-commits`, `rewrite-commits-ai`, and `rewrite-dates`.

`rewrite-commits-ai` also needs an OpenAI-compatible chat completions endpoint, a model name, and an API key.

Manual binary installs do not install runtime dependencies. Install `git`, `gh`, and `git-filter-repo` yourself as needed for the commands you run.

## Quick Start

```bash
git-wrangler clone --user myusername --visibility public --into ./repos
git-wrangler status
git-wrangler pull --rebase
git-wrangler commit --message "chore: update dependencies"
```

Run `git-wrangler --help` for the full command list and `git-wrangler <command> --help` for command-specific flags.

Repository discovery supports regular `.git` directories and linked worktree `.git` files with valid `gitdir:` pointers. Bulk commands process discovered repositories in deterministic order. If any repository operation fails, the command exits nonzero after processing the remaining repositories; clean no-op skips still exit successfully.

## Commands

Remote operations:

- `clone`
- `pull`
- `push`
- `rename-repo`

Local operations:

- `commit`
- `fix-gitignore`
- `license`
- `rename-branch`
- `reset`
- `review`
- `untrack`

History rewriting:

- `remove-secrets`
- `rewrite-authors`
- `rewrite-commits`
- `rewrite-commits-ai`
- `rewrite-dates`

Utility:

- `completion`
- `info`
- `status`
- `version`

## Safety

Destructive history rewrite commands require confirmation before mutation. `--yes` skips confirmation prompts for noninteractive runs. `rewrite-commits-ai` asks before sending redacted context to the configured API and asks again before applying generated messages unless `--yes` is supplied.

Non-history commands that create commits or discard state also require explicit intent: `fix-gitignore --yes`, `untrack --yes`, and `reset --yes` skip the interactive prompts.

## Architecture

The public command implementation lives in Go under `cmd/git-wrangler` and `internal/`.

`cmd/git-wrangler/main.go` only calls `internal/cli.Execute()`. Cobra owns command registration, help, flags, command groups, version output, and shell completions.

Package boundaries:

- `internal/cli` wires Cobra commands to command behavior.
- `internal/repos` discovers repositories from the filesystem and computes display names. It must not run subprocesses.
- `internal/git` owns Git subprocess behavior, including `git-filter-repo` detection.
- `internal/githubcli` owns `gh` subprocess behavior.
- `internal/run` wraps command execution and supports test fakes.
- `internal/ui` owns output styling and plain/color behavior.
- `internal/ai` owns AI commit rewrite context generation, redaction, batching, response validation, and callback generation.
- `internal/version` exposes ldflags-injected release metadata.

## Release

Releases are built with GoReleaser. Tagged releases publish GitHub Release archives, checksums, completions, and update the `kaufmann-dev/homebrew-tap` cask.

Local dry run:

```bash
goreleaser release --snapshot --clean
```

Contributor checks:

```bash
git diff --check
go test ./...
go test -race ./...
go vet ./...
govulncheck ./...
goreleaser check
goreleaser release --snapshot --clean
cd website && pnpm run check
cd website && pnpm run build
cd website && pnpm audit --audit-level moderate
```
