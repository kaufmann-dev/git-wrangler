# Git Wrangler Website

This is the documentation hub and landing page for the **Git Wrangler** CLI, built with [Astro](https://astro.build).

## 🚀 Overview

The Git Wrangler website is designed to be a premium, high-performance documentation site featuring:
- **Geist-inspired Design System**: A sleek, developer-focused aesthetic with dark/light mode support.
- **Dynamic Content Collections**: Automated documentation generation directly from the CLI's `README.md` and metadata headers in the `libexec` scripts.
- **Modular Component Architecture**: Built using a Vanilla-CSS component library for maximum flexibility and performance.
- **Seamless Navigation**: Powered by Astro's client-side routing for an app-like feel.

## 🧞 Commands

All commands are run from the root of the `website` directory from a terminal:

| Command                   | Action                                           |
| :------------------------ | :----------------------------------------------- |
| `npm install`             | Installs dependencies                            |
| `npm run dev`             | Starts local dev server at `localhost:4321`      |
| `npm run build`           | Build the production site to `./dist/`           |
| `npm run preview`         | Preview your build locally, before deploying     |

## 📂 Project Structure

```text
website/
├── public/           # Static assets (images, fonts, etc.)
├── src/
│   ├── components/   # Modular Vanilla-CSS and Astro components
│   ├── content/      # Astro Content Collections (Documentation)
│   ├── layouts/      # Page layouts
│   ├── pages/        # File-based routing (e.g., index.astro, docs/[...slug].astro)
│   └── styles/       # Global CSS variables and design system tokens
└── astro.config.mjs  # Astro configuration
```

## 🛠️ Development

When developing the site, keep in mind:
- **Component Styling**: We use vanilla CSS. Please avoid ad-hoc utility classes (like Tailwind) and instead utilize the defined CSS variables in our design system.
- **Documentation Sync**: Documentation pages are automatically sourced using Astro's Content Collections API. Ensure that changes to CLI metadata headers map correctly in the glob loader pipeline.
