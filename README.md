# Git Wrangler

Git Wrangler is a native CLI for managing many Git repositories at once.

If you keep related projects in one folder, Git Wrangler lets you check status,
pull, push, review, commit, clean up ignores, rename branches, and run careful
history rewrites without jumping from repo to repo by hand.

## Navigation

- [Install](#install)
- [Quick start](#quick-start)
- [Useful workflows](#useful-workflows)
- [Commands](#commands)
- [Safety](#safety)
- [Requirements](#requirements)
- [Architecture and technology](#architecture-and-technology)
- [Development](#development)

## Install

macOS and Linux:

```bash
brew install kaufmann-dev/tap/git-wrangler
```

Upgrade with:

```bash
brew upgrade git-wrangler
```

Homebrew installs shell completions automatically.

> [!TIP]
> **Bash autocompletion on Linux:** Install and enable `bash-completion` first.
> If you use Homebrew, run `brew install bash-completion`.

Windows:

```powershell
scoop bucket add kaufmann-dev https://github.com/kaufmann-dev/scoop-bucket.git
scoop install kaufmann-dev/git-wrangler
```

Scoop and Homebrew install the CLI tool dependencies: `git`, `gh`, and
`git-filter-repo`. Manual binary installs do not, so install the tools listed in
[Requirements](#requirements) yourself as needed.

## Quick start

Clone a set of GitHub repositories into one folder:

```bash
git-wrangler clone --user myusername --visibility public --into ./repos
cd repos
```

See the state of every repository:

```bash
git-wrangler status
```

Pull everything:

```bash
git-wrangler pull --rebase
```

Review unpushed work before you publish it:

```bash
git-wrangler review
```

Run `git-wrangler help` for the full command list, or
`git-wrangler help <command>` for one command.

Check your local runtime dependencies:

```bash
git-wrangler doctor
```

Set up GitHub and AI credentials when you need private GitHub workflows or AI-assisted commit rewrites:

```bash
git-wrangler init
```

## Useful workflows

### Keep a workspace fresh

```bash
git-wrangler status
git-wrangler pull --rebase
git-wrangler status
```

Use this when you have a folder full of projects and want to know what changed,
what is dirty, and what is behind origin.

### Publish local work across repos

```bash
git-wrangler review
git-wrangler push
```

`review` shows unpushed changes first, so you can inspect what will be published
before running `push`.

### Apply the same branch rename everywhere

```bash
git-wrangler rename-branch --oldbranch master --newbranch main
```

This is useful for cleaning up older repositories that should share the same
default branch name.

### Fix common generated files

```bash
git-wrangler fix-gitignore
git-wrangler untrack
```

Use this when build artifacts, dependency folders, or generated files have crept
into tracking and should be ignored.

### Clean up history when you need to

```bash
git-wrangler remove-secrets
git-wrangler rewrite-authors --name "Your Name" --email "you@example.com"
git-wrangler rewrite-commits
```

History rewrite commands are intentionally guarded. They require confirmation,
print warnings, and use `git-filter-repo` for the actual rewrite work.

## Commands

Remote operations:

| Command       | What it does                                                   |
| ------------- | -------------------------------------------------------------- |
| `clone`       | Clone multiple GitHub repositories for a user or organization. |
| `pull`        | Pull the latest changes for every discovered repository.       |
| `push`        | Push local commits to origin.                                  |
| `rename-repo` | Rename GitHub repositories with `gh`.                          |

Local operations:

| Command         | What it does                                                |
| --------------- | ----------------------------------------------------------- |
| `commit`        | Stage all changes and create a commit in every repository.  |
| `fix-gitignore` | Add missing common generated-file patterns to `.gitignore`. |
| `license`       | Add or replace MIT license files.                           |
| `rename-branch` | Rename a branch across repositories.                        |
| `reset`         | Reset current branches to their origin counterparts.        |
| `review`        | Review unpushed changes across repositories.                |
| `untrack`       | Stop tracking files already covered by `.gitignore`.        |

History rewriting:

| Command              | What it does                                                              |
| -------------------- | ------------------------------------------------------------------------- |
| `remove-secrets`     | Purge sensitive files from Git history.                                   |
| `rewrite-authors`    | Rewrite author and committer identity.                                    |
| `rewrite-commits`    | Rewrite commit messages to Conventional Commits.                          |
| `rewrite-commits-ai` | Generate Conventional Commit messages with an OpenAI-compatible endpoint. |
| `rewrite-dates`      | Redistribute commit timestamps.                                           |

Utility:

| Command      | What it does                           |
| ------------ | -------------------------------------- |
| `config`     | Show and edit Git Wrangler setup.      |
| `completion` | Generate shell completion scripts.     |
| `doctor`     | Check runtime dependencies.            |
| `help`       | Show help.                             |
| `info`       | Show detailed repository information.  |
| `init`       | Set up GitHub and AI credentials.      |
| `status`     | Show clean, dirty, and tracking state. |
| `version`    | Print version metadata.                |

## Safety

Git Wrangler is designed for bulk work, so it tries to make risky operations
obvious.

- History rewrite commands require confirmation before mutation.
- `--yes` is the standard flag for noninteractive confirmation.
- Destructive operations print warnings to stderr.
- Bulk commands continue through discovered repositories, then exit nonzero if
  any repository operation failed.
- No-op skips are treated as successful.
- `rewrite-commits-ai` does not send old commit messages as model context and
  redacts sensitive file content before API calls.

Repository discovery supports regular `.git` directories and linked worktree
`.git` files with valid `gitdir:` pointers.

## Requirements

| Tool                                   | Needed for                                                      |
| -------------------------------------- | --------------------------------------------------------------- |
| `git`                                  | Normal repository operations.                                   |
| `gh`                                   | GitHub repository operations such as `clone` and `rename-repo`. |
| `git-filter-repo`                      | History rewrite commands.                                       |
| OpenAI-compatible chat completions API | `rewrite-commits-ai`.                                           |

Run this before private or all-repository GitHub workflows:

```bash
git-wrangler init
```

History rewrite commands that require `git-filter-repo`:

- `remove-secrets`
- `rewrite-authors`
- `rewrite-commits`
- `rewrite-commits-ai`
- `rewrite-dates`

`rewrite-commits-ai` also needs an OpenAI-compatible chat completions endpoint,
a model name, and an API key configured with `git-wrangler init` or
`git-wrangler config`.

Run `git-wrangler doctor` to check local runtime dependencies. Missing `git`
is reported as an error because most commands need it. Missing `gh` or
`git-filter-repo` is reported as a warning because those tools are only needed
for specific workflows.

## Architecture and technology

Git Wrangler is a compiled Go CLI built with Cobra.

The command entrypoint is small: `cmd/git-wrangler/main.go` calls the CLI
package, and the internal packages handle repository discovery, Git commands,
GitHub CLI commands, terminal output, AI commit rewriting, and version metadata.

At a high level:

| Technology        | Role                                                                         |
| ----------------- | ---------------------------------------------------------------------------- |
| Go                | Native binary.                                                               |
| Cobra             | Commands, flags, help, and shell completion generation.                      |
| `git`             | Repository operations.                                                       |
| `go-keyring`      | GitHub and AI credential storage.                                            |
| `gh`              | GitHub repository transport for clone and rename workflows.                  |
| `git-filter-repo` | History rewrites.                                                            |
| GoReleaser        | Release archives, checksums, completions, and package-manager updates.       |

The detailed contributor-facing architecture lives in [AGENTS.md](AGENTS.md).

## Development

| Task                            | Command                                 |
| ------------------------------- | --------------------------------------- |
| Run the focused test suite      | `go test ./...`                         |
| Run the full local check script | `scripts/check`                         |
| Run a release dry run           | `goreleaser release --snapshot --clean` |
