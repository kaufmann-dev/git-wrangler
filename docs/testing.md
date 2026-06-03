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
- Decline-as-skip confirmations.
- No multi-repository command asks one confirmation per repository.
- `--yes` skipping prompts without filling required values.
- stdout/stderr separation.
- Ordered output under concurrency.
- Exit code 1 when any per-repo operation fails.

Use temp-repository tests for real Git behavior only when fake runners would hide the risk. Destructive tests must operate only in `t.TempDir()`.

## Fake Runners

Prefer fake executables or `internal/run` fakes when validating command orchestration. Fake runners should assert the exact command shape that matters and return clear errors for unexpected commands.

Do not depend on global machine state unless the test is specifically about dependency detection.

## JSON Assertions

Each JSON command should have focused coverage that verifies:

- stdout is valid JSON.
- stdout contains no ANSI escape codes or progress text.
- stderr is empty for normal JSON execution.
- per-repo failures appear in `repositories[]` and return exit code 1.
- fatal command errors include `error.message`.
- `config show --json` never exposes stored secret values.

## Concurrency Ordering

Concurrency tests should prove overlap when worker caps matter and prove ordered output after collection. Keep sleeps short and use channels/mutexes instead of time-only assumptions where possible.

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
- Are stdout and stderr still separated by purpose?
- Is JSON mode unaffected or explicitly tested if the command supports `--json`?
- Do per-repo failures return nonzero without stopping ordered reporting?
- Do README, AGENTS, `docs/commands.md`, and `docs/cli-design.md` need updates?
