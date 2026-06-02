---
title: "Installation"
description: "Install Git Wrangler with Homebrew or GitHub Release binaries."
category: "General"
order: 2
---

# Installation

## Homebrew

Homebrew is the primary install path on macOS and Linux:

```bash
brew install kaufmann-dev/tap/git-wrangler
```

Upgrade with:

```bash
brew upgrade git-wrangler
```

Homebrew installs bash, zsh, and fish completions automatically.

## GitHub Releases

Windows users can either run Git Wrangler inside WSL using the Linux/Homebrew path or download the matching Windows archive from GitHub Releases, extract the `git-wrangler` binary, and place it on `PATH`.

Official packaged installs include runtime dependencies. Source installs do not, so install the dependencies below yourself as needed for the commands you run.

Run `git-wrangler doctor` after installation to check the local tools Git Wrangler can use.

## Runtime dependencies

`git` is required for normal repository operations.

`gh` is required for GitHub repository operations such as `clone` and `rename-repo`. Run this before private or all-repository workflows:

```bash
gh auth login
```

`git-filter-repo` is required for history rewrite commands:

- `remove-secrets`
- `rewrite-authors`
- `rewrite-commits`
- `rewrite-commits-ai`
- `rewrite-dates`

`git-wrangler doctor` reports missing `git` as an error because most commands need it. Missing `gh` or `git-filter-repo` is reported as a warning because those tools are only needed for specific workflows.

## Uninstall

If installed with Homebrew:

```bash
brew uninstall git-wrangler
```

If installed manually, remove the binary from wherever you placed it on `PATH`.
