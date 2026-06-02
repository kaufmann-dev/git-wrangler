---
title: "init"
description: "Sets up Git Wrangler GitHub and AI credentials."
category: "Utility"
order: 1
usage: "git-wrangler init"
---

# init

Sets up Git Wrangler's GitHub and AI credentials.

## Usage

```bash
git-wrangler init
```

## What it does

Prompts for the GitHub host, starts GitHub device OAuth, stores the GitHub token in the system keyring, and saves the authenticated username in non-secret config.

It also prompts for AI provider settings and can store the AI API key in the system keyring.

Secrets are never written to the JSON config file.

## Environment overrides

GitHub auth can come from `GIT_WRANGLER_GITHUB_TOKEN` or `GH_TOKEN`.

AI auth can come from `GIT_WRANGLER_AI_API_KEY`. For the `openai` provider, `OPENAI_API_KEY` is also accepted.
