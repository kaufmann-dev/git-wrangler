# Git Wrangler

Git Wrangler is a compiled Go CLI that orchestrates Git operations across every repository in a directory.

## Install

Primary install path:

```bash
brew install kaufmann-dev/tap/git-wrangler
```

Upgrade with Homebrew:

```bash
brew upgrade git-wrangler
```

Homebrew installs shell completions automatically. Linux and Windows users can download binaries from GitHub Releases.

## Runtime Dependencies

`git` is required for normal repository operations.

`gh` is required for GitHub repository operations such as `clone` and `rename-repo`. Run `gh auth login` before private or all-repository GitHub workflows.

`git-filter-repo` is required for history rewrite commands: `remove-secrets`, `rewrite-authors`, `rewrite-commits`, `rewrite-commits-ai`, and `rewrite-dates`.

`rewrite-commits-ai` also needs an OpenAI-compatible chat completions endpoint, a model name, and an API key.

Check your machine with:

```bash
git-wrangler doctor
```

## Quick Start

```bash
git-wrangler clone --user myusername --visibility public --into ./repos
git-wrangler status
git-wrangler pull --rebase
git-wrangler commit --message "chore: update dependencies"
```

Run `git-wrangler help` for the full command list and `git-wrangler help <command>` for command-specific flags.

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
- `doctor`
- `info`
- `status`
- `version`

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

Releases are built with GoReleaser. Tagged releases publish GitHub Release archives, checksums, completions, and update the `kaufmann-dev/homebrew-tap` formula.

CI pins GoReleaser `v2.9.0` so formula publishing remains compatible with the Homebrew install model.

Local dry run:

```bash
goreleaser release --snapshot --clean
```

Contributor checks:

```bash
go test ./...
go test -race ./...
go vet ./...
goreleaser check
git diff --check
```
