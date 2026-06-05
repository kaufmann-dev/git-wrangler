# Testing

Git Wrangler tests should stay fast, isolated, and focused on command behavior rather than the user's real repositories.

## Test Taxonomy

Use unit tests for pure helpers:

- `internal/repos` exact resolution and discovery.
- CLI targeting helpers.
- Confirmation helper behavior.
- JSON writer and JSON-mode suppression.
- Worker-count helpers and ordered result collection.

Use fake-runner command tests for subprocess orchestration:

- Exact `--repo` targeting per command family.
- Automatic `git fetch --prune origin` orchestration for remote-aware commands.
- Decline-as-skip confirmations.
- No multi-repository command asks one confirmation per repository.
- `--yes` skipping prompts without filling required values.
- stdout/stderr separation.
- Ordered output under concurrency.
- durable output printed only after progress closes.
- Exit code 1 when any per-repo operation fails.

Use temp-repository tests for real Git behavior only when fake runners would hide the risk. Destructive tests must operate only in `t.TempDir()`.

## Fake Runners

Prefer fake executables or `internal/run` fakes when validating command orchestration. Fake runners should assert the exact command shape that matters and return clear errors for unexpected commands.

Remote-aware command fakes should account for the default `git fetch --prune origin` phase before inspection or history rewrite planning. Use `--no-fetch` in tests that are explicitly about local-only behavior or that need to isolate unrelated command contracts.

Do not depend on global machine state unless the test is specifically about dependency detection.

## JSON Assertions

Each JSON command should have focused coverage that verifies:

- stdout is valid JSON.
- stdout contains no ANSI escape codes or progress text.
- stderr is empty for normal JSON execution.
- default auto-fetch success is silent.
- fetch failures for `status`, `info`, and `review` appear as per-repository errors with empty stderr.
- per-repo failures appear in `repositories[]` and return exit code 1.
- fatal command errors include `error.message`.
- `config show --json` never exposes stored secret values.

## Concurrency Ordering

Concurrency tests should prove overlap when worker caps matter. Prove ordered output after collection only for commands that still print per-repo result blocks or actionable per-repo skips/failures. Commands that suppress routine success spam should assert summaries and the absence of success chatter instead.

## Human Output Assertions

Human output tests should verify:

- progress stays on stderr and never appears inside stdout tables or repo blocks.
- durable tables, previews, summaries, and repo blocks print after progress closes.
- routine bulk success is summarized rather than printed once per repository.
- actionable skips and failures still identify the repository.
- color-disabled output remains readable with state text such as `OK`, `WARN`, `ERROR`, and `SKIP`.
- destructive commands warn and prompt exactly once for the candidate set.

## Local Checks

Fast local checks:

```bash
scripts/test
scripts/check
```

`scripts/test` runs `go test ./...`.

`scripts/check` runs:

- `git diff --check`
- `go test ./...`
- `go vet ./...`
- optional `goreleaser check` when GoReleaser is installed and new enough

## CI Checks

GitHub CI owns broader verification:

- `go test ./...`
- `go test -race ./...`
- `go vet ./...`
- `govulncheck ./...`
- `goreleaser release --snapshot --clean`
- release archive, completion, Homebrew cask, and Scoop manifest smoke checks
- `git diff --check`

## Command-Change Checklist

When changing a command, check:

- Does `--repo` use exact targeting if the command supports it?
- Does a declined confirmation skip successfully before mutation?
- Does a multi-repository command ask at most one confirmation for the whole candidate set?
- Does `--yes` skip only confirmations?
- If the command relies on remote-tracking refs, does it refresh `origin` by default and honor `--no-fetch`?
- For history rewrites, does fetch failure stop before scan, AI requests, prompts, and mutation?
- Are stdout and stderr still separated by purpose?
- Is JSON mode unaffected or explicitly tested if the command supports `--json`?
- Do per-repo failures return nonzero without stopping ordered reporting?
- Do README, AGENTS, `docs/commands.md`, and `docs/cli-design.md` need updates?
