# Command Reference

This is an internal implementation reference for maintainers and AI agents. It does not replace generated Cobra help. Keep it focused on behavior that command help does not fully explain: targeting, mutation scope, confirmations, dependencies, concurrency, JSON, and output shape.

## Repository Targeting

Most repository commands discover Git worktrees below the current directory when `--repo` is omitted. `--repo PATH` means exact single-repository targeting: operate only on `PATH`, do not recurse below it, and fail if `PATH` is not a normal or linked-worktree Git repository. Bare repository directories are rejected.

Users who previously used `info --repo DIR`, `license --repo DIR`, or `rewrite-authors --repo DIR` to discover nested repositories should omit `--repo` and run from the common parent directory instead.

Commands with `--repo`: `pull`, `fetch`, `push`, `commit`, `fix-gitignore`, `license`, `rename-branch`, `reset`, `review`, `untrack`, `remove-secrets`, `rewrite-authors`, `rewrite-commits`, `rewrite-dates`, `info`, `status`, and `rename-repo`.

Commands without `--repo`: `clone`, `init`, `config`, `doctor`, `version`, `completion`, and `help`.

## Command Matrix

| Command           | Group   | Targeting                                    | Mutates                                                                                       | Confirmation                                                                                            | Required runtime tools                        | Auth/config usage                                         |
| ----------------- | ------- | -------------------------------------------- | --------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------- | --------------------------------------------- | --------------------------------------------------------- |
| `clone`           | Remote  | GitHub user/org from `--user`                | Filesystem clone destination                                                                  | None                                                                                                    | `git`, `gh`                                   | Private/all visibility uses Git Wrangler GitHub auth      |
| `pull`            | Remote  | Discovered repos or exact `--repo`           | Working trees/refs through `git pull`                                                         | None                                                                                                    | `git`                                         | None                                                      |
| `fetch`           | Remote  | Discovered repos or exact `--repo`           | Remote-tracking refs through `git fetch origin`                                               | None                                                                                                    | `git`                                         | None                                                      |
| `push`            | Remote  | Discovered repos or exact `--repo`           | Remote refs                                                                                   | `--force-unsafe` prompts once for all targets unless `--yes`                                            | `git`                                         | None                                                      |
| `rename-repo`     | Remote  | Discovered repos or exact `--repo`           | GitHub repository name and optionally description                                             | Prompts for new values; `gh repo rename` receives `--yes` after a name is entered                       | `git`, `gh`                                   | Requires Git Wrangler GitHub auth                         |
| `commit`          | Local   | Discovered repos or exact `--repo`           | Index and new commits; sends redacted staged context to AI endpoint                           | Data-send prompt unless `--yes`                                                                         | `git`                                         | Requires AI base URL, model, and API key                  |
| `fix-gitignore`   | Local   | Discovered repos or exact `--repo`           | `.gitignore` and a new commit when candidates are added                                       | Prompts once for all candidates unless `--yes`                                                          | `git`                                         | None                                                      |
| `license`         | Local   | Discovered repos or exact `--repo`           | Writes `LICENSE` files                                                                        | `--overwrite` prompts once before replacing existing files unless `--yes`                               | `git`                                         | None                                                      |
| `rename-branch`   | Local   | Discovered repos or exact `--repo`           | Local branch names                                                                            | Prompts for required names only when interactive; no `--yes`                                            | `git`                                         | None                                                      |
| `reset`           | Local   | Discovered repos or exact `--repo`           | Fetches then hard-resets current branch to `origin/<branch>`                                  | Prompts once for all candidates unless `--yes`                                                          | `git`                                         | None                                                      |
| `review`          | Local   | Discovered repos or exact `--repo`           | No                                                                                            | None                                                                                                    | `git`                                         | None                                                      |
| `untrack`         | Local   | Discovered repos or exact `--repo`           | Index and a new commit for ignored tracked files                                              | Prompts once for all candidates unless `--yes`                                                          | `git`                                         | None                                                      |
| `remove-secrets`  | History | Discovered repos or exact `--repo`           | History rewrite through `git-filter-repo`                                                     | Prompts once for all candidates unless `--yes`; declined rewrites are skips                             | `git`, `git-filter-repo` or `git filter-repo` | None                                                      |
| `rewrite-authors` | History | Discovered repos or exact `--repo`           | History rewrite through `git-filter-repo`                                                     | Prompts once for all targets unless `--yes`; declined rewrites are skips                                | `git`, `git-filter-repo` or `git filter-repo` | None                                                      |
| `rewrite-commits` | History | Discovered repos or exact `--repo`           | Sends redacted commit context to AI endpoint, then rewrites history through `git-filter-repo` | AI data-send prompt during generation, then one apply prompt for all listed repositories unless `--yes` | `git`, `git-filter-repo` or `git filter-repo` | Requires AI base URL, model, and API key                  |
| `rewrite-dates`   | History | Discovered repos or exact `--repo`           | History rewrite through `git-filter-repo`                                                     | Prompts once for all candidates unless `--yes`; declined rewrites are skips                             | `git`, `git-filter-repo` or `git filter-repo` | None                                                      |
| `info`            | Utility | Discovered repos or exact `--repo`           | No                                                                                            | None                                                                                                    | `git`                                         | None                                                      |
| `doctor`          | Utility | Current machine/config only                  | No                                                                                            | None                                                                                                    | Checks `git`, `gh`, and `git-filter-repo`     | Reads config and credential sources                       |
| `init`            | Utility | Current user config and credentials          | Config file and optional keyring secrets                                                      | Interactive setup prompts                                                                               | None directly                                 | Writes GitHub/AI config and optional credentials          |
| `config`          | Utility | Current user config and credentials          | `set`/`unset` mutate config or keyring; `show` does not                                       | Secret setters prompt for hidden input                                                                  | None directly                                 | Owns non-secret config display/editing and secret storage |
| `status`          | Utility | Discovered repos or exact `--repo`           | No                                                                                            | None                                                                                                    | `git`                                         | None                                                      |
| `version`         | Utility | Binary metadata only                         | No                                                                                            | None                                                                                                    | None                                          | None                                                      |
| `completion`      | Utility | Shell selected by generated Cobra subcommand | No                                                                                            | None                                                                                                    | None                                          | Generated by Cobra                                        |
| `help`            | Utility | Generated command/help topic                 | No                                                                                            | None                                                                                                    | None                                          | Generated by Cobra                                        |

## Shared Flags

| Flag or argument                              | Commands                                                                                                                                                                     | Behavior                                                                                                                                                                                            |
| --------------------------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `--repo`                                      | `pull`, `fetch`, `push`, `commit`, `fix-gitignore`, `license`, `rename-branch`, `reset`, `review`, `untrack`, `remove-secrets`, `rewrite-*`, `info`, `status`, `rename-repo` | Targets exactly one working-tree repository. Does not discover nested repositories.                                                                                                                 |
| `--yes`                                       | `commit`, `fix-gitignore`, `license`, `push`, `reset`, `untrack`, `remove-secrets`, `rewrite-authors`, `rewrite-commits`, `rewrite-dates`                                    | Skips confirmation prompts only. Multi-repository commands ask at most one confirmation for the candidate set. It does not fill required values such as names, branches, config values, or secrets. |
| `--json`                                      | `status`, `info`, `review`, `doctor`, `config show`, `version`                                                                                                               | Emits exactly one JSON document on stdout. Suppresses progress, colors, prompts, and human summaries.                                                                                               |
| `--force`                                     | `pull`, `push`, `rewrite-authors`                                                                                                                                            | Has command-specific meaning: `git pull --force`, `git push --force-with-lease`, or `git-filter-repo --force`.                                                                                      |
| `--force-unsafe`                              | `push`                                                                                                                                                                       | Uses raw `git push --force` after one aggregate confirmation unless `--yes` is also set. Cannot be combined with `--force`.                                                                         |
| `--prune`                                     | `fetch`                                                                                                                                                                      | Runs `git fetch --prune origin` instead of `git fetch origin`.                                                                                                                                      |
| `--body`                                      | `commit`, `rewrite-commits`                                                                                                                                                  | Requests generated commit message bodies in addition to subjects.                                                                                                                                   |
| `--rpm`                                       | `commit`, `rewrite-commits`                                                                                                                                                  | Caps AI API request starts per minute. Must be positive.                                                                                                                                            |
| `--timeout`                                   | `commit`, `rewrite-commits`                                                                                                                                                  | AI API timeout in seconds. Must be positive.                                                                                                                                                        |
| `--max-chars-per-commit`                      | `commit`, `rewrite-commits`                                                                                                                                                  | Redacted context budget per commit. Must be positive.                                                                                                                                               |
| `--batch-size`                                | `rewrite-commits`                                                                                                                                                            | AI commits per generation request. Must be from 1 through 50.                                                                                                                                       |
| `--skip-conventional`                         | `rewrite-commits`                                                                                                                                                            | Skips commits that already look like Conventional Commits before AI generation.                                                                                                                     |
| `--start-date`, `--end-date`                  | `rewrite-dates`                                                                                                                                                              | Optional `YYYY-MM-DD` bounds. Missing bounds default to the repository's first/latest commit timestamps.                                                                                            |
| `--name`                                      | `license`, `rewrite-authors`                                                                                                                                                 | Required holder name or rewritten author name. Interactive runs prompt when omitted; noninteractive runs fail, and `--yes` does not supply it.                                                      |
| `--email`                                     | `rewrite-authors`                                                                                                                                                            | Required rewritten author email. Interactive runs prompt when omitted; noninteractive or `--yes` runs fail when omitted.                                                                            |
| `--oldbranch`, `--newbranch`                  | `rename-branch`                                                                                                                                                              | Required branch names. Interactive runs prompt when omitted; noninteractive runs fail when omitted.                                                                                                 |
| `--overwrite`                                 | `license`                                                                                                                                                                    | Replaces existing `LICENSE` files after one aggregate confirmation. Creating missing `LICENSE` files does not prompt.                                                                               |
| `--description`                               | `rename-repo`                                                                                                                                                                | Adds description prompts and `gh repo edit --description` calls.                                                                                                                                    |
| `--visibility`, `--user`, `--limit`, `--into` | `clone`                                                                                                                                                                      | Control GitHub repository listing and clone destination. `--user` is required, `--limit` must be at least 1, and private/all cloning requires Git Wrangler GitHub auth.                             |

## Behavior Matrix

| Behavior                                         | Commands                                                                                                                                                                             |
| ------------------------------------------------ | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| Read-only repository scans with up to 32 workers | `status`, `review`, `info`, scan phases of `fix-gitignore`, `remove-secrets`, `rewrite-commits`, `rewrite-dates`                                                                     |
| Independent Git mutations with up to 4 workers   | `fetch`, `pull`, normal `push`, commit preparation/creation, `rename-branch`, apply phases of `remove-secrets`, `rewrite-authors`, `rewrite-commits`, `rewrite-dates`                |
| Sequential per-repo flow                         | `clone`, `rename-repo`, `license`, `push --force-unsafe`; scan/preview/apply flows still ask at most one aggregate confirmation                                                      |
| History rewrite and origin restore               | `remove-secrets`, `rewrite-authors`, `rewrite-commits`, `rewrite-dates`                                                                                                              |
| Returns nonzero on per-repo failure              | Bulk commands return nonzero for validation errors, missing dependencies, failed repo operations, partial failures, or failed cleanup. User-declined no-ops remain successful skips. |
| Progress reported to stderr                      | Bulk scan/mutation phases that use `newProgress`; ordered summaries and per-repo result blocks remain on stdout/stderr after collection                                              |
| JSON output                                      | Read-only/introspection JSON commands write one document to stdout and keep stderr empty except Cobra parse errors or unavoidable process-level failures                             |
| Uses Git Wrangler GitHub credentials             | `clone`, `rename-repo`, `doctor`, `config`, `init`                                                                                                                                   |
| Uses Git Wrangler AI settings                    | `commit`, `rewrite-commits`, `doctor`, `config`, `init`                                                                                                                              |

## JSON Shape

JSON mode is command-local and intentionally limited. It uses this general shape:

```json
{
  "ok": true,
  "summary": {},
  "repositories": []
}
```

Fatal command errors set `ok` to `false` and include `error.message`. Per-repository failures are represented inside `repositories[]` and make the command exit 1. `config show --json` reports credential sources and booleans only; it must not expose stored secrets.

## Per-Command Notes

### `clone`

`clone` lists repositories through `gh repo list` and clones each repository with `gh repo clone`. It performs an initial one-item listing to distinguish empty result sets from listing failures before creating the destination directory. Existing destination directories are skipped successfully.

### `commit`

`commit` prepares staged context in a temporary index so scanning does not stage changes before the user approves the data-send prompt. If context collection fails for any repository, it stops before API calls and before creating commits. Declining the data-send prompt is a successful no-op.

### `fetch`

`fetch` runs `git fetch origin` for every target repository. `fetch --prune` runs `git fetch --prune origin`. Missing or invalid `origin` remotes are per-repository failures and count in the summary.

### `rename-repo`

`rename-repo` is intentionally interactive and sequential. It skips repositories that `gh repo view` cannot identify as GitHub repositories. There is no `--yes`; entered names are passed to `gh repo rename` with `--yes`.

### `license`

`license` creates missing `LICENSE` files without confirmation. `license --overwrite` prompts once before replacing all existing files in the candidate set; `license --overwrite --yes` skips only that overwrite confirmation.

### `reset`

`reset` fetches `origin <current-branch>`, skips detached HEAD and missing remote counterparts, reports ahead/behind counts, warns when the working tree is dirty, and then runs `git reset --hard origin/<branch>` after one aggregate confirmation.

### `remove-secrets`

`remove-secrets` scans history for a fixed set of sensitive filename/path patterns, prints matched files, and only rewrites repositories with matches. It always passes `--partial --force` to `git-filter-repo`.

### `rewrite-commits`

`rewrite-commits` validates AI settings before scanning repositories. Generation may prompt before sending data; applying generated messages is a separate all-repository confirmation. Old commit messages are not sent as model context. Declining either confirmation is a successful no-op before mutation.

### `rewrite-dates`

`rewrite-dates` requires at least two commits per target repository. It prints the old-to-new timestamp summary for each candidate before one aggregate confirmation, preserves the dominant timezone offset when possible, and distributes rewritten timestamps between the selected or inferred bounds.

### `config`

`config set` accepts plaintext values for non-secret keys only. Secret keys (`github.auth`, `ai.api-key`) must be entered through the prompt and are stored through the credential store. `config unset` only removes stored secret values.
