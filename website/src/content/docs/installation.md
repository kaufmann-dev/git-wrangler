---
title: "Installation"
description: "Install Git Wrangler with Homebrew or GitHub Release binaries."
category: "General"
order: 2
---

# Installation

## Homebrew

Homebrew is the primary install path:

```bash
brew install --cask kaufmann-dev/tap/git-wrangler
```

Upgrade with:

```bash
brew upgrade --cask git-wrangler
```

Homebrew installs bash, zsh, and fish completions automatically.

## GitHub Releases

Linux and Windows users can download the matching archive from GitHub Releases, extract the `git-wrangler` binary, and place it on `PATH`.

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

Check your environment with:

```bash
git-wrangler doctor
```

## Uninstall

If installed with Homebrew:

```bash
brew uninstall git-wrangler
```

If installed manually, remove the binary from wherever you placed it on `PATH`.
