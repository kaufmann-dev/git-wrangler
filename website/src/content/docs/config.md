---
title: "config"
description: "Shows and edits Git Wrangler setup."
category: "Utility"
order: 2
usage: "git-wrangler config <show|set|unset>"
---

# config

Shows and edits Git Wrangler setup.

## Usage

```bash
git-wrangler config show
git-wrangler config set github.auth
git-wrangler config set github.host <host>
git-wrangler config set ai.provider <name>
git-wrangler config set ai.base-url <url>
git-wrangler config set ai.model <model>
git-wrangler config set ai.api-key
git-wrangler config unset github.auth
git-wrangler config unset ai.api-key
```

## What it does

`config show` prints non-secret setup values and reports each credential source as `keyring`, `env`, or `missing`.

`config set github.auth` and `config set ai.api-key` prompt for secrets and store them in the system keyring. They do not accept plaintext secret values as command arguments.

Other `config set` commands write non-secret values to the JSON config file under the user config directory.

`config unset` removes the matching keyring secret.

## Environment overrides

Environment variables override keyring credentials without writing secrets to disk:

- GitHub: `GIT_WRANGLER_GITHUB_TOKEN`, then `GH_TOKEN`
- AI: `GIT_WRANGLER_AI_API_KEY`
- OpenAI provider fallback: `OPENAI_API_KEY`
