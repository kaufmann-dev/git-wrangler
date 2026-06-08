# CLI Design

## North Star

Git Wrangler is a human-first fleet console for many repositories. Output should feel calm, dense, and predictable when a user runs one command across a large workspace.

Default human output is the product surface. Do not add scriptability-first output, pagers, global verbosity flags, or machine contracts unless the command explicitly supports JSON or the user asks for that surface.

Keep Cobra-generated `help` and `completion`. Keep the root landing banner.

## Command Rhythm

Use this rhythm for human commands:

1. Validate flags, dependencies, config, and credentials.
2. Discover targets.
3. Refresh `origin` with transient `Fetching repositories` progress when a command relies on remote-tracking refs.
4. Show transient progress on stderr for long-running scan phases.
5. Print durable previews or results on stdout after progress is closed.
6. Print destructive warnings and prompts on stderr when needed.
7. Mutate with transient progress on stderr.
8. Print concise final summaries on stdout.

Progress is never durable output. Tables, previews, summaries, and repo blocks must be printed only after active progress has been closed.

## Streams

Stdout is for durable command results: tables, repo blocks, summaries, normal status lines, JSON documents, and non-secret setup/config recaps.

Stderr is for progress, prompts, warnings, errors, and captured subprocess failure output.

JSON mode writes exactly one JSON document to stdout. It suppresses progress, colors, prompts, warnings, and human summaries. Stderr should stay empty except Cobra parse errors or unavoidable process-level failures.

## Visual Language

Use sentence-case human text and stable labels. Prefer `Repository`, `State`, `Tracking`, `Changed`, `Skipped`, `Failed`, `OK`, `WARN`, `ERROR`, `SKIP`, and `INFO`.

Color supports meaning but never carries meaning alone:

- Green: successful mutation or healthy check.
- Yellow: skip, dirty state, warning, or user-declined no-op.
- Red: error, failed check, behind state, or destructive warning.
- Cyan: informational values such as ahead counts, auth source, current branch, or active target.
- Muted gray: secondary or absent values such as `no remote`, `<unset>`, or `not configured`.
- Bold blue: repository names in multi-line repo blocks when it improves scanning.

Symbols are optional and TTY-only through `internal/ui`. Plain output must remain readable as text.

Do not use decorative boxes, emojis, gradients in command output, or heavy separators. Keep output grep-readable.

## Progress

Use `newProgress` for long-running bulk phases. Prefer one progress surface per phase, such as `Checking status`, `Pulling repositories`, `Sending API requests`, or `Applying AI rewrites`.

Remote-aware reporting and history rewrite planning commands use `Fetching repositories` progress for their automatic `git fetch --prune origin` refresh in human output. JSON mode has no progress, including no fetch progress.

Interactive progress is transient. Non-TTY progress is line-oriented and throttled. JSON mode has no progress.

Call `finishProgressBeforeOutput` or rely on worker helpers that close progress before printing durable output. Do not print tables, previews, summaries, repo blocks, warnings, prompts, or error blocks while progress is active.

## Tables

Use tables only for comparable dense data: `status`, reset previews, and `doctor` checks.

Tables must use ANSI-aware width calculation. Do not hard-code separator lines in command files. Avoid heavy borders unless the project explicitly chooses them.

Long repository names may be truncated with `...` when a command needs compact scan output.

## Repo Blocks

Use repo blocks for multi-line actionable detail: `review`, `fix-gitignore`, `untrack`, `remove-secrets`, and generated rewrite previews.

Separate repo blocks with exactly one blank line. Do not print clean/no-change repositories one by one unless the command is explicitly a detailed report like `info`.

## Warnings And Prompts

Destructive warnings use one style through `renderWarning`. They go to stderr and describe the irreversible action concretely.

Setup prompts, guided prompts, secret prompts, and final confirmations use the shared prompt session. Prompting is available only when both stdin and stderr are TTYs. Tests inject prompt eligibility and streams through that session.

`Ctrl+C` and interactive EOF/`Ctrl+D` cancel the active prompt immediately without waiting for Enter. Cancellation stops later prompts and command work, restores terminal state after secret input, prints `SKIP stopped: operation cancelled` exactly once, and exits nonzero. It is distinct from an empty required value, declined confirmation, invalid guided answer, or missing secret.

Missing required values prompt by default when prompting is available and fail otherwise. `--yes` and `-y` skip confirmations only; they must not fill required values such as names, branches, config values, API keys, or secrets.

Guided commands use command-local `--guided` to prompt for every command-specific behavior option, including targeting and fetch behavior. They never prompt for or summarize meta flags such as `--help`, `--version`, `--guided`, `--yes`, or `--json`. Guided answers are applied through Cobra flag setters, and the selected configuration is summarized on stderr before normal validation and execution. `--guided` requires prompting availability and cannot be combined with `--json`.

Multi-repository commands ask at most one confirmation for the candidate set. A confirmation reached without prompting availability fails nonzero and tells the user to pass `--yes`.

A declined confirmation before mutation is a successful skip/no-op. It should be counted in the summary, not treated as a failure.

## Summaries

Every bulk human command ends with `Summary:` on stdout. Use a consistent count order per command. Color numeric/state values, not punctuation.

Preferred count orders:

- `status`: clean, dirty, behind, no remote, failed.
- `activity`: commits, repositories, failed.
- `pull`: updated, skipped, failed.
- `fetch`: fetched, failed.
- `push`: pushed, skipped, failed.
- `clone`: cloned, skipped, failed.
- `commit`: committed, skipped, failed.
- `fix-gitignore`: with changes, unchanged, failed; then updated, skipped, failed after apply.
- `license`: created, overwritten, skipped, failed.
- `rename-branch`: renamed, skipped, failed.
- `reset`: reset, skipped, failed.
- `review`: with unpushed changes, clean, failed.
- `untrack`: with tracked ignored files, unchanged, failed; then updated, skipped, failed after apply.
- `remove-secrets`: rewritten, clean, skipped, failed.
- `rewrite-authors`: rewritten, skipped, failed.
- `rewrite-commits`: commit messages rewritten, repositories updated, failed.
- `rewrite-dates`: rewritten, skipped, failed for normal rewrites; rolled back, skipped, failed for rollback.
- `rename-repo`: renamed, description updated, skipped, failed.

## Per-Command Output Contract

### `status`

Refresh `origin` first unless `--no-fetch` is set, then show progress while checking repositories. Print one dense table with `Repository`, `State`, and `Tracking`, followed by the standard summary. Rows with fetch or inspection failures should show `ERROR` where possible, with details on stderr. JSON mode keeps fetch failures in repository rows and keeps stderr empty.

### `activity`

Show `Scanning activity` progress without fetching. After progress closes, print fallback warnings and per-repository error blocks, then one aggregated calendar and a `commits`, `repositories`, `failed` summary. Keep years newest first, weeks Sunday-first, and include month and weekday labels. Plain output uses `.`, `1`, `2`, `3`, and `4`; TTY output uses GitHub-style green cells. Show the effective maximum in year headings or the shared global-scale heading.

### `pull`, `fetch`, and `push`

Show one progress line. Suppress routine per-repo success lines. Print actionable skips such as already up to date or nothing to push. Print failures as error blocks. End with a summary.

`push --force-unsafe` remains sequential after one confirmation but uses the same summary style.

### `clone`

Show GitHub auth source once when auth is used. When secure credential storage is unavailable, hide backend errors and direct authenticated cloning to `GIT_WRANGLER_GITHUB_TOKEN`; public cloning continues without auth. Clone sequentially. Print existing-directory skips and failures. Suppress routine cloned lines for large runs; one or two individual success lines are acceptable. End with `cloned`, `skipped`, and `failed`.

### `commit`

Prepare AI commit context with progress. Before network calls, print a data-send notice containing endpoint, model, repository count, context budget, content description, and secret handling. Prompt on stderr. Show API progress with inline retry/detail text, then commit creation progress. Print only failures/skips plus summary unless there is a single small success surface.

### `fix-gitignore`

Scan first. Print candidate repo blocks only for proposed additions. Count clean/no-change repositories in the scan summary. Prompt once, apply with progress, then print the apply summary.

### `license`

Print conflicts, skips, and failures. Suppress routine success lines. `--overwrite` prompts once for existing files. Summarize `created`, `overwritten`, `skipped`, and `failed`.

### `rename-branch`

Progress while checking and applying. Suppress successful rename spam. Print skips for missing source branches or existing targets, print failures, then summarize.

### `reset`

Preparation progress completes before any preview. Print a reset table with repository, branch, ahead, behind, and dirty state for candidates. Print one destructive warning and one prompt. Apply with progress and summarize. Detached HEAD, missing upstream, and already-up-to-date states are skips.

### `review`

Refresh `origin` first unless `--no-fetch` is set, then show review progress. Print only repositories with unpushed changes or errors. Keep per-repo file-change blocks because they are the command result. Added, edited, and removed labels are aligned and printed to stdout. Summarize changed, clean, and failed counts. JSON mode keeps fetch failures in repository rows and keeps stderr empty.

### `untrack`

Scan first. Print candidate blocks only for repositories with tracked ignored files. Count missing `.gitignore` and no-match repositories. Prompt once, apply with progress, and summarize.

### `remove-secrets`

Refresh `origin` first unless `--no-fetch` is set. Fetch failures stop before scan, preview, prompt, or mutation. `--no-fetch` prints a warning before scanning. Scan history with progress. Always show matched files for affected repositories because this is safety-critical. Count clean repositories. Warn and prompt once. Apply with progress and summarize.

### `rewrite-authors`

Refresh `origin` first unless `--no-fetch` is set. Fetch failures stop before the destructive notice, prompt, or mutation. `--no-fetch` prints a warning before continuing. Print a concise destructive notice with repository count and new identity. Warn and prompt once. Apply with progress. Suppress per-repo success lines, print origin-restore warnings and failures, and summarize.

### `rewrite-commits`

Validate AI settings, refresh `origin` unless `--no-fetch` is set, then use phases for repository scanning, commit scanning, API requests, generated preview, destructive warning/prompt, and applying rewrites. Fetch failures stop before scanning or AI requests. `--no-fetch` prints a warning before scanning and before the normal AI data-send confirmation path. Retry details must stay inline or in clean progress logs, never interleaved with durable output. Keep the generated plan preview. Final success is aggregate.

### `rewrite-dates`

Refresh `origin` first unless `--no-fetch` is set. Fetch failures stop before scanning, planning, preview, prompt, or mutation. `--no-fetch` prints a warning before scanning. Preparation progress completes before durable output. Normal rewrite preview starts with a global date-plan header containing repository count, selected commit count, target range, planning timezone, active days, rest days, forced-active days, final post-topology median/p90/maximum commits per active day, filters, frequency, spread, and seed source. Do not show fixed-threshold workload concepts such as `5+/day`. Candidate repo blocks show selected commits, planned range, timezone, compact old-to-new samples, and tag/signature warnings when detected. Rollback preview starts with repositories, known commits, unknown/new commits, exact branch restores, branches that replay new commits, and skipped branches; repo blocks show branch action counts plus samples. Warn and prompt once. Apply with progress and summarize `rewritten`, `skipped`, and `failed` for normal rewrites or `rolled back`, `skipped`, and `failed` for rollback.

### `info`

Refresh `origin` first unless `--no-fetch` is set. Keep the detailed multi-line report. Use aligned key/value rows. Separate repositories with exactly one blank line. Add a summary only for multi-repo runs with failures. JSON mode keeps fetch failures in repository rows and keeps stderr empty.

### `doctor`

Print `Git Wrangler Doctor`, a runtime key/value section, a dependency check table, and a config/auth check table. Use `ERROR` only for critical failures that make doctor exit nonzero. Warnings should not imply the CLI is broken.

### `init`

Require prompting availability before doing work. Keep prompts explicit. Use `GitHub` and `AI` sections. GitHub device authentication prints its one-time code, verification URL, and browser-launch prompt to stderr. Browser launch is best-effort; on failure, print one concise manual-open warning and continue waiting.

While waiting for GitHub authorization, stderr uses one transient `Waiting for GitHub authorization: <duration> remaining` line updated once per second. Clear or finish the waiting line before subsequent success, warning, or error output.

When the keyring is unavailable, skip GitHub OAuth and AI API-key prompts, continue collecting non-secret configuration, and explain that secure credential storage is unavailable, secret setup was skipped, and environment variables must be used instead. Do not expose backend keyring errors. End with `OK Setup complete` and a short non-secret recap with `env`, `keyring`, or `missing` sources.

### `config`

`config show` uses non-secret key/value sections. Secret values are never printed. `config set` and `config unset` use standard `OK Updated <key>` and `OK Unset <key>` lines. Errors should say whether a key is unknown, a value is missing, or plaintext secrets are not accepted.

### `rename-repo`

Require prompting availability before doing work. When secure credential storage is unavailable, hide backend errors and direct users to `GIT_WRANGLER_GITHUB_TOKEN`. Keep sequential interaction. Show auth source once. Use a repo header and concise prompts for each repository. Print active interaction skips/failures inline. End with a summary.

### `root`, `help`, `completion`, and `version`

Keep the root landing banner. Keep Cobra-generated `help` and `completion`. `version` stays compact; `version --json` remains unchanged.
