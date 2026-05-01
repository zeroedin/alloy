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
- **Incremental rebuilds (dev mode)** — Content-hash change detection in `alloy dev` for fast rebuilds on file changes. `alloy build` always does a full clean rebuild for CI/CD reliability.
- **Web Component SSR** — Opt-in two-phase rendering: pipe each page to an external SSR engine via stdin/stdout for Declarative Shadow DOM
- **Tiered plugin system** — Built-in Go filters (ns), in-process JS/WASM plugins (us), Node subprocess plugins (ms)
- **Dev server** — File watching, WebSocket live reload, in-memory rendering, error reporting

## Quick Start

```bash
# Initialize a new project
alloy init

# Start the dev server
alloy dev

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

Template tags (`{{ }}` and `{% %}`) work directly in Markdown — no special syntax needed. Two goldmark extensions handle them: inline tags are preserved as-is, and block shortcodes (`{% tag %}...{% endtag %}` on their own lines) are treated as block-level boundaries so they don't get wrapped in `<p>` tags.

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

Set the engine in config:

```yaml
templates:
  engine: "liquid"   # default — or "go" for html/template
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

  alloy.shortcode("youtube", (args) => {
    return `<iframe src="https://www.youtube.com/embed/${args[0]}"></iframe>`;
  });

  alloy.hook("onContentTransformed", (page) => {
    // page.html, page.toc, page.path, page.url, page.frontMatter
    page.html = page.html.replace(/<img /g, '<img loading="lazy" ');
    return page;
  });
}
```

Plugins can register filters (`{{ value | filterName }}`), shortcodes (`{% shortcodeName "arg" %}`), and lifecycle hooks. Plugin filters override built-in filters with the same name — last loaded wins.

## External Data Sources

Fetch data from APIs at build time. Fetched data merges into `site.data.*` — templates can't tell the difference between local files and fetched data.

```yaml
sources:
  posts:
    type: "rest"
    url: "https://api.example.com/posts.json"
    cache: 3600
    as: "posts"          # Available as {{ site.data.posts }}

  products:
    type: "graphql"
    endpoint: "https://api.example.com/graphql"
    query: "{ products { id, name, price, slug } }"
    cache: 1800
    as: "products"

  blog:
    type: "plugin"       # Plugin owns auth, pagination, everything
    plugin: "cms-posts"
    cache: 3600
    as: "blog"
```

Cached to `.alloy/fetch-cache/` on disk. Use `--refetch` to bypass the cache.

## Lifecycle Hooks

Plugins can hook into 12 lifecycle events. Hooks chain in alphabetical filename order — each receives the previous hook's output.

| Hook | When | Mutable? |
|------|------|----------|
| `onConfig` | After config load | Yes |
| `onBeforeValidation` | Before output path conflict check | Yes (add paths) |
| `onAfterValidation` | After validation passes | Cascade only |
| `onDataFetched` | After external data fetch | Yes |
| `onDataCascadeReady` | Cascade fully resolved | Yes |
| `onContentLoaded` | After content discovery | Yes |
| `onContentTransformed` | After Markdown rendering (per page, receives `{ html, toc, path, url, frontMatter }`) | Yes |
| `onPageRendered` | After layout rendering (per page, receives HTML string) | Yes |
| `onAssetProcess` | Per-asset processing (receives `{ path, content }`) | Yes |
| `onBuildComplete` | Build finished | No |
| `onDevServerStart` | Dev server ready | No |
| `onFileChanged` | File changed in watch mode | No |

## Web Component SSR (Opt-In)

Alloy's two-phase rendering separates content rendering from component SSR. Phase 1 produces intermediate HTML with raw component tags. Phase 2 (if configured) extracts the `<body>` inner content and pipes it to an external SSR engine via stdin. The engine handles all component rendering internally — element discovery, deduplication, shadow root rendering, and Declarative Shadow DOM injection — and returns the transformed body content via stdout. Alloy re-inserts the result into the original document skeleton, preserving `<head>`, `<script>` tags, and other document structure.

Two communication modes:

```yaml
# Exec mode (default) — one process per page
ssr:
  command: "golit render --defs ./bundles"

# Stream mode — persistent process, NUL-delimited, amortized startup
ssr:
  command: "golit serve --stdio"
  mode: "stream"
  timeout: "30s"
```

Pages without custom elements skip SSR entirely. Failed pages preserve their original HTML — one bad page doesn't abort a 500-page build.

Compatible with any SSR engine that reads stdin and writes stdout. [golit](https://github.com/zeroedin/golit) is recommended.

## Dev Server

Two commands, same infrastructure:

- **`alloy dev`** — Dev mode. Phase 1 only (Liquid + Markdown), components render client-side. Pages held in memory, no disk writes. Static files served from source. Drafts visible. On file changes, only affected pages are rebuilt (incremental via content-hash cache).
- **`alloy serve`** — Production server. Full production pipeline (Phase 1 + Phase 2 SSR if configured). Writes to `_site/` and serves from disk. Drafts excluded. Same output as `alloy build`.

Both commands include WebSocket live reload, file watching with 50ms debounce, port auto-increment (tries up to 10 ports), custom 404 page support, and error overlay in the browser. Layout changes invalidate all pages using that layout. Config changes trigger a full rebuild.

## CLI

```
alloy init                  Create a new project config
alloy build                 Full clean build to _site/ (always rebuilds all pages)
alloy dev                   Dev server with live reload (incremental rebuilds, drafts visible)
alloy serve                 Production server (same pipeline as build, served locally)
alloy version               Print version
```

**Flags:**
```
--config, -c       Config file path (default: alloy.config.yaml)
--root, -r         Project root directory (default: config file's directory)
--output, -o       Output directory (default: _site)
--port, -p         Server port (default: 3000, auto-increments if occupied)
--verbose, -v      Verbose logging
--quiet, -q        Suppress output
--no-drafts        Hide drafts in dev mode (alloy dev only)
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

## Development

### Prerequisites

- **Go 1.25+** — `go version`
- **Node.js** (optional) — only needed for Tier 3 plugins. Not required to build or test Alloy itself.

### Build

```bash
# Clone and build
git clone https://github.com/zeroedin/alloy.git
cd alloy
go build -o alloy .

# Verify
./alloy version
```

### Test

```bash
# Run all tests
go test ./...

# Run a specific package
go test ./internal/template/...
go test ./internal/pipeline/...
go test ./test/integration/...

# Verbose output
go test ./... -v
```

Tests use [Ginkgo](https://onsi.github.io/ginkgo/) + [Gomega](https://onsi.github.io/gomega/). Tests are spec-derived and immutable — they define expected behavior. If a test fails, the implementation must change, not the test.

### Project Layout

```
cmd/                    CLI commands (init, build, serve)
internal/
  cache/                Build cache for incremental rebuilds
  cascade/              Data cascade (5-level merge)
  collection/           Collections and taxonomies
  config/               Config loading (YAML, TOML, JSON)
  content/              Content discovery, front matter, markdown
  i18n/                 Multilingual support
  output/               Output writing, sitemap, feeds
  pagination/           Pagination and virtual pages
  permalink/            URL generation
  pipeline/             Build pipeline orchestration
  plugin/               Plugin system (QuickJS, WASM, Node)
  server/               Dev server, file watcher, WebSocket
  ssr/                  Web Component SSR (Phase 2)
  static/               Static file and passthrough copy
  template/             Liquid + Go template engines, filters
test/
  fixtures/             Test site fixtures
  integration/          Cross-package integration tests
plans/
  PLAN.md               Specification
  IMPLEMENTATION.md     Implementation guide
```

### Architecture

The spec lives in `plans/PLAN.md`. The implementation guide is `plans/IMPLEMENTATION.md`. Both are the source of truth — tests encode the spec, implementation must conform to tests.

**Workflow**: Spec changes → tests → implementation. Tests are written first (red), implementation makes them green. Tests are never modified to pass.

## License

[MIT](LICENSE)
