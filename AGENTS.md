# Agent Guidelines for git-wrangler

This file documents critical bug classes and architectural decisions discovered during audits of the bash scripts in this repository. Any new scripts or modifications to existing scripts MUST adhere to these guidelines to ensure cross-platform compatibility, security, and correctness.

## 0. Repository Architecture
All subcommand scripts live in `libexec/` and follow the naming convention `git-wrangler-<subcommand>` (no `.sh` extension). The root `git-wrangler` dispatcher routes `git-wrangler <subcommand>` invocations to `libexec/git-wrangler-<subcommand>` via `exec bash`.

When adding a new subcommand, create `libexec/git-wrangler-<subcommand>` and include the standard header block (see §7). The help system discovers subcommands dynamically — no registration step is needed.

## 1. `while read` Loop Safety (Whitespace and Backslashes)
**DO NOT** use a bare `read` command (e.g., `while read var; do`). 
Without `-r`, Bash interprets backslashes as escape characters, which will unexpectedly mangle Windows file paths. Without `IFS=`, Bash strips leading and trailing whitespace from the input.

* **Bad:** `while read repo; do`
* **Good:** `while IFS= read -r repo; do`

## 2. Avoid Pipe-into-while Subshells
**DO NOT** pipe data directly into a `while` loop. 
Piping into a loop (e.g., `echo "$list" | while ...`) runs the loop body in a subshell, meaning any state/variables modified inside the loop are destroyed when the loop terminates. Furthermore, passing an unquoted variable to `echo` can accidentally trigger `echo` flags (like `-n` or `-e`) if a file path happens to match.

* **Bad:**
  ```bash
  echo "$git_repositories" | while read repo; do
      # ...
  done
  ```
* **Good:** Use a Here-String instead:
  ```bash
  while IFS= read -r repo; do
      # ...
  done <<< "$git_repositories"
  ```

## 3. Prevent `printf` Format String Injection
**DO NOT** interpolate variables directly into the format string of `printf`. 
If a variable contains a `%` character (e.g., in a branch name, commit message, or directory name), `printf` will attempt to parse it as a format specifier. If no arguments are provided to satisfy the specifier, `printf` will output garbage data or cause the script to fail.

* **Bad:** `printf "\e[31mError: $error_message\e[0m\n"`
* **Good:** Pass external data as separate arguments using `%s`:
  ```bash
  printf "\e[31mError: %s\e[0m\n" "$error_message"
  ```

## 4. `git check-ignore` Exit Codes
Remember that `git check-ignore` exits with `0` (Success) when a path **IS** ignored, and `1` (Failure) when it is **NOT** ignored. Be careful not to invert this logic when checking if a file needs to be added to `.gitignore`.

* **Correct Logic:**
  ```bash
  if git check-ignore -q "$path" 2>/dev/null; then
      # The path IS covered by .gitignore
  fi
  ```

## 5. Correct Data Sources for File Existence Checks
When checking if a file or folder physically exists on disk (e.g., before adding it to a `.gitignore`), **DO NOT** use `git ls-files --cached`. This only queries the Git index, making untracked files invisible. Instead, query the actual filesystem using `find . -type d`, `find . -type f`, or `[ -e "$path" ]`.
*(Note: `git ls-files --cached --ignored` is perfectly fine if the explicit goal is to identify tracked files that shouldn't be).*

## 6. `find` Depth Limitations
When using `find` to discover files or directories (e.g., `dist/` or `node_modules/`), avoid hardcoded `-maxdepth` limits unless you are strictly trying to discover top-level directories (like `.git/` repository roots). An artificial depth cap will cause `find` to silently miss matched files nested deep within subdirectories.

## 7. Standard Script Structure & Boilerplate
All subcommand scripts in `libexec/` follow a standardized structure to maintain consistency:
1. **Shebang:** `#!/bin/bash`
2. **Header Block:** A comment block delimited by `# ====` lines. The first three lines are the machine-readable fields used by the top-level help menu. Everything after them is the human-readable documentation rendered by `git-wrangler help <subcommand>`.
   ```bash
   # ====
   # Usage: git-wrangler <subcommand> [--arg1 <value>] [--flag]
   # Description: Brief one-line explanation of the subcommand's purpose.
   # Category: Remote Operations | Local Operations | History Rewriting | Utility
   #
   # One or more sentences explaining what the subcommand does, any important
   # prerequisites, and its default behaviour.
   #
   # Options:
   #   --flag1 <value>  (required) What this flag does.
   #   --flag2          (optional) What this flag does.
   #
   # Example:
   #     git-wrangler <subcommand> --flag1 value
   # ====
   ```
   - **Usage** must use the `git-wrangler <subcommand>` syntax (not `./script.sh`).
   - **Description** must be a single line — it is used verbatim in the top-level help menu.
   - **Category** must be one of: `Remote Operations`, `Local Operations`, `History Rewriting`, or `Utility`.
   - **Description paragraph** (below Category) should be one or more sentences explaining the command in plain language.
   - **Options** section lists every accepted flag with its required/optional status and a short description. Omit this section for commands that take no arguments.
   - **Example** / **Examples** section provides one or more ready-to-run usage examples.
3. **Variables & Argument Parsing:** Default variable assignments followed by a `while [[ $# -gt 0 ]]; do ... case ...` loop for argument parsing. Unknown arguments should throw a red error and exit 1.
4. **Prerequisite Checks:** Use `command -v <cmd> &> /dev/null` to verify required tools (`git`, `gh`, `python3`) are installed before executing logic. For `git-filter-repo`, support both `git-filter-repo` and `git filter-repo`, preferring the standalone executable when both are available.
5. **Target Discovery:** Use `find` to locate target `.git` directories and store them in a variable. Exit gracefully with a yellow message if none are found.
6. **Execution Loop:** Iterate over the repositories using the standardized `while IFS= read -r` and `<<< "$git_repositories"` here-string.

## 8. Subshell Isolation for Repository Iteration
When iterating over repositories, the body of the loop MUST be wrapped in a subshell `( ... )`. This safely isolates the `cd "$repo_dir"` command, ensuring that directory changes and local variables do not leak into the rest of the script or affect subsequent iterations.
```bash
while IFS= read -r git_dir; do
    (
        repo_dir=$(dirname "$git_dir")
        cd "$repo_dir" || exit
        
        # ... core script logic ...
    )
done <<< "$git_repositories"
```

## 9. Standardized Repository Naming
To display the repository name in log messages, correctly handle the case where the repository is the current working directory (`.`):
```bash
if [ "$repo_dir" = "." ]; then
    repo_name_display="${PWD##*/}"
else
    repo_name_display=$(basename "$repo_dir")
fi
```
Always use `$repo_name_display` in user-facing output.

## 10. Output Formatting and Colors
Subcommands MUST source the shared UI helper immediately after the header block. Commands that need shared repo discovery, display-name handling, option value validation, prerequisite checks, filter-repo detection, confirmations, or CPU-count detection MUST source `git-wrangler-core` immediately after `git-wrangler-ui`:
```bash
UI_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"
# shellcheck source=libexec/git-wrangler-ui
source "$UI_DIR/git-wrangler-ui"
# shellcheck source=libexec/git-wrangler-core
source "$UI_DIR/git-wrangler-core"
```

Use the helper vocabulary instead of hardcoded ANSI escapes:
- `gw_ok`, `gw_warn`, `gw_error`, `gw_info`, `gw_step`, `gw_skip`
- `gw_header`, `gw_repo`, `gw_prompt`
- `$GW_RED`, `$GW_GREEN`, `$GW_YELLOW`, `$GW_CYAN`, `$GW_MUTED`, `$GW_BOLD`, `$GW_RESET` only when formatting inline tabular output

Colors and styling MUST respect `NO_COLOR`, `CLICOLOR=0`, `CLICOLOR_FORCE`, `TERM=dumb`, and non-TTY output as implemented by `libexec/git-wrangler-ui`. Fatal errors and prerequisite failures MUST still be written to stderr.

## 11. Command Output Capture and Error Handling
When running a command that might fail, capture its output (both stdout and stderr) and only display it if an error occurs. Never let command output bleed directly into the terminal unless intended.
```bash
if command_output=$(git commit -m "Message" 2>&1); then
    gw_ok "$repo_name_display" "Commit successful"
else
    gw_error "$repo_name_display" "Commit failed:"
    printf "%s\n\n" "$command_output" >&2
fi
```

## 12. Error Streams and Pipe Safety
When a script encounters a fatal error or a prerequisite failure, the output MUST be redirected to standard error (`>&2`). This ensures that if a user pipes a `git-wrangler` command (e.g., `git-wrangler status | grep "dirty"`), the error message still correctly appears on their screen instead of being swallowed by the pipe.

## 13. Bash Performance Policy
Optimize measured bottlenecks, not every line. Prefer changes that remove complexity while reducing repeated work.

- In repository loops, prefer Bash parameter expansion for simple path pieces instead of `dirname`/`basename` subprocesses.
- Batch parsing when possible: one `awk` or one `git status --porcelain=v2 --branch` parse is preferable to repeated `grep | awk | tr` chains.
- Cache repeated Git output inside each repository loop instead of running the same Git query more than once.
- Keep mutating and destructive commands sequential unless a future explicit flag defines parallel semantics.
- Any parallel read-only command must preserve deterministic output order.
- Benchmark meaningful performance changes with `scripts/bench`; do not add broad `# PERF:` comments for obvious shell idioms.

## 14. Project Checks
Use root-level contributor scripts for verification:

- `scripts/check` runs Bash syntax checks, optional ShellCheck/shfmt checks, and the website build when dependencies are installed.
- `scripts/test` runs integration tests against temporary Git repositories only.
- `scripts/bench` creates temporary multi-repo fixtures for lightweight timing.
