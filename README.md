<img src="docs/static/images/alloy-logo.svg" alt="Alloy" width="120">
  
# Alloy

A fast, extensible static site generator. Built in Go.

## Why Alloy?

Static site generators tend to make you choose between speed and extensibility. Alloy doesn't. It compiles thousands of pages in seconds, uses Liquid templates for a familiar developer experience, and supports a tiered plugin system — from in-process WASM to a full Node.js bridge — without sacrificing build performance.

## Features

- **Fast builds** — Go-powered concurrent pipeline targeting < 5s for 1,000 pages
- **Liquid templates** — Familiar syntax with Go `html/template` as an alternative
- **Data cascade** — Global data, directory-level `_data.yaml`, and front matter merge predictably (6 levels, last wins)
- **Collections and taxonomies** — Date-based section collections, cross-cutting taxonomy groups, auto-generated term pages
- **Pagination** — Paginated lists and virtual page generation from data
- **Content lifecycle** — Draft/future/expired content with sensible defaults
- **Permalinks** — Fast token replacement with Liquid fallback for complex patterns
- **Multiple output formats** — HTML, JSON, XML, or anything else from a single content file
- **i18n** — Opt-in multilingual support with per-language content trees and shared layouts
- **Incremental rebuilds (dev mode)** — Content-hash change detection in `alloy dev` for fast rebuilds on file changes. `alloy build` always does a full clean rebuild for CI/CD reliability.
- **Tiered plugin system** — Built-in Go filters (ns), in-process JS/WASM plugins (us), Node subprocess plugins (ms)
- **Dev server** — File watching, WebSocket live reload, error reporting

## Quick Start

```bash
# Install via Homebrew (macOS / Linux)
brew tap zeroedin/alloy-ssg
brew install alloy-ssg

# Scaffold a new project
alloy init my-site && cd my-site

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

See [Project Structure](https://alloyssg.dev/getting-started/project-structure/) for details.

## Configuration

All site settings live in `alloy.config.yaml` (TOML and JSON also supported). Directory paths, build output, template engine, taxonomies, pagination, and passthrough mappings are all configurable. See [Configuration](https://alloyssg.dev/getting-started/) for the full reference.

## Content

Content files live in `content/` with YAML, TOML, or JSON front matter. Template tags (`{{ }}` and `{% %}`) work directly in Markdown. See [Content](https://alloyssg.dev/content/) for details.

## Templates

Alloy supports Liquid (default) and Go `html/template` as built-in engines. Both share the same layout lookup order, data context, and built-in filters. See [Templates](https://alloyssg.dev/templates/) for details.

## Data Cascade

Six levels of data merge predictably (last wins): global data, directory data, front matter, and three plugin hook stages. See [Data Cascade](https://alloyssg.dev/content/data-cascade/) for the full merge order.

## Plugins

Three tiers, matched to what the plugin needs:

| Tier | Runtime | Latency | Use case |
|------|---------|---------|----------|
| 1 | Go built-in | ~ns | String, date, array, URL, math filters |
| 2 | JS (QuickJS) or WASM via wazero | ~us | Custom filters, shortcodes, data transforms |
| 3 | Node subprocess | ~ms | PostCSS, Sharp, Lit SSR, npm packages |

Drop a `.js` file in `plugins/` and it runs on embedded QuickJS. Export `runtime: "node"` for Node. Drop a `.wasm` file for compiled plugins. No config needed. See [Plugins](https://alloyssg.dev/plugins/) for details.

## Lifecycle Hooks

Plugins can hook into 13 lifecycle events covering config, content rendering, asset processing, and build completion. Hooks chain in alphabetical filename order. See [Lifecycle Events](https://alloyssg.dev/hooks/) for the full list and payloads.

## CLI

```
alloy init                  Scaffold a new project
alloy build                 Full clean build to _site/
alloy dev                   Dev server with live reload (incremental rebuilds, drafts visible)
alloy serve                 Production server (same pipeline as build, served locally)
alloy version               Print version
```

See [CLI Reference](https://alloyssg.dev/cli/) for all flags and build modes.

## Dependencies

| Package | Purpose | License |
|---------|---------|---------|
| [liquidgo](https://github.com/Notifuse/liquidgo) | Liquid template engine | MIT |
| [goldmark](https://github.com/yuin/goldmark) | Markdown rendering | MIT |
| [cobra](https://github.com/spf13/cobra) | CLI framework | Apache-2.0 |
| [fsnotify](https://github.com/fsnotify/fsnotify) | File watching | BSD-3 |
| [wazero](https://github.com/tetratelabs/wazero) | WASM runtime (plugins) | Apache-2.0 |
| [gorilla/websocket](https://github.com/gorilla/websocket) | Live reload | BSD-3 |
| [sonic](https://github.com/bytedance/sonic) | High-performance JSON | Apache-2.0 |
| [qjs](https://github.com/nicholasgasior/goquickjs) | QuickJS binding (Tier 2 JS plugins) | MIT |
| [toml](https://github.com/BurntSushi/toml) | TOML config/front matter parsing | MIT |
| [doublestar](https://github.com/bmatcuk/doublestar) | Glob pattern matching | MIT |
| [strftime](https://github.com/lestrrat-go/strftime) | Date formatting | MIT |

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
cmd/                    CLI commands (init, build, dev, serve)
internal/
  assets/               Asset processing hooks
  cache/                Build cache for incremental rebuilds
  cascade/              Data cascade (6-level merge)
  collection/           Collections and taxonomies
  config/               Config loading (YAML, TOML, JSON)
  content/              Content discovery, front matter, markdown
  data/                 Data file loading (YAML, JSON, TOML, CSV)
  fetch/                External data source fetching (REST, GraphQL)
  fileutil/             File utility helpers
  i18n/                 Multilingual support
  jsonutil/             JSON utilities (sonic-backed)
  ordered/              Ordered map for key-order preservation
  output/               Output writing, sitemap, feeds
  pagination/           Pagination and virtual pages
  permalink/            URL generation
  pipeline/             Build pipeline orchestration
  plugin/               Plugin system (QuickJS, WASM, Node)
  server/               Dev server, file watcher, WebSocket
  ssr/                  Web Component SSR (Phase 2)
  static/               Static file and passthrough copy
  template/             Liquid + Go template engines, filters
  validation/           Output path conflict detection
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
