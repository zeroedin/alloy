---
title: "Configuration"
layout: "doc"
weight: 1
section: "configuration"
description: "Alloy configuration reference — all options for alloy.config.yaml."
---

## Config File

Alloy looks for `alloy.config.yaml`, `alloy.config.toml`, or `alloy.config.json` in the project root. YAML is recommended.

```yaml
title: "My Site"
baseURL: "https://example.com"

build:
  output: "_site"
  clean: true

content:
  formats: ["md", "html"]
  markdown:
    goldmark:
      unsafe: true
      typographer: true
      templateTags: true

templates:
  engine: "liquid"       # "liquid" (default) or "go"

plugins:
  node: true             # Enable Node bridge for Tier 3 plugins
  timeout: 5000          # Plugin timeout in ms

structure:
  content: "content"
  layouts: "layouts"
  assets: "assets"
  static: "static"
  data: "data"

permalinks:
  blog: "/:year/:month/:slug/"
  default: "/:slug/"

taxonomies:
  tags:
  categories:

pagination:
  path: "page"

passthrough:
  - from: "../design-system/dist/elements"
    to: "elements"

ssr:
  command: "golit render --defs ./bundles"
  mode: "stream"          # "exec" (default) or "stream"
  timeout: "30s"

sources:
  posts:
    type: "rest"
    url: "https://api.example.com/posts.json"
    cache: 3600
    as: "posts"

languages:
  en:
    title: "My Site"
    weight: 1
    root: true
  fr:
    title: "Mon Site"
    weight: 2
```

## Required Fields

- **`title`** — Site title. Used in templates as `site.title`.
- **`baseURL`** — Production URL. Used by the `url` and `absolute_url` filters.

## Sections

Each configuration block is documented in detail:

- [Build](/configuration/build/) — Output directory, clean builds
- [Content](/configuration/content/) — Formats, Markdown options
- [Templates](/configuration/templates/) — Engine selection
- [Permalinks](/configuration/permalinks/) — URL patterns and tokens
- [Structure](/configuration/structure/) — Custom directory paths
- [Passthrough](/configuration/passthrough/) — External directory mapping
- [i18n](/configuration/i18n/) — Multilingual support
- [SSR](/configuration/ssr/) — Web Component server-side rendering
