---
title: "fix-gitignore"
description: "Audits and fixes .gitignore files by adding missing entries."
category: "Local Operations"
order: 5
usage: "git-wrangler fix-gitignore"
---

# fix-gitignore

Audits and fixes `.gitignore` files by adding missing entries.

## Usage

```bash
git-wrangler fix-gitignore
```

This command takes no arguments.

## What it does

Audits and fixes `.gitignore` files across Git repositories found under the current directory. Adds missing entries for files and directories that physically exist in the repo but are not yet covered by `.gitignore` rules.

**Candidate entries checked:**

| Pattern         | Applies to               |
| --------------- | ------------------------ |
| `node_modules/` | JavaScript/Node projects |
| `dist/`         | Build output             |
| `build/`        | Build output             |
| `bin/`          | Compiled binaries        |
| `obj/`          | .NET intermediate files  |
| `.idea/`        | JetBrains IDE config     |
| `vendor/`       | PHP/Go dependencies      |
| `wp-includes/`  | WordPress core           |
| `.DS_Store`     | macOS metadata           |
| `Thumbs.db`     | Windows thumbnail cache  |
| `*.log`         | Log files                |

## Example output

```
my-api:
  Added: node_modules/, .DS_Store
  Committed .gitignore updates
  Skipped (already covered): dist/
  Skipped (not present on disk): .idea/, vendor/
```

## Example

```bash
git-wrangler fix-gitignore
```

## Notes

- Only adds entries — never removes existing rules
- Only adds a pattern if that file/directory **physically exists on disk**
- Commits the updated `.gitignore` automatically
- Does not untrack already-tracked files (use [`git-wrangler untrack`](/docs/untrack) for that)
