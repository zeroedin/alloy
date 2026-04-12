# Alloy

A fast, extensible static site generator. Built in Go, ships as a single binary.

## Why Alloy?

Static site generators tend to make you choose between speed and extensibility. Alloy doesn't. It compiles thousands of pages in seconds, uses Liquid templates for a familiar developer experience, and supports a tiered plugin system — from in-process WASM to a full Node.js bridge — without sacrificing build performance.

## Features

- **Fast builds** — Go-powered concurrent pipeline targeting < 5s for 1,000 pages
- **Liquid templates** — Familiar syntax with Go `html/template` as an alternative
- **Data cascade** — Global data, directory-level `_data.yaml`, and front matter merge predictably (5 levels, last wins)
- **Collections and taxonomies** — Date-based section collections, cross-cutting taxonomy groups, auto-generated term pages
- **Pagination** — Paginated lists and virtual page generation from data
- **Content lifecycle** — Draft/future/expired content with sensible defaults
- **Permalinks** — Fast token replacement with Liquid fallback for complex patterns
- **Multiple output formats** — HTML, JSON, XML, or anything else from a single content file
- **i18n** — Opt-in multilingual support with per-language content trees and shared layouts
- **Incremental rebuilds** — Content-hash change detection, fine-grained template invalidation
- **Web Component SSR** — Opt-in two-phase rendering with deduplicated Declarative Shadow DOM (golit, lit-ssr-wasm, or Lit SSR via Node)
- **Tiered plugin system** — Built-in Go filters (ns), in-process JS/WASM plugins (us), Node subprocess plugins (ms)
- **Dev server** — File watching, WebSocket live reload, in-memory rendering, error reporting

## Quick Start

```bash
# Initialize a new project
alloy init

# Start the dev server
alloy serve

# Build for production
alloy build
```

## Project Structure

```
my-site/
├── alloy.config.yaml      # Site configuration (YAML, TOML, or JSON)
├── content/               # Content files (Markdown, HTML)
│   ├── index.md
│   └── blog/
│       ├── _data.yaml     # Directory-level data (cascades down)
│       └── first-post.md
├── layouts/               # Liquid templates
│   ├── default.liquid
│   └── partials/
├── assets/                # Processed via plugin hooks
├── static/                # Copied as-is to output
├── data/                  # Global data files (YAML, JSON, TOML, CSV)
└── plugins/               # JS, WASM, or Node plugins
```

## Configuration

```yaml
# alloy.config.yaml
title: "My Site"
baseURL: "https://example.com"

build:
  output: "_site"
  clean: true

content:
  formats: ["md", "html"]

permalinks:
  blog: "/:year/:month/:slug/"
  default: "/:slug/"

taxonomies:
  tags:
  categories:
```

All directory paths are configurable via the `structure:` block. External directories can be mapped into the output with `passthrough:`.

## Content

Content files live in `content/` and require front matter (YAML `---`, TOML `+++`, or JSON `{`):

```markdown
---
title: "My First Post"
date: 2026-04-10
tags: ["go", "static-sites"]
draft: false
---

# Hello World

Published on {{ page.date | date: "%B %d, %Y" }}.
```

Template tags (`{{ }}` and `{% %}`) work directly in Markdown — no special syntax needed. A goldmark extension preserves them through Markdown processing automatically.

## Templates

Alloy supports two built-in template engines (project-wide setting):

**Liquid** (default):
```liquid
{% include "partials/header" %}
<h1>{{ page.title }}</h1>
{{ content }}
```

**Go** (`html/template`):
```html
{{ template "partials/header" . }}
<h1>{{ .page.title }}</h1>
{{ .content }}
```

Both engines share the same layout lookup order, data context, and built-in filters.

## Data Cascade

Five levels, last wins:

1. Global data (`data/*.yaml`)
2. Directory data (`content/blog/_data.yaml` — cascades into subdirs)
3. Front matter (per-file)
4. Pre-render computed data (plugin hook)
5. Post-render computed data (plugin hook)

Objects deep-merge. Arrays replace. Shared data is loaded once and referenced by pointer — no deep copies per page.

## Plugins

Three tiers, matched to what the plugin needs:

| Tier | Runtime | Latency | Use case |
|------|---------|---------|----------|
| 1 | Go built-in | ~ns | String, date, array, URL, math filters |
| 2 | JS (QuickJS) or WASM via wazero | ~us | Custom filters, shortcodes, data transforms |
| 3 | Node subprocess | ~ms | PostCSS, Sharp, Lit SSR, npm packages |

Drop a `.js` file in `plugins/` and it runs on embedded QuickJS. Export `runtime: "node"` for Node. Drop a `.wasm` file for compiled plugins. No config needed.

```javascript
// plugins/word-count.js
export default function(alloy) {
  alloy.filter("wordCount", (content) => {
    return content.split(/\s+/).filter(w => w.length > 0).length;
  });
}
```

## Web Component SSR (Opt-In)

Alloy's two-phase rendering separates content rendering from component SSR. Phase 1 produces intermediate HTML with raw component tags. Phase 2 (if configured) scans for custom elements, deduplicates instances by tag + attributes, SSR's each unique instance once, and stamps Declarative Shadow DOM back into every page.

Supports golit (Go-native), lit-ssr-wasm (official Lit SSR compiled to WASM), or Lit SSR on Node as a plugin.

## CLI

```
alloy init                  Create a new project config
alloy build                 Build the site to _site/
alloy serve                 Dev server with live reload
alloy serve --preview       Production pipeline served locally
alloy version               Print version
```

**Flags:**
```
--config, -c       Config file path (default: alloy.config.yaml)
--output, -o       Output directory (default: _site)
--port, -p         Dev server port (default: 3000)
--verbose, -v      Verbose logging
--quiet, -q        Suppress output
--no-drafts        Hide drafts in dev mode
--refetch          Bypass external data cache
```

## Performance Targets

| Scenario | Target |
|----------|--------|
| 1,000 pages (no SSR) | < 5 seconds |
| 1,000 pages (with SSR) | < 10 seconds |
| Incremental rebuild (dev) | < 200ms |
| Cold start to first serve | < 3 seconds |

## Dependencies

| Package | Purpose | License |
|---------|---------|---------|
| [liquidgo](https://github.com/Notifuse/liquidgo) | Liquid template engine | MIT |
| [goldmark](https://github.com/yuin/goldmark) | Markdown rendering | MIT |
| [cobra](https://github.com/spf13/cobra) | CLI framework | Apache-2.0 |
| [fsnotify](https://github.com/fsnotify/fsnotify) | File watching | BSD-3 |
| [wazero](https://github.com/tetratelabs/wazero) | WASM runtime (plugins) | Apache-2.0 |
| [gorilla/websocket](https://github.com/gorilla/websocket) | Live reload | BSD-3 |

## License

[MIT](LICENSE)
