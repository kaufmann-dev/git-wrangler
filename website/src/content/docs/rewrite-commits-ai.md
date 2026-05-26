---
title: "rewrite-commits-ai"
description: "Rewrites commit messages with an OpenAI-compatible AI endpoint."
category: "History Rewriting"
order: 3
usage: "git-wrangler rewrite-commits-ai --base-url <url> --model <model> [options]"
---

# rewrite-commits-ai

Generates Conventional Commit messages with an OpenAI-compatible chat completions endpoint, previews the results, and rewrites history with `git-filter-repo` after confirmation.

## Usage

```bash
git-wrangler rewrite-commits-ai --base-url <url> --model <model> [options]
```

## Options

- `--base-url <url>` is required. If it does not end in `/chat/completions`, Git Wrangler appends that path.
- `--model <model>` is required.
- `--api-key <key>` provides an API key for this run.
- `--api-key-env <name>` reads the API key from an environment variable. Defaults to `OPENAI_API_KEY`.
- `--batch-size <number>` defaults to `10` and must be between `1` and `50`.
- `--max-chars-per-commit <number>` defaults to `3000`.
- `--timeout <seconds>` defaults to `90`.
- `--skip-conventional` skips messages that already use Conventional Commits.

## Privacy controls

The command sends repository name, short commit id, file status, numstat, and redacted diff snippets.

It does not send API keys in commit context and does not include old commit messages in model context.

Sensitive file contents are hidden, including `.env`, private keys, credential or secret config files, and certificate/key bundles. Common secret-like tokens are redacted from diff text.

## Confirmation flow

Before any API call, Git Wrangler prints a data-send notice and asks for confirmation.

After valid messages are generated, Git Wrangler prints a summary and sample messages, then asks before rewriting history.

Noninteractive runs fail if a required confirmation cannot be collected.

## Example

```bash
git-wrangler rewrite-commits-ai \
  --base-url https://api.openai.com/v1 \
  --model gpt-4.1-mini \
  --api-key-env OPENAI_API_KEY
```

> **Warning:** This rewrites Git history. You will need to force-push to update remotes.
