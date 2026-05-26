# Go Rewrite Contract

This document defines the Bash behavior that the Go implementation must preserve. The Bash command files remain in `libexec/` as reference documentation and help metadata, but public command execution is implemented in Go.

## Replacement Rule

Go command behavior must continue to match the relevant `scripts/test` contract coverage and golden fixtures. A full replacement must pass `scripts/check`, `scripts/test`, and the command-specific golden outputs before Bash reference behavior is changed.

The root `git-wrangler` launcher builds and executes the Go CLI. It must not dispatch public commands to the Bash reference scripts.

## Command Dispatch

The root `git-wrangler` dispatcher accepts `git-wrangler <subcommand> [options]`. If no subcommand is provided, it dispatches to `help`.

Subcommands are resolved by the Go command registry. Subcommand names must not contain `/` or `\`. Unknown subcommands fail with a message that includes `Unknown subcommand` and exit nonzero.

The help menu is discovered dynamically from subcommand header metadata in `libexec/git-wrangler-*`. The Go implementation must preserve the public command names, usage text, categories, descriptions, and detailed help rendering unless a deliberate public behavior change updates the golden fixtures and docs.

## Repository Discovery

Repository-oriented commands discover Git repositories by finding `.git` directories in the current directory and immediate subdirectories only. This is equivalent to:

```bash
find . -maxdepth 2 -type d -name '.git'
```

Do not search arbitrarily deep for repository roots unless the public contract changes. Mutating commands must process repositories sequentially.

## Display Names

User-facing repository names use the repository directory basename. If the repository is the current working directory (`.`), the display name is the current directory basename.

Paths containing spaces, leading or trailing whitespace, backslashes, percent signs, and shell metacharacters must remain intact. Output formatting must never treat path, branch, commit message, or user-provided data as a `printf` format string.

## Streams

Fatal errors, prerequisite failures, and destructive warnings go to stderr. Normal command results and skip/status messages go to stdout unless the current Bash command already writes a specific warning to stderr.

Command output from `git`, `gh`, `git-filter-repo`, Python, and API calls should be captured and printed only on failure unless the command intentionally streams that output as part of its UI.

## Terminal Presentation

Terminal styling is controlled by the shared UI behavior:

- `NO_COLOR` disables color and styling.
- `CLICOLOR=0` disables color and styling.
- `TERM=dumb` disables color and Unicode symbols.
- `CLICOLOR_FORCE` forces color unless one of the disabling settings is present.
- Non-TTY output is plain by default.

Golden fixtures are captured with `NO_COLOR=1 TERM=dumb`. Go output must match those plain fixtures before replacing equivalent Bash behavior.

## Prompts

Interactive confirmation prompts use `[y/N]` semantics. Only `y` or `Y` confirms in Bash confirmation helpers unless a command has an explicitly different prompt.

Commands that need interactive setup or destructive confirmation must fail, skip, or stop without mutation when confirmation is absent. Noninteractive `--confirm` flags are the only current way to bypass destructive prompts.

## Destructive Safeguards

History rewriting and destructive local operations must keep the current safeguards:

- `remove-secrets` requires `--confirm`.
- `reset` prompts unless `--confirm` is provided.
- `rewrite-dates` prompts unless `--confirm` is provided.
- `push --force` maps to `--force-with-lease`.
- `push --force-unsafe` requires confirmation.
- `rewrite-authors`, `rewrite-commits`, `rewrite-commits-ai`, `rewrite-dates`, and `remove-secrets` must support both `git-filter-repo` and `git filter-repo`, preferring the standalone executable.
- Commands that rewrite history must restore the `origin` remote when `git-filter-repo` removes it and the original remote was present.

## Parser Contract

All unknown options fail nonzero. Every `--flag <value>` option must reject missing values and the next token starting with `--` before shifting parser arguments.

Required argument combinations fail before prerequisite checks where the Bash command currently does so. Parser and required-argument failures are part of the contract suite.

## Fake Dependency Tests

`scripts/test` uses fake `gh`, fake `git-filter-repo`, fake install layouts, and temporary Git repositories to exercise behavior without touching user repositories or the real checkout. A Go rewrite must remain testable with the same strategy:

- no network access is required for command contract tests;
- no test may update, reset, uninstall, or delete the real checkout;
- temp repositories must be created under the test temp directory;
- fake dependency logs are valid assertions for external command arguments.

## Golden Fixtures

Stable plain output fixtures live under `tests/fixtures/golden/`. The current required fixtures cover:

- top-level `help`;
- detailed `help push`;
- detailed `help remove-secrets`;
- `status` for clean, dirty, and no-remote repositories;
- representative parser and dispatcher errors.

Temporary paths and commit hashes are normalized by the test harness before fixture comparison. Any intentional user-visible output change must update fixtures in the same change and explain the behavior change in review.
