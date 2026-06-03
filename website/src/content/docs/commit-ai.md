---
title: "commit-ai"
description: "Generates and creates one AI Conventional Commit per changed repository."
category: "AI Commands"
order: 1
usage: "git-wrangler commit-ai [options]"
---

# commit-ai

Generates one Conventional Commit message per changed repository with an OpenAI-compatible chat completions endpoint, then creates the commits.

## Usage

```bash
git-wrangler commit-ai [options]
```

## What it does

Iterates through Git repositories found under the current directory, prepares `git add -A` context with a temporary index, skips repositories with no changes, sends redacted staged change context to the configured AI endpoint, and commits each changed repository with the generated message.

Diff bodies for common generated, vendor, cache, build, and upload paths are hidden while file names and stats remain visible.

If generation fails for any repository after retries, Git Wrangler creates no commits and exits nonzero. The real index is staged only after valid AI messages are available.

## Options

| Flag                           | Default | Description                                            |
| ------------------------------ | ------- | ------------------------------------------------------ |
| `--max-chars-per-commit <num>` | `3000`  | Maximum redacted staged context characters per commit. |
| `--rpm <num>`                  | `300`   | Maximum API requests to start per minute.              |
| `--timeout <seconds>`          | `90`    | API timeout in seconds.                                |
| `--body`                       | `false` | Generate a subject and body instead of subject only.   |
| `--yes`                        | `false` | Skip the data-send confirmation prompt.                |

AI provider, base URL, model, and API key come from `git-wrangler init`, `git-wrangler config`, or supported environment variables.

## Confirmation flow

Before any API call, Git Wrangler prints the endpoint, model, repository count, context budget, and privacy summary, then asks for confirmation.

After valid messages are generated, commits are created automatically. There is no second confirmation prompt.

## Example

```bash
git-wrangler config set ai.model gpt-4.1-mini
git-wrangler config set ai.api-key
git-wrangler commit-ai --body
```
