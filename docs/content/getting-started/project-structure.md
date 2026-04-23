---
title: "Project Structure"
layout: "doc"
weight: 3
section: "getting-started"
description: "Understand Alloy's directory layout and what each directory does."
---

## Default Layout

```
my-site/
├── alloy.config.yaml      # Site configuration
├── content/               # Content files (Markdown, HTML)
│   ├── index.md
│   └── blog/
│       ├── _data.yaml     # Directory-level data (cascades down)
│       └── first-post.md
├── layouts/               # Liquid templates
│   ├── default.liquid     # Fallback layout
│   └── partials/          # Reusable template fragments
├── assets/                # Processed via plugin hooks
├── static/                # Copied as-is to output
├── data/                  # Global data files (YAML, JSON, TOML, CSV)
└── plugins/               # JS, WASM, or Node plugins
```

## Directories

### `content/`

Content files with front matter. Supports Markdown (`.md`), HTML (`.html`), and plain text (`.txt`). Subdirectories create URL structure. `_data.yaml` files cascade data to all pages in the directory and below.

### `layouts/`

Liquid templates (`.liquid`) or Go templates (`.html`). Layout resolution: front matter `layout:` field > section name > filename match > `default.liquid` fallback.

### `data/`

Global data files accessible in templates as `site.data.*`. Supports YAML, JSON, TOML, and CSV. A file at `data/navigation.yaml` is available as `site.data.navigation`.

### `static/`

Files copied verbatim to the output. No processing, no fingerprinting. `static/robots.txt` becomes `_site/robots.txt`.

### `assets/`

Files copied to output with optional plugin processing via the `onAssetProcess` hook. Use plugins for CSS minification, image optimization, etc.

### `plugins/`

Drop a `.js` file for QuickJS (Tier 2), a `.wasm` file for compiled WASM (Tier 2), or a `.js` file with `export const runtime = "node"` for Node (Tier 3). No configuration needed.

### `_site/`

Build output. Created by `alloy build`, served by `alloy serve --preview`. Never edit files here — they're regenerated on every build.

### `.alloy/`

Cache directory. Contains `cache.json` (content hashes for incremental rebuilds), `components.json` (SSR component tracking), and `fetch-cache/` (external data source cache).

## Custom Paths

Override any directory with the `structure:` config block:

```yaml
structure:
  content: "./docs/pages/"
  layouts: "./docs/layouts/"
  assets: "./docs/assets/"
  static: "./docs/static/"
  data: "./data/"
```

All paths are relative to the project root.
