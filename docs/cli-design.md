# CLI Design

Git Wrangler is a human-first Go/Cobra CLI. Keep generated Cobra `help` and `completion`, command-local flags, and output that reads clearly during repeated multi-repository use.

## Command Surface

Public commands are grouped as remote, local, history, and utility commands in `internal/cli/root.go`. Do not add global scriptability surfaces. JSON exists only on read-only/introspection commands: `status`, `info`, `review`, `doctor`, `config show`, and `version`.

Do not restore `update` or `uninstall`. Distribution tools own updates and removal.

## Repository Targeting

Default repository targeting discovers worktrees below the current directory. `--repo PATH` is exact targeting: it resolves one normal or linked-worktree repository and does not recurse below it. Non-repository paths and bare repositories are errors.

Exact targeting belongs in `internal/repos` and the shared helper in `internal/cli`. Command implementations should not hand-roll repository discovery.

## Confirmations

`--yes` is command-local and skips confirmations only. It must not supply required values such as names, branches, config values, API keys, or secrets.

Multi-repository commands must ask at most one confirmation for the whole candidate set. A confirmation must never be asked N times simply because there are N repositories.

A user-declined confirmation before mutation is a skip/no-op, not a failure. Validation errors, missing dependencies, failed per-repo operations, partial failures, and cleanup failures still return nonzero.

`license` creates missing `LICENSE` files without confirmation. `license --overwrite` prompts once before replacing all existing `LICENSE` files in the candidate set, and `--yes` skips only that overwrite prompt.

## Output Streams

Use stdout for normal command results, per-repo result blocks, final summaries, and JSON documents.

Use stderr for prompts, warnings, progress, and errors.

JSON mode writes exactly one document to stdout. It suppresses progress, colors, prompts, and human summaries. Stderr should stay empty except for Cobra parse errors or unavoidable process-level failures.

## Status Vocabulary

Use the shared helpers in `internal/cli/helpers.go` for status lines when possible:

- `OK` / success
- `WARN` / warning
- `ERROR` / failure
- `SKIP` / no-op
- `INFO` / context

Avoid direct colored `fmt.Fprintf` in new repeated output paths. Existing direct output can stay unless changing it simplifies repeated behavior or fixes stream separation.

## Progress And Ordering

Long-running bulk phases should report progress to stderr through `newProgress`. Progress must not interleave with ordered stdout summaries or per-repo result blocks.

Workers return result structs. Print only after collection so repository output order stays stable.

Worker categories:

- Read-only scans cap at 32 workers.
- Independent Git mutations cap at 4 workers.
- Confirmed history rewrite apply phases use the mutation cap.
- `clone`, `rename-repo`, `license`, and `push --force-unsafe` stay sequential.

## Result Blocks

Per-repo blocks should be compact and separated by blank lines only when the command prints multi-line details. Summaries should be short count lines at the end of human output.

Keep cosmetic changes narrow. Output changes should trace to readability, stream separation, JSON suppression, or consistent status wording.
