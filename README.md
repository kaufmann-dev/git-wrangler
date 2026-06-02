# Git Wrangler

[![CI](https://github.com/kaufmann-dev/git-wrangler/actions/workflows/ci.yml/badge.svg)](https://github.com/kaufmann-dev/git-wrangler/actions/workflows/ci.yml)
[![Release](https://github.com/kaufmann-dev/git-wrangler/actions/workflows/release.yml/badge.svg)](https://github.com/kaufmann-dev/git-wrangler/actions/workflows/release.yml)
[![License: MIT](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)

```text
  ____ _ _     __        __                       _
 / ___(_) |_   \ \      / / __ __ _ _ __   __ _ | | ___ _ __
| |  _| | __|   \ \ /\ / / '__/ _` | '_ \ / _` || |/ _ \ '__|
| |_| | | |_     \ V  V /| | | (_| | | | | (_| || |  __/ |
 \____|_|\__|     \_/\_/ |_|  \__,_|_| |_|\__, ||_|\___|_|
                                           |___/
```

A native Go CLI for managing many Git repositories from one directory.

Git Wrangler is for workspaces full of related repositories. Run status checks,
pulls, pushes, commits, reviews, branch renames, GitHub repo operations, and
guarded history rewrites without stepping through every project by hand.

Full docs live at [git-wrangler.kaufmann.dev](https://git-wrangler.kaufmann.dev).
Release archives are available from
[GitHub Releases](https://github.com/kaufmann-dev/git-wrangler/releases).

## Navigation

- [Install](#install)
- [Quick Start](#quick-start)
- [Usage](#usage)
- [Commands](#commands)
- [Runtime Dependencies](#runtime-dependencies)
- [Safety](#safety)
- [Development](#development)
- [License](#license)

## Install

macOS and Linux:

```bash
brew install kaufmann-dev/tap/git-wrangler
```

Upgrade:

```bash
brew upgrade git-wrangler
```

Windows:

```powershell
scoop bucket add kaufmann-dev https://github.com/kaufmann-dev/scoop-bucket.git
scoop install kaufmann-dev/git-wrangler
```

Upgrade:

```powershell
scoop update git-wrangler
```

Homebrew and Scoop install the normal CLI dependencies: `git`, `gh`, and
`git-filter-repo`. If you install a release archive manually, install those
tools yourself for the commands that need them.

Homebrew installs bash, zsh, and fish completions automatically. Release
archives include completion scripts under `completions/`, and the CLI can print
completion scripts for bash, zsh, fish, and PowerShell:

```bash
git-wrangler completion --help
```

On bash, install and enable `bash-completion` if completions are not already
available.

## Quick Start

Show the built-in command overview:

```bash
git-wrangler
```

Open the full help and command-specific help:

```bash
git-wrangler help
git-wrangler help status
```

Set up GitHub and AI credentials when you need private GitHub workflows or
AI-assisted commits:

```bash
git-wrangler init
```

Check your local setup:

```bash
git-wrangler doctor
```

## Usage

```bash
git-wrangler <command> [flags]
```

Run `git-wrangler` for the compact command overview, `git-wrangler help` for
the full command list, and `git-wrangler help <command>` for command-specific
flags.

Git Wrangler discovers regular `.git` directories and linked worktree `.git`
files below the current directory, processes repositories in stable order, and
keeps repository result output ordered.

## Commands

| Command              | Purpose                                                           |
| -------------------- | ----------------------------------------------------------------- |
| `clone`              | Clone multiple GitHub repositories for a user or organization.    |
| `pull`               | Pull latest changes for discovered repositories.                  |
| `push`               | Push local commits to origin.                                     |
| `rename-repo`        | Rename GitHub repositories through `gh`.                          |
| `commit`             | Stage all changes and create a commit in each changed repository. |
| `commit-ai`          | Generate and create one AI Conventional Commit per repository.    |
| `config`             | Show and edit Git Wrangler setup.                                 |
| `doctor`             | Check runtime dependencies and local configuration.               |
| `fix-gitignore`      | Add missing common generated-file patterns to `.gitignore`.       |
| `info`               | Show detailed repository information.                             |
| `init`               | Set up GitHub and AI credentials.                                 |
| `license`            | Add or replace MIT license files.                                 |
| `remove-secrets`     | Purge sensitive files from Git history.                           |
| `rename-branch`      | Rename a branch across repositories.                              |
| `reset`              | Reset current branches to their origin counterparts.              |
| `review`             | Review unpushed changes across repositories.                      |
| `rewrite-authors`    | Rewrite author and committer identity.                            |
| `rewrite-commits`    | Rewrite commit messages to Conventional Commits.                  |
| `rewrite-commits-ai` | Generate AI Conventional Commit rewrites, then rewrite history.   |
| `rewrite-dates`      | Redistribute commit timestamps.                                   |
| `status`             | Show clean, dirty, ahead, behind, and remote state.               |
| `untrack`            | Stop tracking files already covered by `.gitignore`.              |
| `version`            | Print version metadata.                                           |
| `completion`         | Generate shell completion scripts.                                |
| `help`               | Show command help.                                                |

## Runtime Dependencies

| Tool                                   | Needed for                                                      |
| -------------------------------------- | --------------------------------------------------------------- |
| `git`                                  | Normal repository operations.                                   |
| `gh`                                   | GitHub repository operations such as `clone` and `rename-repo`. |
| `git-filter-repo`                      | History rewrite commands.                                       |
| OpenAI-compatible chat completions API | `commit-ai` and `rewrite-commits-ai`.                           |

`git-wrangler doctor` reports missing `git` as an error because most commands
need it. Missing `gh` or `git-filter-repo` is reported as a warning because
those tools are only needed for specific workflows.

Private or all-repository GitHub workflows require Git Wrangler GitHub auth:

```bash
git-wrangler init
```

AI commands require an OpenAI-compatible base URL, model, and API key configured
with `git-wrangler init`, `git-wrangler config`, or supported environment
variables.

## Safety

Git Wrangler is built for bulk operations, so risky paths are explicit.

- History rewrite commands require confirmation before mutation.
- `--yes` is the standard noninteractive confirmation flag.
- Destructive operations print warnings to stderr.
- Bulk commands keep going after per-repository failures, then exit nonzero if
  any repository failed.
- No-op skips remain successful.
- `push --force` uses `--force-with-lease`; raw force push is exposed as
  `push --force-unsafe`.
- AI commands redact sensitive file contents and common secret patterns before
  API calls.
- `rewrite-commits-ai` does not send old commit messages as model context.

History rewrite commands use `git-filter-repo`:

- `remove-secrets`
- `rewrite-authors`
- `rewrite-commits`
- `rewrite-commits-ai`
- `rewrite-dates`

## Development

Git Wrangler is a compiled Go CLI built with Cobra. The executable entrypoint is
`cmd/git-wrangler/main.go`; contributor-facing package boundaries and workflow
rules live in [AGENTS.md](AGENTS.md).

Common local checks:

| Task                     | Command                                 |
| ------------------------ | --------------------------------------- |
| Go test suite            | `go test ./...`                         |
| Race-enabled test suite  | `go test -race ./...`                   |
| Static vet checks        | `go vet ./...`                          |
| Full local check wrapper | `scripts/check`                         |
| Release dry run          | `goreleaser release --snapshot --clean` |

For website changes, also run:

```bash
cd website
pnpm run check
pnpm run build
pnpm audit --audit-level moderate
```

## License

MIT. See [LICENSE](LICENSE).
