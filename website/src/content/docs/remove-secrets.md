---
title: "remove-secrets"
description: "Permanently purges sensitive files from the entire Git history."
category: "History Rewriting"
order: 4
usage: "wrangler remove-secrets"
---

# remove-secrets

Permanently purges sensitive files from the entire Git history.

## Usage

```bash
wrangler remove-secrets
```

This command takes no arguments.

## What it does

Permanently purges files containing sensitive data from the **entire Git history** of all managed repositories (across all branches and tags). Scans for common secret file patterns and removes any matches using `git-filter-repo`. The remote origin URL is automatically restored after the rewrite.

## Target patterns

The following patterns are scanned and removed if found in history:

| Pattern | Description |
|---|---|
| `.env`, `.env.*` | Environment variable files |
| `*.pem`, `*.key` | TLS/SSL certificates and private keys |
| `*.p12`, `*.pfx` | PKCS#12 certificate stores |
| `id_rsa`, `id_rsa.pub` | RSA SSH keys |
| `id_ed25519`, `id_ed25519.pub` | Ed25519 SSH keys |
| `config.json` | Configuration files |
| `secrets.json`, `credentials.json` | Secret stores |
| `*.secret` | Secret files |

## Prerequisites

- `git-filter-repo` must be installed

## Example

```bash
wrangler remove-secrets
```

## Notes

> **Warning:** This permanently rewrites Git history. You will need to force-push to update remotes, and all collaborators must re-clone.

- The command scans history first and reports found files before removing them
- Repositories with no matching patterns are skipped cleanly
- Remote `origin` is preserved and restored after the rewrite
