# Git Wrangler Website

This is the documentation hub and landing page for the Git Wrangler CLI, built with Astro.

## Overview

The site documents the current Go/Cobra CLI, package-manager installation, GitHub Release binaries, command usage, and release architecture.

## Commands

Run these from the `website` directory:

```bash
pnpm install
pnpm dev
pnpm build
pnpm preview
```

## Project Structure

```text
website/
├── public/
├── src/
│   ├── components/
│   ├── content/
│   ├── layouts/
│   ├── pages/
│   └── styles/
└── astro.config.mjs
```

Use the existing CSS variables and component patterns when changing the site. Documentation pages are Markdown files loaded through Astro Content Collections.
