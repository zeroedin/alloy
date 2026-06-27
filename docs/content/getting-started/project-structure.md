---
layout: doc
title: Project Structure
nav_weight: 30
description: "The standard directory layout for an Alloy project: config, content, layouts, assets, static, and data."
---

A standard Alloy project follows this directory layout:

```
my-site/
├── alloy.config.yaml          # Site config (YAML, TOML, or JSON)
├── content/                   # Content files (Markdown, HTML)
│   ├── index.md               # Site root page -> /
│   ├── about.html             # Standalone page -> /about/
│   └── blog/
│       ├── _data.yaml         # Directory-level data (cascades down)
│       ├── index.md           # Blog landing -> /blog/
│       └── first-post.md      # Blog post -> /blog/first-post/
├── layouts/                   # Liquid templates
│   ├── default.liquid         # Fallback layout
│   ├── post.liquid            # Blog post layout
│   └── partials/              # Reusable template fragments
│       ├── header.liquid
│       └── footer.liquid
├── assets/                    # Processed assets (CSS, JS, images)
├── static/                    # Copied as-is to output
├── data/                      # Global data files (YAML, JSON, CSV)
│   └── navigation.yaml
└── plugins/                   # Optional plugins
```

## Config file

Alloy detects config files by extension in this order: `alloy.config.yaml`, `alloy.config.yml`, `alloy.config.toml`, `alloy.config.json`. The first match wins.

```yaml
# alloy.config.yaml
title: "My Site"
baseURL: "https://example.com"
language: "en"

build:
  output: "_site"
  clean: true

content:
  formats: ["md", "html"]
  markdown:
    goldmark:
      unsafe: true
      typographer: true
      templateTags: true     # false = treat {{ }} and {% %} as literal text

templates:
  engine: "liquid"

taxonomies:
  tags:
  categories:
```

Front matter supports three formats, detected by delimiter:

| Delimiter | Format | Example |
|---|---|---|
| `---` | YAML | `---`  `title: "My Post"`  `---` |
| `+++` | TOML | `+++`  `title = "My Post"`  `+++` |
| `{` | JSON | `{ "title": "My Post" }` |

## `content/`

Content files are Markdown or HTML with front matter. Front matter is required on all content files -- empty front matter (`---\n---`) is valid, but missing delimiters is a build error.

Files whose extension does not match `content.formats` (default: `["md", "html"]`) are treated as passthrough -- copied directly to output without processing. This lets you colocate assets with content:

```
content/about/
├── index.md              # content (processed)
├── hero.png              # passthrough (copied to _site/about/hero.png)
└── diagram.svg           # passthrough (copied to _site/about/diagram.svg)
```

HTML files without front matter are classified based on content: full documents (starting with `<!DOCTYPE` or `<html>`) become passthrough; fragments become content pages with empty front matter, inheriting layout from the `_data.yaml` cascade.

## `layouts/`

Template files rendered by the configured engine. The Liquid engine looks for `.liquid` files first, then falls back to bare extensions. The Go template engine uses bare extensions (`.html`) directly.

Layout lookup follows an explicit chain:

1. `layout:` from front matter or `_data.yaml` cascade
2. Section or filename match (e.g., `post.liquid` for blog children)
3. `default.liquid` fallback
4. Build error if nothing matches

Layouts can chain via front matter `layout:` directives, enabling multi-level composition:

```liquid
<!-- layouts/has-toc.liquid -->
---
layout: "base"
---
<div class="with-toc">
  <aside>{% include "partials/toc" %}</aside>
  <main>{{ content }}</main>
</div>
```

## `data/`

Global data files accessible in templates as `site.data.*`. Supports YAML, TOML, JSON, and CSV. The filename (without extension) becomes the key:

```
data/
├── navigation.yaml    # -> site.data.navigation
├── authors.json       # -> site.data.authors
└── team.csv           # -> site.data.team (array of maps)
```

Two files with the same stem name (e.g., `team.csv` and `team.yaml`) cause a build error -- no silent overwrites.

External data files can be mapped into the data namespace via config:

```yaml
data:
  files:
    tokens: "node_modules/@rhds/tokens/json/rhds.tokens.json"
    cem: "../custom-elements.json"
```

These are accessible as `site.data.tokens` and `site.data.cem`, identical to local data files.

## `_data.yaml`

Directory-level data files that cascade to all descendant content. Each `_data.yaml` deep-merges with its parent's data. Common uses include setting a shared layout, permalink pattern, or default tags for an entire section:

```yaml
# content/blog/_data.yaml
layout: post
permalink: "/blog/:year/:month/:slug/"
tags: ["blog"]
```

Every page in `content/blog/` and its subdirectories inherits these values unless overridden in front matter.

## `static/`

Files copied verbatim to the output root with no processing:

```
static/
├── favicon.ico          # -> _site/favicon.ico
├── robots.txt           # -> _site/robots.txt
└── images/
    └── logo.png         # -> _site/images/logo.png
```

## `assets/`

Processed assets. Copied to output and available for plugin hooks like `onAssetProcess`.

## `plugins/`

Optional directory for plugin files. Alloy detects the plugin tier from the file:

- `.js` files without `runtime: "node"` run on embedded QuickJS (Tier 2)
- `.wasm` files run as compiled WASM via wazero (Tier 2)
- `.js` files with `runtime: "node"` run as Node subprocess (Tier 3)

The Node bridge is only spawned when the project has Tier 3 plugins.

## Custom directory structure

Override default paths with the `structure:` config key. All paths are relative to the project root:

```yaml
# alloy.config.yaml -- monorepo example
structure:
  content: "./docs/pages/"
  layouts: "./docs/layouts/"
  assets: "./docs/assets/"
  static: "./docs/static/"
  data: "./data/"
  plugins: "./tools/plugins/"
```

The pipeline, file watcher, and dev server all use the configured paths. When a key is omitted, it defaults to its standard name.

## Passthrough copy

Map external directories into the output via config:

```yaml
passthrough:
  - from: "../design-system/dist/elements"
    to: "elements"
  - from: "../shared-assets/fonts"
    to: "assets/fonts"
```

Passthrough supports glob patterns and exclude filters:

```yaml
passthrough:
  - from: "elements/**/*.{js,css}"
    to: "assets/packages/elements"
    exclude:
      - "*.html"
      - "demo/"
      - "**/*.map"
```

See also [CLI Reference](/cli/) for commands and flags.
