# Agent Guidelines for git-wrangler

Git Wrangler is a standard compiled Go CLI. Keep changes small, direct, and aligned with the existing package boundaries.

## Architecture

`cmd/git-wrangler/main.go` must only call `internal/cli.Execute()`.

`internal/cli` owns Cobra command registration, command groups, generated help, flags, `version`, `completion`, and command wiring. Use `SilenceUsage: true` and `SilenceErrors: true`, and print command errors once to stderr.

`internal/repos` is filesystem-only repository discovery and display-name handling. It discovers normal `.git` directories and linked worktree `.git` files with valid `gitdir:` pointers. It must not call Git, `gh`, or any subprocess.

`internal/git` owns Git subprocess behavior, including `git-filter-repo` detection and history rewrite helper execution. Support both `git-filter-repo` and `git filter-repo`, preferring the standalone executable.

`internal/config` owns non-secret JSON config at the user config path.

`internal/credentials` owns secret storage and resolution through `go-keyring`, with environment variable overrides and fallbacks.

`internal/auth` owns GitHub device OAuth and username lookup for `git-wrangler init`.

`internal/githubcli` owns `gh` subprocess behavior. `clone` and `rename-repo` must keep using `gh` as the GitHub transport and pass Git Wrangler-owned tokens through `GH_TOKEN`/`GH_HOST`; do not reimplement repository listing, clone, rename, or edit flows.

`internal/run` owns command execution wrappers, default subprocess timeouts, and concurrency-safe fake-command support for tests.

`internal/ui` owns output streams, colors, plain output behavior, status vocabulary, prompts, and terminal detection.

`internal/ai` owns `rewrite-commits-ai`: redaction, batching, OpenAI-compatible chat completions calls, response validation, retry behavior, and callback generation.

`internal/version` owns `Version`, `Commit`, and `Date`, defaulting to `dev`, `unknown`, and `unknown`. GoReleaser injects release values with ldflags.

## Commands

Keep these public commands unless the user explicitly asks to change the surface:

`clone`, `commit`, `config`, `doctor`, `fix-gitignore`, `info`, `init`, `license`, `pull`, `push`, `remove-secrets`, `rename-branch`, `rename-repo`, `reset`, `review`, `rewrite-authors`, `rewrite-commits`, `rewrite-commits-ai`, `rewrite-dates`, `status`, `untrack`, `version`, and Cobra-generated `completion` and `help`.

Do not restore `update` or `uninstall`. Updates are handled by Homebrew or manual replacement of release binaries.

## Runtime Dependencies

Normal CLI use may depend on:

- `git`
- `gh` for GitHub repository operations
- `git-filter-repo` for history rewrite operations

Do not add Python, Node, npm, pnpm, Go, or shell-script runtimes as normal CLI dependencies.

## Release

Use GoReleaser for release builds, GitHub Release archives, checksums, completions, and Homebrew tap cask updates. CI uses the latest GoReleaser v2 release.

The Homebrew cask is generated for `kaufmann-dev/homebrew-tap` with dependencies on `git`, `gh`, and `git-filter-repo`. It must install bash, zsh, and fish completions from release archives.

GitHub OAuth setup uses the public client ID embedded in `internal/auth.GitHubOAuthClientID`.

Local release dry run:

```bash
goreleaser release --snapshot --clean
```

## Tests

Use Go tests with `testing`, `t.TempDir`, and fake executables or `internal/run` fakes where subprocess behavior matters. Mutation tests must operate only in temporary repositories.

Required checks:

```bash
go test ./...
go test -race ./...
go vet ./...
govulncheck ./...
goreleaser check
goreleaser release --snapshot --clean
git diff --check
cd website && pnpm run check
cd website && pnpm run build
cd website && pnpm audit --audit-level moderate
```

`scripts/check` wraps `git diff --check`, Go tests, race tests, vet, optional `govulncheck`, GoReleaser v2 checks, and website check/build/audit when local website dependencies are installed. `scripts/test` runs `go test ./...`. `scripts/bench` builds a temporary CLI binary and times read-only status checks against temporary repositories.

## History Rewrite Safety

History rewrite commands must require explicit confirmation before mutation, with `--yes` as the standard noninteractive confirmation flag. Capture and restore `origin` when `git-filter-repo` removes it. Print warnings to stderr for destructive operations. Bulk commands must return nonzero if any repository operation fails; no-op skips remain successful.

`rewrite-commits-ai` must fail before scanning repositories when no API key is available. It must not save plaintext API keys, must not send old commit messages as model context, and must redact sensitive file contents and common secret patterns before API calls.
