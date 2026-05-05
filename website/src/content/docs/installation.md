---
title: "Installation"
description: "Install Git Wrangler with a single curl command."
category: "General"
order: 2
---

# Installation

## Prerequisites

Before installing Git Wrangler, make sure you have:

| Dependency | Required for | Install |
|---|---|---|
| [`git`](https://git-scm.com/) | All operations | [git-scm.com](https://git-scm.com/) |
| [`gh`](https://cli.github.com/) | `clone`, `rename-repo` | [cli.github.com](https://cli.github.com/) |
| [`git-filter-repo`](https://github.com/newren/git-filter-repo) | History rewriting | [GitHub](https://github.com/newren/git-filter-repo) |

`git` is required for everything. `gh` and `git-filter-repo` are only needed if you plan to use their respective commands.

## One-liner install

Run the following in your terminal:

```bash
curl -fsSL https://raw.githubusercontent.com/kaufmann-dev/git-wrangler/main/install | bash
```

This will:
1. Clone the repository to `~/.wrangler`
2. Create a symlink at `~/.local/bin/wrangler` (or `/usr/local/bin` as fallback)
3. Verify the installation

## Platform support

| Platform | Status |
|---|---|
| macOS | ✅ Native |
| Linux | ✅ Native |
| Windows (Git Bash) | ✅ Supported |
| Windows (WSL) | ✅ Supported |

## Verify the installation

```bash
wrangler help
```

You should see the help menu listing all available commands.

## Updating

Keep Git Wrangler up to date by running:

```bash
wrangler update
```

This compares your local commit against the remote and applies the update if one is available.

## Uninstalling

To remove Git Wrangler cleanly:

```bash
wrangler uninstall
```

This removes the `~/.wrangler` directory and the `wrangler` symlink. No other files are touched.

## Manual install

If you prefer to clone manually:

```bash
git clone https://github.com/kaufmann-dev/git-wrangler ~/.wrangler
ln -s ~/.wrangler/wrangler ~/.local/bin/wrangler
chmod +x ~/.wrangler/wrangler ~/.wrangler/libexec/wrangler-*
```
