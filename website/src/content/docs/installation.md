---
title: "Installation"
description: "Install Git Wrangler with a single curl command."
category: "General"
order: 2
---

# Installation

## Prerequisites

Before installing Git Wrangler, make sure you have:

| Dependency                   | Required for                          | Install                                             |
| ---------------------------- | ------------------------------------- | --------------------------------------------------- |
| `git`                        | All operations                        | [git-scm.com](https://git-scm.com/)                 |
| `gh`                         | `clone`, `rename-repo`                | [cli.github.com](https://cli.github.com/)           |
| `git-filter-repo`            | History rewriting                     | [GitHub](https://github.com/newren/git-filter-repo) |
| Python 3                     | `rewrite-commits-ai`, `rewrite-dates` | [python.org](https://www.python.org/)               |
| OpenAI-compatible API access | `rewrite-commits-ai`                  | Your provider's API dashboard                       |

`git` is required for everything. `gh`, `git-filter-repo`, Python 3, and API access are only needed if you plan to use their respective commands.

## One-liner install

Run the following in your terminal:

```bash
curl -fsSL https://raw.githubusercontent.com/kaufmann-dev/git-wrangler/main/install | bash
```

This will:
1. Clone the repository to `~/.git-wrangler`
2. Create a symlink at `~/.local/bin/git-wrangler` (or `/usr/local/bin` as fallback)
3. Verify the installation

## Platform support

| Platform           | Status       |
| ------------------ | ------------ |
| macOS              | ✅ Native    |
| Linux              | ✅ Native    |
| Windows (Git Bash) | ✅ Supported |
| Windows (WSL)      | ✅ Supported |

## Verify the installation

```bash
git-wrangler help
```

You should see the help menu listing all available commands.

## Updating

Keep Git Wrangler up to date by running:

```bash
git-wrangler update
```

This compares your local commit against the remote and applies the update if one is available.

## Uninstalling

To remove Git Wrangler cleanly:

```bash
git-wrangler uninstall
```

This removes the `~/.git-wrangler` directory and the `git-wrangler` symlink. No other files are touched.

## Manual install

If you prefer to clone manually:

```bash
git clone https://github.com/kaufmann-dev/git-wrangler ~/.git-wrangler
ln -s ~/.git-wrangler/git-wrangler ~/.local/bin/git-wrangler
chmod +x ~/.git-wrangler/git-wrangler ~/.git-wrangler/libexec/git-wrangler-*
```
