# Agent Guidelines for git-wrangler

This file documents critical bug classes and architectural decisions discovered during audits of the bash scripts in this repository. Any new scripts or modifications to existing scripts MUST adhere to these guidelines to ensure cross-platform compatibility, security, and correctness.

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
