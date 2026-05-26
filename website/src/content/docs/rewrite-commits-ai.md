---
title: "rewrite-commits-ai"
description: "Rewrites commit messages with an OpenAI-compatible AI endpoint."
category: "History Rewriting"
order: 3
usage: "git-wrangler rewrite-commits-ai --base-url <url> --model <model> [options]"
---

# rewrite-commits-ai

Rewrites commit messages with an OpenAI-compatible AI endpoint.

## Usage

```bash
git-wrangler rewrite-commits-ai --base-url <url> --model <model> [options]
```

## What it does

Scans all managed repositories, sends compact redacted commit context to an OpenAI-compatible chat completions endpoint, previews a single global summary, and asks once before rewriting history with `git-filter-repo`. By default, commits that already use Conventional Commits are processed too; use `--skip-conventional` to process only commits that do not already match the convention.

The command always processes all eligible commits. It does not have a dry-run mode; if you do not want to apply the generated messages, deny the final confirmation. The generated messages are temporary and are discarded when you decline.

## Options

| Flag                              | Required | Description                                                                 |
| --------------------------------- | -------- | --------------------------------------------------------------------------- |
| `--base-url <url>`                | Optional | OpenAI-compatible API base URL. Prompts if missing.                         |
| `--model <model>`                 | Optional | Model name to use. Prompts if missing.                                      |
| `--api-key <key>`                 | Optional | API key for the endpoint. Prompts if missing.                               |
| `--api-key-env <name>`            | Optional | Environment variable to read for the API key. Defaults to `OPENAI_API_KEY`. |
| `--batch-size <number>`           | Optional | Commits per API request. Defaults to `10`.                                  |
| `--max-chars-per-commit <number>` | Optional | Per-commit context budget. Defaults to `3000`.                              |
| `--timeout <seconds>`             | Optional | API request timeout. Defaults to `90`.                                      |
| `--skip-conventional`             | Optional | Only process commits that do not already use Conventional Commits.          |

## API setup

The command uses generic OpenAI-compatible chat completions. Configure the endpoint, model, and API key for your provider:

```bash
git-wrangler rewrite-commits-ai \
  --base-url https://api.openai.com/v1 \
  --model gpt-4o-mini \
  --api-key-env OPENAI_API_KEY
```

DeepSeek example:

```bash
git-wrangler rewrite-commits-ai \
  --base-url https://api.deepseek.com \
  --model deepseek-v4-flash \
  --api-key-env DEEPSEEK_API_KEY
```

If required values are missing, the command prompts in the terminal. You can save base URL, model, and API key environment defaults to `~/.git-wrangler/config`. If you choose to save an API key, Git Wrangler warns first because the key is stored as plaintext with file mode `600`.

## Cost and privacy controls

- Sends file paths, file stats, and redacted diff snippets only
- Does not send old commit messages as model context
- Batches commits to reduce request overhead
- Processes existing Conventional Commit messages unless `--skip-conventional` is provided
- Redacts common secrets and hides sensitive file contents before sending data
- Shows endpoint, model, repository count, total and selected commit counts, batch count, and context budget before the first API request

## Prerequisites

- `git-filter-repo` must be installed
- Python 3 must be available
- An OpenAI-compatible API endpoint, model, and API key are required

## Notes

> **Warning:** This rewrites Git history. You will need to force-push to update remotes.

- The final confirmation happens once globally, not once per repository
- Failed API batches are retried automatically, and remaining failed commits can be retried without regenerating successful commits
- Remote `origin` is preserved and restored after each rewrite
