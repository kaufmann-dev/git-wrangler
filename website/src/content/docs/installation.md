---
title: "Installation"
description: "Install Git Wrangler with Homebrew, Scoop, or GitHub Release binaries."
category: "General"
order: 2
---

# Installation

## Homebrew

Homebrew is the recommended install path on macOS and Linux:

```bash
brew install kaufmann-dev/tap/git-wrangler
```

Upgrade with:

```bash
brew upgrade git-wrangler
```

Homebrew installs bash, zsh, and fish completions automatically.

## Scoop

Scoop is the recommended native Windows install path:

```powershell
scoop bucket add kaufmann-dev https://github.com/kaufmann-dev/scoop-bucket.git
scoop install kaufmann-dev/git-wrangler
```

Scoop installs `git`, `gh`, and `git-filter-repo`.

## GitHub Releases

Windows users can also run Git Wrangler inside WSL using the Linux/Homebrew path or download the matching Windows archive from GitHub Releases, extract the `git-wrangler` binary, and place it on `PATH`.

Manual binary installs do not install runtime dependencies. Install the dependencies below yourself as needed for the commands you run.

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

## Uninstall

If installed with Homebrew:

```bash
brew uninstall git-wrangler
```

If installed with Scoop:

```powershell
scoop uninstall git-wrangler
```

If installed manually, remove the binary from wherever you placed it on `PATH`.
