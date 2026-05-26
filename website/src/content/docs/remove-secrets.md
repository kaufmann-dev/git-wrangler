---
title: "remove-secrets"
description: "Permanently purges sensitive files from the entire Git history."
category: "History Rewriting"
order: 5
usage: "git-wrangler remove-secrets --confirm"
---

# remove-secrets

Permanently purges sensitive files from the entire Git history.

## Usage

```bash
git-wrangler remove-secrets --confirm
```

`--confirm` is required before any destructive history rewrite is performed.

## What it does

Permanently purges files containing sensitive data from the **entire Git history** of all managed repositories (across all branches and tags). Scans for common secret file patterns and removes any matches using `git-filter-repo`. The remote origin URL is automatically restored after the rewrite.

## Target patterns

The following patterns are scanned and removed if found in history:

| Pattern group                        | Examples                                                                                                                                |
| ------------------------------------ | --------------------------------------------------------------------------------------------------------------------------------------- |
| Environment and package credentials  | `.env`, `.env.*`, `.npmrc`, `.pypirc`, `.netrc`, `.git-credentials`                                                                     |
| Private keys and certificates        | `*.pem`, `*.key`, `*.p12`, `*.pfx`, `*.asc`, `*.gpg`, `*.crt`, `*.cer`, `*.cert`                                                        |
| SSH keys                             | `id_rsa`, `id_rsa.pub`, `id_ed25519`, `id_ed25519.pub`, `*_rsa`, `*_ed25519`                                                            |
| Secret stores                        | `secrets.json`, `credentials.json`, `*secret*.json`, `*credential*.json`, `*.secret`                                                    |
| Container and Kubernetes credentials | `.docker/config.json`, `.kube/config`, `kubeconfig`                                                                                     |
| Cloud credentials                    | `.aws/credentials`, `.aws/config`, `.config/gcloud/*`, `application_default_credentials.json`, `azureProfile.json`, `accessTokens.json` |

## Prerequisites

- `git-filter-repo` must be installed

## Options

| Flag        | Required     | Description                               |
| ----------- | ------------ | ----------------------------------------- |
| `--confirm` | **Required** | Confirm noninteractive history rewriting. |

## Example

```bash
git-wrangler remove-secrets --confirm
```

## Notes

> **Warning:** This permanently rewrites Git history. You will need to force-push to update remotes, and all collaborators must re-clone.

- The command scans history first and reports found files before requiring `--confirm` to remove them
- Generic `config.json` files are not removed unless they match a credential-specific path such as `.docker/config.json`
- Repositories with no matching patterns are skipped cleanly
- Remote `origin` is preserved and restored after the rewrite
