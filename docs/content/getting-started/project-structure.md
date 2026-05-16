---
layout: doc
title: Project Structure
---

A typical Alloy project:

```
my-site/
├── alloy.config.yaml       # Site configuration
├── content/                 # Content files (Markdown, HTML)
│   ├── index.md             # Home page → /
│   ├── about.html           # Standalone page → /about/
│   └── blog/
│       ├── _data.yaml       # Directory-level defaults
│       ├── index.md         # Blog listing → /blog/
│       └── first-post.md    # Blog post → /blog/first-post/
├── layouts/                 # Liquid templates
│   ├── default.liquid       # Fallback layout
│   ├── post.liquid          # Post-specific layout
│   └── partials/            # Reusable fragments
│       ├── header.liquid
│       └── footer.liquid
├── static/                  # Copied to output as-is
├── data/                    # Global data files (YAML, JSON, CSV)
│   └── navigation.yaml
└── plugins/                 # QuickJS, WASM, or Node plugins
    └── word-count.js
```

## Directories

**`content/`** — Markdown and HTML pages. Subdirectories become URL segments. Each directory can include a `_data.yaml` file that sets defaults for all pages beneath it.

**`layouts/`** — Liquid templates that wrap content. Pages reference a layout by name in front matter (`layout: post`). The `default.liquid` layout is the fallback when no layout is specified. The `partials/` subdirectory holds reusable fragments included with `{% include "partials/header" %}`.

**`static/`** — Files copied verbatim to the output directory. Images, fonts, pre-built CSS and JS belong here.

**`data/`** — Global data files available in templates as `site.data.<filename>`. A file at `data/navigation.yaml` is accessible as `site.data.navigation`.

**`plugins/`** — Plugin files. Alloy supports three tiers: embedded QuickJS (`.js`), compiled WASM (`.wasm`), and Node.js (`.js` with `runtime: "node"`). See [Plugins](/plugins/) for details.

## Configuration

Alloy looks for a config file at the project root. Supported formats:

| File | Format |
|---|---|
| `alloy.config.yaml` | YAML |
| `alloy.config.yml` | YAML |
| `alloy.config.toml` | TOML |
| `alloy.config.json` | JSON |

A minimal config:

```yaml
title: "My Site"
baseURL: "https://example.com"
build:
  output: "_site"
```

## Custom directory paths

The `structure:` config overrides default directory locations. This is useful for monorepos or projects with non-standard layouts:

```yaml
# alloy.config.yaml — monorepo example
structure:
  content: "./docs/pages/"
  layouts: "./docs/layouts/"
  static: "./docs/static/"
  data: "./data/"
```

All paths are relative to the project root. When a key is omitted, it defaults to the standard name.

## Front matter formats

Content files require front matter. The format is detected by the opening delimiter:

| Delimiter | Format | Example |
|---|---|---|
| `---` | YAML | `---`<br>`title: "My Post"`<br>`---` |
| `+++` | TOML | `+++`<br>`title = "My Post"`<br>`+++` |
| `{` | JSON | `{ "title": "My Post" }` |

Front matter is required on all content files. Empty front matter (`---` followed by `---`) is valid when a page has no metadata to set. Layouts, partials, and data files do not use front matter.
