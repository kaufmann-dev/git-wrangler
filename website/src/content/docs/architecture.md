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
- AI Commands
- History Rewriting
- Utility

Cobra generates help output and the completion command. There is no external help metadata source.

## Package boundaries

`internal/repos` discovers repositories from the filesystem and computes display names. It does not run subprocesses.

`internal/git` owns Git command execution and `git-filter-repo` detection.

`internal/config` owns non-secret JSON config.

`internal/credentials` owns keyring-backed secret storage and environment credential resolution.

`internal/auth` owns GitHub device OAuth setup.

`internal/githubcli` owns `gh` command construction and execution.

`internal/run` wraps subprocess execution and supports fake command behavior in tests.

`internal/ui` owns output streams, color/plain behavior, prompts, and status vocabulary.

`internal/ai` owns AI commit creation and AI rewrite generation: context redaction, batching, OpenAI-compatible API calls, response validation, retry behavior, and callback generation.

`internal/version` exposes release metadata injected by GoReleaser.

## Release flow

GoReleaser builds native archives for macOS, Linux, and Windows on amd64 and arm64. It also publishes checksums, packages shell completions, and updates package-manager metadata.

The release workflow uses the latest GoReleaser v2 release.

Homebrew is the package-manager path for macOS and Linux:

```bash
brew install kaufmann-dev/tap/git-wrangler
```

Scoop is the native Windows package-manager path:

```powershell
scoop bucket add kaufmann-dev https://github.com/kaufmann-dev/scoop-bucket.git
scoop install kaufmann-dev/git-wrangler
```

GitHub Release binaries are the secondary distribution path.

## Adding a command

1. Add the Cobra command in `internal/cli`.
2. Put shared filesystem, Git, GitHub CLI, UI, or AI behavior in the owning internal package.
3. Add focused Go tests.
4. Run `scripts/check`.
