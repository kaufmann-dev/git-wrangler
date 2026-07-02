# Agent Guidelines for git-wrangler

Git Wrangler is a standard compiled Go CLI. Keep changes small, direct, and aligned with the existing package boundaries.

## Reference Docs

Do not automatically load these files into context with `AGENTS.md`; open them only when the task touches their subject:

- `docs/cli-design.md` — CLI UX rules for targeting, confirmations, JSON, streams, progress, status vocabulary, summaries, and result block spacing.
- `docs/commands.md` — command behavior matrix, shared flags, repository targeting, JSON shape, concurrency categories, and per-command notes.
- `docs/testing.md` — test taxonomy, fake-runner guidance, temp-repo rules, JSON/output assertions, concurrency ordering tests, and local vs CI checks.

## Architecture

`cmd/git-wrangler/main.go` must only call `internal/cli.Execute()`.

`internal/cli` owns Cobra command registration, command groups, generated help, flags, `version`, `completion`, and command wiring. Use `SilenceUsage: true` and `SilenceErrors: true`, and print command errors once to stderr.

`internal/cli` also owns command-local repository target selection, confirmation handling, JSON output, progress plumbing, and ordered result reporting. Commands that support `--repo` must use exact single-repository targeting through the shared helper; commands without `--repo` keep discovering repositories below the current directory.

Remote-aware read/report commands and history rewrite planning stay `origin`-centric. `status`, `info`, `review`, `remove-secrets`, `rewrite-authors`, `rewrite-commits`, `rewrite-dates`, and `rewrite-hours` refresh with `git fetch --prune origin` before inspecting remote-tracking refs unless `--no-fetch` is set. `rollback-rewrites` is local-only and never fetches. Fetch failures are per-repository failures for read/report commands and hard stops before planning or mutation for history rewrites.

Bulk per-repository work in `internal/cli` should use ordered worker patterns: read-only scans cap at 32 workers, independent Git mutations cap at 4 workers, and history rewrites that are not explicitly parallelized remain sequential. Workers must return result structs; print only after collection so repository output order stays stable. Confirmed AI and non-AI rewrite applications run repositories in parallel with the independent Git mutation cap.

Long-running bulk phases should report progress to stderr with the shared progress helper. Progress must not change ordered stdout summaries or interleave repository result blocks.

`internal/repos` is filesystem-only repository discovery and display-name handling. It discovers normal `.git` directories and linked worktree `.git` files with valid `gitdir:` pointers. It must not call Git, `gh`, or any subprocess.

`internal/repos` also owns exact repository resolution for `--repo`. It accepts normal worktrees and valid linked-worktree `.git` files, rejects non-repositories and bare repositories, and does not recurse below the supplied path.

`internal/git` owns Git subprocess behavior, including `git-filter-repo` detection and history rewrite helper execution. Support both `git-filter-repo` and `git filter-repo`, preferring the standalone executable.

`internal/config` owns non-secret JSON config at the user config path.

`internal/credentials` owns secret storage and resolution through `go-keyring`, with environment variable overrides and fallbacks. GitHub credentials resolve only from `GIT_WRANGLER_GITHUB_TOKEN` or Git Wrangler's keyring account; inbound `GH_TOKEN` is not a credential source.

`internal/auth` owns GitHub device OAuth and username lookup for `git-wrangler init`.

`internal/githubcli` owns `gh` subprocess behavior. `clone` and `rename-repo` must keep using `gh` as the GitHub transport and pass Git Wrangler-owned tokens through outbound `GH_TOKEN`/`GH_HOST` transport variables; do not reimplement repository listing, clone, rename, or edit flows.

`internal/run` owns command execution wrappers, optional streaming stdout with buffered fake-runner fallback, default subprocess timeouts, and concurrency-safe fake-command support for tests.

`internal/ui` owns output streams, colors, plain output behavior, status vocabulary, prompts, and terminal detection.

`internal/ai` owns AI commit creation and AI rewrite generation: redaction, batching, OpenAI-compatible chat completions calls, request pacing and concurrency, response validation, retry behavior, and callback generation. Merge commits (two or more parents) are excluded during scanning and counted as a dedicated skipped-merges stat. `RequireScope` gates both commit selection and generated-output validation: with it set, the prompt demands a scope and scopeless subjects are rejected as invalid; without it, scope stays optional per the Conventional Commits spec. AI batch request starts are paced by `--rpm` while in-flight requests are bounded by `--concurrency` (default 8, worker pool); keep these knobs orthogonal. All API calls share one pooled HTTP/1.1 client (HTTP/2 disabled); do not create per-request clients. Batch retries use up to four attempts with jittered exponential backoff, then per-commit retries and a final single-commit recovery pass. Transient retry attempts are aggregated into one end-of-run summary progress event instead of per-attempt output.

`internal/version` owns `Version`, `Commit`, and `Date`, defaulting to `dev`, `unknown`, and `unknown`. GoReleaser injects release values with ldflags.

## Commands

Keep these public commands unless the user explicitly asks to change the surface:

`activity`, `clone`, `commit`, `config`, `doctor`, `fetch`, `fix-gitignore`, `info`, `init`, `license`, `log`, `pull`, `push`, `remove-secrets`, `rename-branch`, `rename-repo`, `reset`, `review`, `rewrite-authors`, `rewrite-commits`, `rewrite-dates`, `rewrite-hours`, `rollback-rewrites`, `status`, `untrack`, `version`, and Cobra-generated `completion` and `help`.

Do not restore `update` or `uninstall`. Updates are handled by Homebrew, Scoop, or manual replacement of release binaries.

`--yes` is command-local and skips confirmations only. It must not fill required values such as names, branch names, config values, API keys, or secrets. Multi-repository commands must ask at most one confirmation for the whole candidate set, never once per repository. Declining a confirmation before mutation is a successful skip/no-op, not a failure.

`--guided` is command-local on repository workflow commands. It requires both stdin and stderr to be TTYs, prompts for every command-specific behavior option but no meta flags, applies answers through Cobra flag setters, and prints a selected-configuration summary to stderr before execution. Missing required values prompt by default only when both streams are TTYs. `--guided --json` is invalid. Non-TTY confirmations fail with guidance to pass `--yes`; `--yes` and `-y` skip confirmations only.

All interactive prompts use the shared context-aware prompt session. `Ctrl+C` and interactive EOF/`Ctrl+D` cancel the active prompt immediately, stop later prompts and work, print `SKIP stopped: operation cancelled` exactly once, and exit nonzero. Cancellation must not be treated as an empty required value, declined confirmation, invalid guided answer, or missing secret. Secret prompt cancellation must restore terminal state before returning.

`--json` is command-local and limited to `status`, `info`, `review`, `doctor`, `config show`, and `version`. JSON mode writes one document to stdout, suppresses colors/progress/prompts/human summaries, keeps stderr empty except Cobra parse errors or unavoidable process-level failures, and must not expose stored secrets.

`--no-fetch` is command-local and limited to `status`, `info`, `review`, `remove-secrets`, `rewrite-authors`, `rewrite-commits`, `rewrite-dates`, and `rewrite-hours`. It skips only the automatic origin refresh; it must not change repository targeting or other command behavior.

## Runtime Dependencies

Normal CLI use may depend on:

- `git`
- `gh` for GitHub repository operations
- `git-filter-repo` for history rewrite operations

Do not add Python, Node, npm, pnpm, Go, or shell-script runtimes as normal CLI dependencies.

## Release

Use GoReleaser for release builds, GitHub Release archives, checksums, completions, Homebrew tap cask updates, and Scoop bucket updates. CI uses the latest GoReleaser v2 release.

The Homebrew cask is generated for `kaufmann-dev/homebrew-tap` with dependencies on `git`, `gh`, and `git-filter-repo`. It must install bash, zsh, and fish completions from release archives.

GitHub OAuth setup uses the public client ID embedded in `internal/auth.GitHubOAuthClientID`.

The Scoop manifest is generated for `kaufmann-dev/scoop-bucket` with dependencies on `git`, `gh`, and `git-filter-repo`. Release automation requires `SCOOP_BUCKET_GITHUB_TOKEN` with write access to that bucket.

Local release dry run:

```bash
goreleaser release --snapshot --clean
```

## Tests

Use Go tests with `testing`, `t.TempDir`, and fake executables or `internal/run` fakes where subprocess behavior matters. Mutation tests must operate only in temporary repositories.

Fast local checks:

```bash
go test ./...
go vet ./...
goreleaser check
git diff --check
```

`scripts/check` wraps `git diff --check`, Go tests, vet, and optional GoReleaser v2 checks. `scripts/test` runs `go test ./...`. `scripts/bench` builds a temporary CLI binary and times read-only status checks against temporary repositories.

CI owns slower checks: `go test -race ./...`, `govulncheck ./...`, `goreleaser release --snapshot --clean`, release archive/completion/package smoke checks, and `git diff --check`.

## History Rewrite Safety

Keep this as a short top-level section because history rewrite commands are destructive and cross-cut several packages; put detailed command behavior in `docs/commands.md`.

History rewrite commands must require explicit confirmation before mutation, with `--yes` as the standard noninteractive confirmation flag. Capture and restore `origin` when `git-filter-repo` removes it. Print warnings to stderr for destructive operations. Bulk commands must return nonzero if any repository operation fails; no-op skips and declined confirmations remain successful.

`rewrite-authors`, `rewrite-commits`, `rewrite-dates`, and `rewrite-hours` share a command-neutral cumulative baseline under `.git/git-wrangler/baseline/`. Existing baseline metadata must not be overwritten; later rewrites add only newly eligible commits and update current SHA mappings from `.git/filter-repo/commit-map`. `rollback-rewrites` restores that shared baseline across commands while preserving commits created between rewrite runs, replaying unbaselined commits only when parent links must be reconnected. `remove-secrets` is the only history rewrite command that clears the shared baseline and creates no replacement baseline.

`rewrite-dates --window HH:MM-HH:MM` uses the existing date planner with one uniform time window for every targeted repository and every day. `rewrite-dates --window mon-fri=HH:MM-HH:MM` applies explicit windows only to listed days and uses generated per-day working hours for omitted days. Omitting `--window` preserves generated per-day working hours. `rewrite-hours --window HH:MM-HH:MM` is required, preserves each commit's current calendar date, and rewrites author and committer dates into the window. `rewrite-hours` day-of-week schedules rewrite only listed days and leave omitted days unchanged. Do not restore `rewrite-dates --rollback`; rollback is handled by `rollback-rewrites`.

AI-backed commands must fail before scanning repositories when no API key is available. They must not save plaintext API keys, and they must redact sensitive file contents and common secret patterns before API calls.
