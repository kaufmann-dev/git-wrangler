---
title: "Architecture"
description: "How Git Wrangler is built with Go, Cobra, and focused internal packages."
category: "General"
order: 3
---

# Architecture

Git Wrangler is a standard compiled Go CLI.

The executable entrypoint is intentionally tiny: `cmd/git-wrangler/main.go` calls `internal/cli.Execute()`, and the internal packages own the actual behavior.

## Command layer

`internal/cli` defines the Cobra root command, command groups, help text, flags, `version`, and `completion`.

The command groups are:

- Remote Operations
- Local Operations
- History Rewriting
- Utility

Cobra generates the help and completion commands. There is no external help metadata source.

## Package boundaries

`internal/repos` discovers repositories from the filesystem and computes display names. It does not run subprocesses.

`internal/git` owns Git command execution and `git-filter-repo` detection.

`internal/githubcli` owns `gh` command construction and execution.

`internal/run` wraps subprocess execution and supports fake command behavior in tests.

`internal/ui` owns output streams, color/plain behavior, prompts, and status vocabulary.

`internal/ai` owns AI-assisted commit message rewriting: context redaction, batching, OpenAI-compatible API calls, response validation, retry behavior, and callback generation.

`internal/version` exposes release metadata injected by GoReleaser.

## Release flow

GoReleaser builds native archives for macOS, Linux, and Windows on amd64 and arm64. It also publishes checksums, packages shell completions, and updates the `kaufmann-dev/homebrew-tap` cask.

The release workflow uses the latest GoReleaser v2 release.

Homebrew is the primary distribution path:

```bash
brew install --cask kaufmann-dev/tap/git-wrangler
```

GitHub Release binaries are the secondary distribution path.

## Adding a command

1. Add the Cobra command in `internal/cli`.
2. Put shared filesystem, Git, GitHub CLI, UI, or AI behavior in the owning internal package.
3. Add focused Go tests.
4. Run `scripts/check`.
