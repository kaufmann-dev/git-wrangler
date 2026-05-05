---
title: "review"
description: "Reviews committed changes before pushing across repositories."
category: "Local Operations"
order: 2
usage: "wrangler review"
---

# review

Reviews committed changes before pushing across repositories.

## Usage

```bash
wrangler review
```

This command takes no arguments.

## What it does

Iterates through Git repositories and displays a summary of added, edited, and removed files that have been committed but not yet pushed. If an entire directory was deleted, it groups the changes and reports the directory deletion instead of listing every individual file.

## Example output

```
my-api:
  Added:    src/auth/jwt.ts
  Added:    src/auth/middleware.ts
  Edited:   src/index.ts
  Removed:  old-module/ (entire folder)

design-system:
  Edited:   tokens/colors.css
  Edited:   tokens/spacing.css
```

## Example

```bash
wrangler review
```

## Use case

Run `wrangler review` before `wrangler push` to verify exactly what will be pushed across all repositories — without opening each one individually.
