---
layout: doc
title: Configuration
nav_weight: 15
description: "All alloy.config.yaml options with types, defaults, and examples."
---

Alloy reads its configuration from `alloy.config.yaml` (also `.yml`, `.toml`, or `.json`) in the project root. The first match wins.

```yaml
# alloy.config.yaml
title: "My Site"
baseURL: "https://example.com"
language: "en"
```

Both `title` and `baseURL` are required. Other fields default to sensible values when omitted.

## Top-level fields

| Field | Type | Default | Description |
|---|---|---|---|
| `title` | string | — (required) | Site title. Available in templates as `site.title`. |
| `baseURL` | string | — (required) | Full URL including protocol. Used by `absolute_url` filter and sitemap generation. |
| `language` | string | `"en"` | Default site language code. Available as `site.language`. |
| `updateCheck` | boolean | `false` | Check for newer Alloy versions when `alloy dev` or `alloy serve` starts. See [CLI Reference](/cli/#passive-update-notifications). |

## `build`

Controls output directory and cleanup behavior.

```yaml
build:
  output: "_site"
  clean: true
```

| Field | Type | Default | Description |
|---|---|---|---|
| `build.output` | string | `"_site"` | Output directory for the built site. |
| `build.clean` | boolean | `true` | Delete the output directory before each build. Set `false` to preserve files from previous builds. |

## `content`

Controls which files are treated as content and how Markdown is rendered.

```yaml
content:
  formats: ["md", "html"]
  markdown:
    toc: true
    goldmark:
      unsafe: true
      typographer: false
      templateTags: true
      autoHeadingID: true
      customElements: true
```

| Field | Type | Default | Description |
|---|---|---|---|
| `content.formats` | string[] | `["md", "html"]` | File extensions treated as content pages. Other files in `content/` are copied as passthrough. |
| `content.markdown.toc` | boolean | `true` | Generate `page.toc` heading structure for Markdown pages. Set `false` to disable. |
| `content.markdown.goldmark.unsafe` | boolean | `true` | Allow raw HTML in Markdown. Set `false` to escape all HTML tags. |
| `content.markdown.goldmark.typographer` | boolean | `false` | Convert straight quotes to smart quotes, `--` to en-dash, `---` to em-dash. |
| `content.markdown.goldmark.templateTags` | boolean | `true` | Treat `{{ }}` and `{% %}` in Markdown prose as template syntax. Set `false` to render them as literal text. |
| `content.markdown.goldmark.autoHeadingID` | boolean | `true` | Add `id` attributes to headings. Also activates `{.class #id key=value}` block-level attribute syntax. |
| `content.markdown.goldmark.customElements` | boolean | `true` | Treat multi-line custom elements (tags with hyphens) as block-level HTML. Preserves content inside verbatim. |

## `templates`

```yaml
templates:
  engine: "liquid"
```

| Field | Type | Default | Description |
|---|---|---|---|
| `templates.engine` | string | `"liquid"` | Template engine. `"liquid"` or `"gotemplate"` (alias `"go"`). One engine per project. |

## `structure`

Override default directory names. All paths are relative to the project root.

```yaml
structure:
  content: "content"
  layouts: "layouts"
  assets: "assets"
  static: "static"
  data: "data"
  plugins: "plugins"
```

| Field | Type | Default | Description |
|---|---|---|---|
| `structure.content` | string | `"content"` | Content files directory. |
| `structure.layouts` | string | `"layouts"` | Template layouts directory. |
| `structure.assets` | string | `"assets"` | Processed assets directory. |
| `structure.static` | string | `"static"` | Static files directory (copied verbatim to output root). |
| `structure.data` | string | `"data"` | Global data files directory. |
| `structure.plugins` | string | `"plugins"` | Plugin files directory. |

## `plugins`

```yaml
plugins:
  timeout: 5000
  workers: "auto"
```

| Field | Type | Default | Description |
|---|---|---|---|
| `plugins.timeout` | integer | `5000` | Hook timeout in milliseconds. Alloy discards a timed-out hook's mutations. |
| `plugins.workers` | integer or `"auto"` | `"auto"` | Node subprocess worker count for per-page hooks. `"auto"` uses `min(CPU_count / 2, 8)` with a floor of 2. |

## `taxonomies`

Declare which front matter keys create taxonomy collections. Each key can be a bare declaration or an object with options.

```yaml
taxonomies:
  tags:
  categories:
    permalink: "/sections/:slug/"
    layout: "term"
    render: false
```

| Field | Type | Default | Description |
|---|---|---|---|
| `taxonomies.<name>.permalink` | string | `"/<name>/:slug/"` | URL pattern for term pages. |
| `taxonomies.<name>.layout` | string | taxonomy name | Layout template for index and term pages. |
| `taxonomies.<name>.render` | boolean | `true` | Generate output pages. Set `false` for data-only taxonomies. |

See [Taxonomies](/collections/taxonomies/) for usage.

## `collections`

Declare sections as collections without date-based permalink patterns.

```yaml
collections:
  releases:
    sortBy: "date"
    order: "desc"
```

| Field | Type | Default | Description |
|---|---|---|---|
| `collections.<name>.sortBy` | string | `"date"` | Front matter field to sort by. |
| `collections.<name>.order` | string | `"desc"` | Sort direction. `"asc"` or `"desc"`. |

See [Collections](/collections/) for usage.

## `pagination`

```yaml
pagination:
  path: "page"
```

| Field | Type | Default | Description |
|---|---|---|---|
| `pagination.path` | string | `"page"` | URL segment for paginated pages (e.g., `/articles/page/2/`). |

See [Pagination](/content/pagination/) for usage.

## `data`

Map external files into the `site.data.*` namespace.

```yaml
data:
  files:
    tokens: "node_modules/@rhds/tokens/json/rhds.tokens.json"
    cem: "../custom-elements.json"
```

| Field | Type | Default | Description |
|---|---|---|---|
| `data.files.<key>` | string | — | Path to an external data file (YAML, JSON, or TOML). Relative to project root. Available as `site.data.<key>`. |

See [Data Files](/content/data-files/) for formats and external sources.

## `sources`

Fetch data from REST APIs, GraphQL endpoints, or plugin handlers at build time.

```yaml
sources:
  posts:
    type: "rest"
    url: "https://api.example.com/posts.json"
    cache: 3600
    as: "posts"

  products:
    type: "graphql"
    endpoint: "https://api.example.com/graphql"
    query: |
      { products { id, name, price } }
    cache: 1800
    as: "products"

  blog:
    type: "plugin"
    plugin: "cms-posts"
    cache: 3600
    as: "blog"
```

| Field | Type | Default | Description |
|---|---|---|---|
| `sources.<name>.type` | string | — | `"rest"`, `"graphql"`, or `"plugin"`. |
| `sources.<name>.url` | string | — | Endpoint URL (rest). |
| `sources.<name>.endpoint` | string | — | GraphQL endpoint URL. |
| `sources.<name>.query` | string | — | GraphQL query string. |
| `sources.<name>.plugin` | string | — | Plugin source handler name (for `type: "plugin"`). |
| `sources.<name>.cache` | integer | — | Cache TTL in seconds. Cached to `.alloy/fetch-cache/`. |
| `sources.<name>.as` | string | source key | `site.data.*` key for the fetched data. Defaults to the source map key. |

See [Data Files — External data sources](/content/data-files/#external-data-sources) for usage.

## `passthrough`

Copy external directories or glob patterns into the output.

```yaml
passthrough:
  - from: "../design-system/dist/elements"
    to: "elements"
  - from: "fonts/**/*.woff2"
    to: "assets/fonts"
    exclude:
      - "*.map"
```

| Field | Type | Default | Description |
|---|---|---|---|
| `passthrough[].from` | string | — | Source path or glob pattern. Relative to project root. |
| `passthrough[].to` | string | — | Destination path in the output directory. `"."` targets the output root. |
| `passthrough[].exclude` | string[] | — | Glob patterns to skip. |

## `watch`

Register external directories for file watching during `alloy dev` and `alloy serve`. Changes trigger the appropriate pipeline stage.

```yaml
watch:
  - from: "../design-system/elements"
    type: "content"
  - from: "../shared-layouts"
    type: "layout"
```

| Field | Type | Default | Description |
|---|---|---|---|
| `watch[].from` | string | — | Directory to watch. Cannot overlap `structure.*` directories. |
| `watch[].type` | string | — | Pipeline stage to trigger: `"content"`, `"layout"`, or `"data"`. |

## `sitemap`

Alloy generates `sitemap.xml` by default. Disable it or set global defaults.

```yaml
# Object form — set defaults
sitemap:
  changefreq: "weekly"
  priority: 0.5

# Boolean form — disable entirely
sitemap: false
```

| Field | Type | Default | Description |
|---|---|---|---|
| `sitemap` | boolean or object | `true` | Set `false` to disable sitemap generation. |
| `sitemap.changefreq` | string | — | Default `<changefreq>` for all pages. |
| `sitemap.priority` | float | — | Default `<priority>` for all pages (0.0–1.0). |

Per-page overrides in front matter: `sitemap: { priority: 1.0 }` or `sitemap: false` to exclude a page.

## `languages`

Opt-in multilingual support. Each key is a language code with its own content tree under `content/<code>/`.

```yaml
languages:
  en:
    title: "My Site"
    weight: 1
    root: true
    strings:
      read_more: "Read more"
  fr:
    title: "Mon Site"
    weight: 2
    strings:
      read_more: "Lire la suite"
```

| Field | Type | Default | Description |
|---|---|---|---|
| `languages.<code>.title` | string | — | Language-specific site title. Overrides top-level `title`. |
| `languages.<code>.weight` | integer | — | Sort order. Lowest weight is the default language. |
| `languages.<code>.root` | boolean | `false` | Output at site root (`_site/`) instead of `_site/<code>/`. |
| `languages.<code>.strings` | map | — | Translation strings for shared layouts. Access via `site.language.strings.<key>`. |

See [Internationalization](/advanced/i18n/) for content structure and template usage.

## `ssr`

Experimental server-side rendering. See [SSR](/experimental/ssr/) for details.

```yaml
ssr:
  command: "node ssr-worker.js"
  mode: "full"
  timeout: "10s"
```

| Field | Type | Default | Description |
|---|---|---|---|
| `ssr.command` | string | — | SSR worker command. |
| `ssr.mode` | string | — | Rendering mode. |
| `ssr.timeout` | string | — | Per-page timeout (Go duration string, e.g., `"10s"`). |

## Full example

```yaml
title: "My Site"
baseURL: "https://example.com"
language: "en"
updateCheck: true

build:
  output: "_site"
  clean: true

content:
  formats: ["md", "html"]
  markdown:
    toc: true
    goldmark:
      unsafe: true
      typographer: true
      templateTags: true
      autoHeadingID: true
      customElements: true

templates:
  engine: "liquid"

structure:
  content: "content"
  layouts: "layouts"

plugins:
  timeout: 5000
  workers: "auto"

taxonomies:
  tags:
  categories:

pagination:
  path: "page"

sitemap:
  changefreq: "weekly"
  priority: 0.5

passthrough:
  - from: "node_modules/@rhds/elements/elements"
    to: "assets/elements"
    exclude: ["*.map", "demo/"]

sources:
  team:
    type: "rest"
    url: "https://api.example.com/team.json"
    cache: 3600
    as: "team"
```
