---
title: "rewrite-commits-ai"
description: "Rewrites commit messages with an OpenAI-compatible AI endpoint."
category: "AI Commands"
order: 2
usage: "git-wrangler rewrite-commits-ai [options]"
---

# rewrite-commits-ai

Generates Conventional Commit messages with an OpenAI-compatible chat completions endpoint, previews the results, and rewrites history with `git-filter-repo` after confirmation.

By default, generated messages are subject-only. Use `--body` to generate a subject and body.

## Usage

```bash
git-wrangler rewrite-commits-ai [options]
```

## Options

- `--batch-size <number>` defaults to `10` and must be between `1` and `50`.
- `--max-chars-per-commit <number>` defaults to `3000`.
- `--timeout <seconds>` defaults to `90`.
- `--skip-conventional` skips messages that already use Conventional Commits.
- `--body` generates a subject and body instead of subject only.
- `--yes` skips the data-send and rewrite confirmation prompts.

AI provider, base URL, model, and API key come from `git-wrangler init`, `git-wrangler config`, or supported environment variables.

## Privacy controls

The command sends repository name, short commit id, file status, numstat, and redacted diff snippets.

It does not send API keys in commit context and does not include old commit messages in model context.

Sensitive file contents are hidden, including `.env`, private keys, credential or secret config files, and certificate/key bundles. Common secret-like tokens are redacted from diff text.

## Confirmation flow

Before any API call, Git Wrangler prints a data-send notice and asks for confirmation.

After valid messages are generated, Git Wrangler prints a summary and sample messages, then asks before rewriting history.

Use `--yes` for noninteractive runs after configuring all required values or setting environment variables.

## Example

```bash
git-wrangler config set ai.model gpt-4.1-mini
git-wrangler config set ai.api-key
git-wrangler rewrite-commits-ai
```

> **Warning:** This rewrites Git history. You will need to force-push to update remotes.
