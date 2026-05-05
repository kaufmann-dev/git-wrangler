# Agent Guidelines for git-wrangler

This file documents critical bug classes and architectural decisions discovered during audits of the bash scripts in this repository. Any new scripts or modifications to existing scripts MUST adhere to these guidelines to ensure cross-platform compatibility, security, and correctness.

## 0. Repository Architecture
All subcommand scripts live in `libexec/` and follow the naming convention `wrangler-<subcommand>` (no `.sh` extension). The root `wrangler` dispatcher routes `wrangler <subcommand>` invocations to `libexec/wrangler-<subcommand>` via `exec bash`.

When adding a new subcommand, create `libexec/wrangler-<subcommand>` and include the standard header block (see §7). The help system discovers subcommands dynamically — no registration step is needed.

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
2. **Header Block:** A comment block delimited by `# ====` lines. The first three lines are the machine-readable fields used by the top-level help menu. Everything after them is the human-readable documentation rendered by `wrangler help <subcommand>`.
   ```bash
   # ====
   # Usage: wrangler <subcommand> [--arg1 <value>] [--flag]
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
   #     wrangler <subcommand> --flag1 value
   # ====
   ```
   - **Usage** must use the `wrangler <subcommand>` syntax (not `./script.sh`).
   - **Description** must be a single line — it is used verbatim in the top-level help menu.
   - **Category** must be one of: `Remote Operations`, `Local Operations`, `History Rewriting`, or `Utility`.
   - **Description paragraph** (below Category) should be one or more sentences explaining the command in plain language.
   - **Options** section lists every accepted flag with its required/optional status and a short description. Omit this section for commands that take no arguments.
   - **Example** / **Examples** section provides one or more ready-to-run usage examples.
3. **Variables & Argument Parsing:** Default variable assignments followed by a `while [[ $# -gt 0 ]]; do ... case ...` loop for argument parsing. Unknown arguments should throw a red error and exit 1.
4. **Prerequisite Checks:** Use `command -v <cmd> &> /dev/null` to verify required tools (`git`, `gh`, `git-filter-repo`) are installed before executing logic.
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
Scripts must use colored `printf` statements to provide clear visual feedback to the user:
- **Red (`\e[31m`)**: Errors, failures, and missing prerequisites. **Must be redirected to stderr using `>&2`.**
- **Green (`\e[32m`)**: Success and completed operations.
- **Yellow (`\e[33m`)**: Warnings, skipped operations, and "no changes needed" states.
- **Reset (`\e[0m`)**: Always reset the color at the end of the string.
Example: `printf "\e[31mError: Commit failed for %s\e[0m\n" "$repo_name_display" >&2``

## 11. Command Output Capture and Error Handling
When running a command that might fail, capture its output (both stdout and stderr) and only display it if an error occurs. Never let command output bleed directly into the terminal unless intended.
```bash
if command_output=$(git commit -m "Message" 2>&1); then
    printf "\e[32mCommit successful for %s\e[0m\n" "$repo_name_display"
else
    printf "\e[31mError: Commit failed for %s:\n%s\e[0m\n\n" "$repo_name_display" "$command_output" >&2
fi
```

## 12. Error Streams and Pipe Safety
When a script encounters a fatal error or a prerequisite failure, the output MUST be redirected to standard error (`>&2`). This ensures that if a user pipes a `wrangler` command (e.g., `wrangler status | grep "dirty"`), the error message still correctly appears on their screen instead of being swallowed by the pipe.
