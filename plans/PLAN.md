# Alloy — Static Site Generator Specification

## Context

Alloy is a new static site generator that combines Hugo's raw speed with 11ty's extensibility. The name reflects the idea: an alloy of two great tools into something stronger than either.

**Problem**: Hugo is blazing fast but rigid — Go templates are unfriendly and extensibility requires rebuilding the binary. 11ty is beautifully extensible but JavaScript-based, making it slow for large sites. No SSG today offers both fast builds for thousands of pages AND a rich plugin ecosystem.

**Goal**: Build a Go-based SSG that compiles thousands of pages in seconds, uses Liquid templates for familiar DX, and supports a Node.js plugin bridge for extensibility — without sacrificing performance.

---

## Core Architecture

```
┌─────────────────────────────────────────────────┐
│                   Alloy CLI                      │
│              (single Go binary)                  │
├─────────────────────────────────────────────────┤
│  Config Loader → Content Discovery → Data Cascade│
│       ↓               ↓                ↓         │
│  Front Matter    File Watcher     Global Data    │
│  Extraction      (dev mode)       Merge          │
├─────────────────────────────────────────────────┤
│              Build Pipeline                      │
│  ┌─────┐  ┌──────┐  ┌────────┐  ┌───────────┐  │
│  │Parse│→ │Transform│→│Template│→ │Write Output│ │
│  │Content│ │(plugins)│ │Render │  │   Files   │  │
│  └─────┘  └──────┘  └────────┘  └───────────┘  │
├─────────────────────────────────────────────────┤
│  Template Engine          │  Assets              │
│  (Notifuse/liquidgo)      │  (copy, plugin hooks) │
├───────────────────────────┤──────────────────────┤
│          Node Bridge (opt-in, IPC)               │
│     Event hooks for JS/TS plugin execution       │
└─────────────────────────────────────────────────┘
```

### Language & Runtime
- **Core**: Go (single binary, no runtime deps). Use the latest stable Go version, provided all dependencies are compatible.
- **Template engine**: Notifuse/liquidgo (Liquid syntax, MIT, zero deps, parse-once/render-many)
- **Node bridge**: Opt-in. Only spawned if the project has JS/TS plugins. Communicates via IPC (length-prefixed JSON-RPC over stdin/stdout).

---

## 1. Project Structure

A user's Alloy project:

```
my-site/
├── alloy.config.yaml          # Site config (YAML, TOML, or JSON)
├── content/                   # Content files (Markdown, HTML, txt)
│   ├── index.md               # Site root page → /
│   ├── about.html             # Standalone page → /about/
│   └── blog/
│       ├── _data.yaml         # Directory-level data (cascades down)
│       ├── index.md           # Blog landing page → /blog/
│       ├── first-post.md      # Blog post → /blog/first-post/
│       └── second-post/       # Page bundle (co-located assets)
│           ├── index.md       # → /blog/second-post/
│           └── hero.jpg
├── layouts/                   # Template files (Liquid)
│   ├── default.liquid         # Fallback layout for all pages
│   ├── blog.liquid            # Blog index layout (matches section name)
│   ├── post.liquid            # Blog post layout (child of date-based section)
│   └── partials/              # Reusable template fragments
│       ├── header.liquid
│       └── footer.liquid
├── assets/                    # Processed assets (CSS, JS, images)
│   ├── css/
│   ├── js/
│   └── img/
├── static/                    # Copied as-is to output
├── data/                      # Global data files (YAML, JSON, CSV)
│   └── navigation.yaml
└── plugins/                   # Plugins (opt-in)
    ├── word-count.js          # JS plugin — runs on embedded QuickJS (Tier 2)
    ├── custom-slugify.wasm    # WASM plugin — compiled from Rust/AS/Go (Tier 2)
    └── css-minifier.js        # Node plugin — exports runtime: "node" (Tier 3)
```

### Config File (`alloy.config.yaml`)
```yaml
title: "My Site"
baseURL: "https://example.com"
language: "en"

build:
  output: "_site"              # Output directory
  clean: true                  # Clean output before build

content:
  formats: ["md", "html"]     # Enabled content formats (default)
  markdown:
    goldmark:                  # Goldmark options
      unsafe: true             # Required: pass through raw HTML blocks
      typographer: true
      templateTags: true       # Auto-detect and preserve {{ }}/{% %} in Markdown (default: true)

templates:
  engine: "liquid"             # "liquid" (default) or "go" (Go html/template)

plugins:
  node: true                   # Enable Node bridge
  timeout: 5000                # Plugin timeout (ms)

structure:                       # Override default directory paths (all relative to project root)
  content: "content"             # Default: "content"
  layouts: "layouts"             # Default: "layouts"
  assets: "assets"               # Default: "assets"
  static: "static"               # Default: "static"
  data: "data"                   # Default: "data"

taxonomies:
  tags:                          # auto-generates /tags/ and /tags/:slug/
  categories:                    # auto-generates /categories/ and /categories/:slug/

pagination:
  path: "page"                 # Paginated URL segment (default: "page" → /blog/page/2/)

passthrough:
  - from: "../design-system/dist/elements"
    to: "elements"
  - from: "../shared-assets/fonts"
    to: "assets/fonts"
```

### Custom Directory Structure

The default project structure uses `content/`, `layouts/`, `assets/`, `static/`, and `data/` at the project root. The `structure:` config overrides these paths for projects with non-standard layouts:

```yaml
# alloy.config.yaml — monorepo example
structure:
  content: "./docs/pages/"
  layouts: "./docs/layouts/"
  assets: "./docs/assets/"
  static: "./docs/static/"
  data: "./data/"

passthrough:
  - from: "./elements/dist/"
    to: "assets/packages/elements"
```

For a project structured like:

```
my-monorepo/
├── alloy.config.yaml
├── data/
│   └── navigation.yaml
├── docs/
│   ├── pages/              ← content
│   ├── layouts/            ← layouts
│   ├── assets/             ← assets
│   └── static/             ← static
└── elements/
    ├── dist/               ← passthrough to output
    └── src/
```

All `structure:` paths are relative to the project root. When omitted, each directory defaults to its standard name (`content`, `layouts`, etc.). The pipeline, file watcher, and dev server all use the configured paths — no other changes needed.

### Supported Data Formats

Alloy supports YAML, TOML, and JSON wherever structured data is accepted:

**Config file** — detected by file extension:
- `alloy.config.yaml` or `alloy.config.yml`
- `alloy.config.toml`
- `alloy.config.json`

**Front matter** — detected by delimiter:

| Delimiter | Format | Example |
|---|---|---|
| `---` | YAML | `---`<br>`title: "My Post"`<br>`---` |
| `+++` | TOML | `+++`<br>`title = "My Post"`<br>`+++` |
| `{` | JSON | `{ "title": "My Post" }` |

**Front matter is required on content files only.** Files in the content directory whose extension matches `content.formats` (default: `["md", "html"]`) are content files — front matter delimiters are required. Empty front matter (`---\n---`) is valid (all fields default to nil/zero), but no delimiters at all is not. This avoids the edge-case bugs Hugo encounters with frontmatter-less files and ensures every page has an explicit metadata boundary. The error message should suggest adding empty front matter if the author has no metadata to set. Layouts, partials, data files, and other non-content files do not have or require front matter — they are templates and structured data, not content pages.

**Content-colocated passthrough** — Files in the content directory whose extension does NOT match `content.formats` are automatically copied to the output directory preserving their path relative to `content/`. No front matter check, no template processing, no Markdown rendering — raw file copy. This enables colocation of assets with content:

```
content/about/
├── index.md              ← content file (processed through pipeline)
├── diagram.svg           ← passthrough (copied to _site/about/diagram.svg)
├── hero.png              ← passthrough (copied to _site/about/hero.png)
├── page.html             ← has front matter → content file (processed)
├── standalone.html       ← NO front matter + <!DOCTYPE> → passthrough (full doc)
└── fragment.html         ← NO front matter + no DOCTYPE → content (fragment, wrapped by cascade layout)
```

**HTML front matter detection** — `.html` files matching `content.formats` are classified based on their content:

1. **Has front matter** (`---`, `+++`, `{`) → content page, processed normally
2. **No front matter + full HTML document** (starts with `<!DOCTYPE` or `<html>`) → passthrough, copied to output as-is
3. **No front matter + HTML fragment** (no DOCTYPE, no `<html>`) → content page with empty front matter. The file body is the page content, rendered into the cascade layout via `{{ content }}`. All metadata (layout, tags, etc.) comes from the `_data.yaml` cascade. **Note:** HTML fragments go through template processing — Liquid/Go template tags ARE evaluated.

**Engine-specific content extensions** — `content.formats` controls which extensions are eligible to be content, but `templates.engine` controls which template syntax Alloy can render. Both must agree:

- `templates.engine: "liquid"` + `liquid` in `content.formats` → `.liquid` files processed as content
- `templates.engine: "gotemplate"` → `.liquid` files are always passthrough, even if `liquid` is in `content.formats` (the Go template engine cannot render Liquid syntax)
- `.liquid` is NOT in the default `content.formats` list — with default settings, `.liquid` files in `content/` are passthrough

Fragments inherit layout from the cascade chain: `_data.yaml` `layout:` → filename match → `default.liquid` fallback. A `_data.yaml` with `layout: element` wraps every fragment in that directory with the element layout, producing full HTML documents in the output. `layout: false` in `_data.yaml` skips layout wrapping — the fragment passes through unwrapped.

```
content/patterns/card/
├── _data.yaml           ← layout: "element"
├── index.html           ← has front matter → content page
└── patterns/
    ├── themes.html      ← fragment → wrapped in element layout
    └── image.html       ← fragment → wrapped in element layout
```

`.md` files always require front matter — they are always content. A markdown file without front matter delimiters is a build error.

During content discovery, `DiscoverWithPassthrough` collects two lists: content pages (matching formats) and passthrough files (everything else). Excluded from passthrough: `_data.yaml`/`_data.yml` (cascade data), dot-prefixed files (`.DS_Store`, `.gitkeep`, etc.), and directories. The pipeline copies passthrough files to the output directory during Phase 3 (output writing), alongside static and passthrough-config files. In dev mode, passthrough files in `content/` are served directly from source (no copy needed).

**Data files** (`data/`, `_data.*`) — detected by file extension:
- `.yaml`, `.yml`
- `.toml`
- `.json`
- `.csv` (parsed as array of maps, header row = keys)

All formats parse into `map[string]any` — the rest of the pipeline (data cascade, template context, plugin hooks) is format-agnostic.

**Data file name collisions are build errors.** Data files are keyed by stem name (filename without extension), so `team.csv` and `team.yaml` both claim the key `"team"`. If two or more data files in the same directory share a stem name, the build fails:

```
[alloy] ERROR Data file conflict in data/:
        "team" is claimed by:
          1. team.csv
          2. team.yaml
        Resolve by renaming one file.
        Build aborted.
```

This follows the same principle as output path conflicts (§2): no silent overwrites, no priority system. The user must resolve collisions explicitly.

**External data files** — Files outside the `data/` directory can be mapped into the data namespace via config:

```yaml
data:
  files:
    cem: "../custom-elements.json"
    tokens: "node_modules/@rhds/tokens/json/rhds.tokens.json"
```

Each key becomes a `site.data.*` entry: `site.data.cem`, `site.data.tokens`. Paths are resolved relative to the project root. The file is parsed by extension (`.json`, `.yaml`, `.yml`, `.toml`, `.csv`) using the same parsers as `data/` directory files.

External data files are loaded alongside `data/` directory files during the data loading step — they share the same `site.data.*` namespace. Templates access external data identically to local data (`{{ site.data.cem.schemaVersion }}`). Moving a file between `data/` and external config is a config change, not a template change.

**Collision handling:** If an external file key matches a `data/` directory file stem (e.g., `cem` key in config AND `data/cem.json` on disk), the build fails with the same collision error as two `data/` files sharing a stem. The user controls key names — choose keys that don't conflict with filenames in `data/`.

External file not found is a build error — not a warning, not silently skipped. The config explicitly declares the file; if it doesn't exist, the build fails.

---

## 1b. Permalinks and URL Customization

### Permalink Resolution

Permalink patterns are **not configured at the site level**. There is no `permalinks:` key in `alloy.config.yaml`. URL patterns are section-level data that belongs in `_data.yaml` — sections own their own URL structure.

**Resolution order:**
1. Front matter `permalink:` (always wins)
2. `_data.yaml` cascade `permalink:` (section-level pattern)
3. `DefaultFromPath` — file path maps directly to URL (`content/about.md` → `/about/`)

**Section-level patterns via `_data.yaml`** — use token replacement for section-wide URL patterns:

```yaml
# content/blog/_data.yaml — all blog posts get date-based URLs
permalink: "/:year/:month/:slug/"
```

This cascades to all pages in `content/blog/` and subdirectories. A post at `content/blog/my-post.md` with `date: 2026-04-10` becomes `/2026/04/my-post/`. To include the section prefix: `permalink: "/blog/:year/:month/:slug/"` or `permalink: "/:section/:year/:month/:slug/"`.

**Available tokens:**

| Token | Value | Example |
|---|---|---|
| `:year` | Content date (4-digit) | `2026` |
| `:month` | Content date (2-digit) | `04` |
| `:day` | Content date (2-digit) | `10` |
| `:slug` | Slugified title or filename | `my-first-post` |
| `:title` | Raw title from front matter | `My First Post` |
| `:section` | Top-level content directory | `blog` |
| `:filename` | Source filename without extension | `my-first-post` |

**Front matter permalinks containing `{{ }}` trigger template rendering** — uses the configured template engine (Liquid or Go templates), ~50µs per page:

```yaml
---
title: "My Post"
# Liquid engine:
permalink: "/{{ page.customField | slugify }}/{{ page.date | date: '%Y' }}/"
# Go template engine:
# permalink: "/{{ .page.customField | slugify }}/{{ .page.date | date "%Y" }}/"
---
```

The permalink template syntax must match the configured engine. A Liquid permalink (`{{ page.slug }}`) in a Go template project will fail — and vice versa. Detection uses `{{` which is shared by both engines, but rendering uses the configured engine.

**Static front matter overrides** are also token-free fast path:

```yaml
---
permalink: "/custom/path/here/"     # Static, no rendering needed
slug: "custom-slug"                 # Override just the :slug token
---
```

Pages without a `permalink` in front matter or `_data.yaml` cascade use `DefaultFromPath` — the file path maps directly to the URL (`content/blog/my-post.md` → `/blog/my-post/`).

### Index Files

Index files (`index.md`, `index.html`) are directory landing pages and resolve to their parent directory path by default, **skipping** cascade permalink patterns:

- `content/index.md` → `/` (site root)
- `content/blog/index.md` → `/blog/` (section landing)
- `content/blog/second-post/index.md` → `/blog/second-post/` (page bundle)

This prevents a `_data.yaml` permalink pattern from turning `content/index.md` (title: "Home") into `/home/` instead of `/`.

**Front matter `permalink:` still overrides** — useful when the site is served from a subdirectory:
```yaml
---
title: "Home"
permalink: "/docs/"   # site lives at example.com/docs/
---
```

The lookup order for index files is:
1. Front matter `permalink:` (if set) — always honored
2. `DefaultFromPath` — strips `/index` suffix, returns directory path

Non-index files follow the full chain: front matter `permalink:` → `_data.yaml` cascade `permalink:` → `DefaultFromPath`.

**Performance:** 3000 pages with token replacement ≈ 1ms. Only pages with `{{ }}` in their permalink pay the Liquid rendering cost.

### `permalink: false`

Process the page (include it in collections, make its data available) but don't write an output file. Useful for data-only pages that feed into other pages via collections:

```yaml
---
title: "Shared Config"
permalink: false
layout: false
sharedData: "value available in collections but no output page"
---
```

### Aliases

A page can be output at multiple paths. Aliases are additional output locations for the same rendered content — not redirects.

```yaml
---
title: "About Us"
permalink: "/about/"
aliases:
  - /about-us/
  - /team/
---
```

This writes the same rendered HTML to `_site/about/index.html`, `_site/about-us/index.html`, and `_site/team/index.html`. Three identical files. No redirects, no meta refresh — Alloy doesn't know or care what server the site runs under.

Aliases are included in the pre-build validation pass. If an alias conflicts with another page's output path, the build fails with a conflict error.

---

## 1c. Pagination and Virtual Pages

### Pagination — Two Use Cases, One Mechanism

Pagination serves both paginated list pages AND virtual page generation. The difference is `perPage`:

- **`perPage` omitted or `1`** — one page per item (virtual pages). This is the default.
- **`perPage > 1`** — items are chunked into groups, one page per chunk (paginated list).

**Virtual pages from data** (default, `perPage: 1`):

```yaml
---
pagination:
  data: site.data.team          # From data/team.yaml
  as: member
permalink: "/team/{{ member.slug }}/"
---
<h1>{{ member.name }}</h1>
<p>{{ member.role }}</p>
```

One template + 20 team members in data → 20 output pages. No content files needed. `perPage` defaults to 1 — no need to specify it.

**Paginated list page** (`perPage > 1`):

```yaml
---
layout: article-list
pagination:
  data: collections.articles
  perPage: 10
  as: articles
permalink: "/articles/"
---
{% for article in articles %}
  <h2><a href="{{ article.url }}">{{ article.title }}</a></h2>
  <p>{{ article.summary }}</p>
{% endfor %}

{% if pagination.previousPage %}<a href="{{ pagination.previousPage }}">Previous</a>{% endif %}
{% if pagination.nextPage %}<a href="{{ pagination.nextPage }}">Next</a>{% endif %}
```

47 articles with `perPage: 10` → 5 pages:
- `/articles/` — articles 1-10 (first page, no segment)
- `/articles/page/2/` — articles 11-20
- `/articles/page/3/` — articles 21-30
- `/articles/page/4/` — articles 31-40
- `/articles/page/5/` — articles 41-47

Alloy appends the page segments automatically. The `permalink` defines the base URL. The page path segment is configured globally:

```yaml
# alloy.config.yaml
pagination:
  path: "page"           # default — /articles/page/2/, /articles/page/3/
```

The user can change the segment word (`path: "p"` → `/articles/p/2/`, `path: "seite"` → `/articles/seite/2/`). First page always outputs at the base permalink with no segment.

### Pagination Context

Templates with pagination receive a `pagination` object:

```liquid
{{ pagination.pageNumber }}    -- current page (1-based)
{{ pagination.totalPages }}    -- total page count
{{ pagination.previousPage }}  -- URL of previous page (nil if first)
{{ pagination.nextPage }}      -- URL of next page (nil if last)
{{ pagination.first }}         -- URL of first page
{{ pagination.last }}          -- URL of last page
{{ pagination.items }}         -- items on current page (aliased via 'as' key)
```

### Front Matter Interpolation

When generating virtual pages (`perPage: 1`), string-valued front matter fields that contain template tags (`{{ }}` or `{% %}`) are interpolated using the pagination `as:` variable context. This allows dynamic page metadata:

```yaml
---
title: "{{ member.name }}"
heading: "About {{ member.name | upcase }}"
description: "Profile page for {{ member.name }}"
layout: default
pagination:
  data: site.data.team
  perPage: 1
  as: member
permalink: "/team/{{ member.slug }}/"
---
```

For a team member `{name: "Alice", slug: "alice"}`, the virtual page gets:
- `page.title` → `"Alice"`
- `page.heading` → `"About ALICE"`
- `page.description` → `"Profile page for Alice"`
- `page.url` → `"/team/alice/"`

**Rules:**
- Only string-valued front matter fields are interpolated. Non-string values (numbers, booleans, arrays, maps) are left unchanged.
- Only fields containing `{{ }}` or `{% %}` markers are rendered through the template engine. Fields without markers skip the renderer entirely — no performance cost.
- Interpolation uses the same template engine and renderer as permalink processing. Full Liquid (or Go template) syntax is supported, including filters.
- The template context contains only the `as:` variable (e.g., `{member: item}`). `site.*`, `page.*`, and `collections.*` are not available during front matter interpolation — this runs at pagination time, before the full template context is built.
- Skipped fields: `permalink` (already processed), `layout`, `pagination`, and any key starting with `_` (internal transport keys such as `_paginationCtx`, `_paginationAs`, `_paginationData`).
- Only applies to virtual pages (`perPage: 1`). Paginated list pages (`perPage > 1`) do not interpolate front matter — the `as:` variable is a list, not a single item.

---

## 1d. Auto-Generated Output Files

### Built-in (no plugins needed)

**Sitemap** (`_site/sitemap.xml`):
Auto-generated from all published pages. Configurable in config:
```yaml
sitemap:
  changefreq: "weekly"
  priority: 0.5
```
Per-page override via front matter: `sitemap: { priority: 0.8 }` or `sitemap: false` to exclude a single page.

Disable sitemap generation entirely at the site level:
```yaml
sitemap: false
```

**RSS/Atom Feed** (`_site/feed.xml`):
Feeds are **not auto-generated**. They are opt-in by placing a `feed.xml` template in the appropriate `layouts/` subdirectory. The template uses standard template context (collections, taxonomy data, site data) to build the XML. Alloy provides helpful filters (`rfc822_date`, `xml_escape`) but no built-in feed template.

**Feed template placement determines output path and context:**

| Template location | Output path | Use case |
|---|---|---|
| `layouts/feed.xml` | `/feed.xml` | Site-wide feed (template accesses any collection) |
| `layouts/blog/feed.xml` | `/blog/feed.xml` | Section feed (template receives section context) |
| `layouts/taxonomies/tags/feed.xml` | `/tags/:slug/feed.xml` | Per-term feed (rendered once per term with term context) |

No `feed:` config block exists. The template controls what data it renders — the same `collections.*`, `taxonomy.*`, and `site.*` context available to any other template.

---

## 1e. Output Formats

Liquid is format-agnostic — templates can output HTML, JSON, XML, plain text, or anything else. Everything in `layouts/` is a template — rendered by whichever engine is configured.

### Template File Extensions

The `.liquid` extension marks a file as a Liquid template. Files without `.liquid` are Go templates (or shared static files with no template tags). The file extension before `.liquid` (or the bare extension for Go) determines the output format.

**Liquid engine** — looks for `.liquid` files first, falls back to bare extension:

```
layouts/feed.xml.liquid         → rendered by Liquid, outputs XML
layouts/api.json.liquid         → rendered by Liquid, outputs JSON
layouts/default.liquid          → rendered by Liquid, outputs HTML (default)
```

**Go engine** — uses bare extension files directly:

```
layouts/feed.xml                → rendered by Go templates, outputs XML
layouts/api.json                → rendered by Go templates, outputs JSON
layouts/default.html            → rendered by Go templates, outputs HTML
```

**The template engine is a global, project-wide setting** — not per-page. One engine is active per build. The `.liquid` extension is a convenience for users who want to keep both Liquid and Go template files in `layouts/` side by side (e.g., to make switching engines easier), but only the configured engine's files are used at build time:

```
layouts/
├── default.liquid     ← used when engine: "liquid"
├── default.html       ← used when engine: "go" (or as Liquid fallback)
├── feed.xml.liquid    ← used when engine: "liquid"
├── feed.xml           ← used when engine: "go" (or as Liquid fallback)
└── robots.txt         ← static content, no template tags, used by either
```

If the Liquid engine finds no `.liquid` file, it falls back to the bare extension and parses it as Liquid. The Go engine only reads bare extension files — it never reads `.liquid` files. Alloy does not inspect file contents to determine the engine — it sends whatever file it finds to the configured engine. If the file contains syntax for the wrong engine, the engine will return a parse error and the build fails. This is the implementor's responsibility, not something Alloy guards against.

### Multiple Output Formats

Content files can specify which output formats to generate:
```yaml
---
title: "My Post"
outputs: ["html", "json"]     # Generate both /my-post/index.html and /my-post/index.json
---
```

The corresponding layout must exist for each format. For a page requesting `json` output with the Liquid engine, Alloy looks for `layouts/single.json.liquid` first, then `layouts/single.json` (following the standard layout lookup order).

---

## 1f. Content Lifecycle

### Draft / Future / Expired Content

Controlled via front matter:

```yaml
---
title: "Upcoming Feature"
draft: true                    # Excluded from build, visible in dev server only
publishDate: 2026-05-01       # Not published until this date
expiryDate: 2026-12-31        # Removed from output after this date
---
```

- **Default state is published.** If `draft` is `false` or not present, and `publishDate` is not set or is in the past, the page is published immediately. You opt into hiding content, not into showing it.
- `draft: true` → always excluded from `alloy build` and `alloy serve`. Visible only in `alloy dev` (dev mode always shows drafts so authors can preview their work). **A draft page ignores `publishDate` and `expiryDate` in dev mode** — it behaves as if it were published now. Date fields are still used for sort ordering within collections.
- `publishDate` in the future → excluded from `alloy build`, `alloy serve`, AND `alloy dev`. Future-dated pages are hidden everywhere until their publish date arrives. To preview a future-dated page, set `draft: true` — the draft flag overrides date filtering in dev mode.
- `expiryDate` in the past → excluded from `alloy build`, `alloy serve`, and `alloy dev`. To preview an expired page, set `draft: true`.
- All three interact with collections and pagination:
  - **Drafts**: excluded from `collections.*` in `alloy build` and `alloy serve`, **included** in `alloy dev` (so authors can preview paginated lists with draft content)
  - **Future `publishDate`**: excluded from `collections.*` in build, serve, and dev
  - **Past `expiryDate`**: excluded from `collections.*` in build, serve, and dev
  - **Pagination** always operates on the post-filtered collection. Lifecycle filtering happens first, then pagination chunks the remaining items. A paginated list of 47 articles with 3 drafts produces 5 pages of 10 in build/serve mode (44 items) but may produce different page counts in dev mode (47 items, drafts included).

### Content Summaries

**Front matter `summary`** (highest priority):

```yaml
---
title: "My Post"
summary: "A short description of this post"
---
```

Supports HTML via YAML multiline strings:
```yaml
summary: |
  <h2>Custom formatted summary</h2>
  <p>With full HTML support</p>
```

If no `summary` in front matter, `page.summary` is `nil`. Alloy does not auto-generate summaries — the author provides one explicitly or the template handles the absence.

Available in templates as `{{ page.summary }}`.

**Summaries are static data — not template-rendered.** They do not pass through the Liquid or Go template engine. Rendering summaries through the template engine would require coupling the content transformation and template rendering pipeline stages (or a two-pass render), both of which break pipeline independence and degrade incremental build performance — this is a known cause of slowdowns in 11ty.

### Dynamic Summaries

For display-time summary composition, build the summary in the list template rather than in the content file. The list template has full access to each page's front matter:

```liquid
{% for article in collections.articles %}
  <article>
    <h2><a href="{{ article.url }}">{{ article.title }}</a></h2>
    <p>By {{ article.author }} — {{ article.date | date: "%B %Y" }}</p>
    <p>{{ article.summary }}</p>
  </article>
{% endfor %}
```

Liquid's `{% capture %}` and Go's `{{ define }}` can compose summaries within a single template, but these are local variables — they do not feed back into the data cascade and are not accessible from other templates or collection loops.

### Table of Contents (`page.toc`)

Alloy automatically extracts the heading structure from each page during markdown rendering and exposes it as `page.toc` — a nested array of headings available in templates. Sites control the TOC markup; Alloy provides the data.

```liquid
<!-- layouts/partials/toc.liquid -->
<nav class="toc">
  {% for item in page.toc %}
    <a href="#{{ item.id }}">{{ item.text }}</a>
    {% if item.children.size > 0 %}
      <ul>
        {% for child in item.children %}
          <li><a href="#{{ child.id }}">{{ child.text }}</a></li>
        {% endfor %}
      </ul>
    {% endif %}
  {% endfor %}
</nav>
```

**Data structure** — Each entry in `page.toc` has:

| Field | Type | Description |
|---|---|---|
| `id` | string | The heading's `id` attribute (auto-generated slug or `{#custom-id}` override) |
| `text` | string | Plain text content of the heading (no HTML) |
| `level` | int | Heading level (2-6; h1 is excluded — it's the page title) |
| `children` | array | Nested headings one level deeper |

Nesting follows the heading hierarchy — h3s nest under h2s, h4s under h3s. The top-level array contains the shallowest headings (typically h2). Pages with no headings (or only h1) have an empty `page.toc`.

**Auto heading IDs** — Alloy enables goldmark's `parser.WithAutoHeadingID()` by default. Every heading gets a slugified `id` attribute automatically (e.g., "Getting Started" → `id="getting-started"`). Duplicate headings get a numeric suffix (`getting-started-1`). The algorithm is goldmark's default. Configurable:

```yaml
content:
  markdown:
    autoHeadingID: true   # default: true — every heading gets an id attribute
```

Set `content.markdown.autoHeadingID: false` to disable auto-generated heading IDs. Headings render as plain `<h2>My Section</h2>` with no `id` attribute. Note: TOC anchor links (`#my-section`) won't work without heading IDs.

**Heading attributes** — Authors can override the auto-generated ID using the heading attributes syntax:

```markdown
## My Heading {#custom-id}
## My Heading {.custom-class}
## My Heading {#custom-id .custom-class}
```

Manual `{#id}` overrides take precedence over auto-generated IDs. Alloy enables `parser.WithAttribute()` alongside `parser.WithAutoHeadingID()`.

**Extraction** — TOC is extracted from the goldmark AST during markdown rendering (Phase 1, step 3), not from rendered HTML. This is fast and reliable — no HTML parsing pass needed. The AST contains the heading nodes with their auto-generated or attribute-overridden IDs.

**Important:** If a render hook (`render-heading.liquid`) modifies heading IDs in the HTML output, the TOC data will not reflect those changes — it reflects the AST-level IDs. To sync, use the `onContentTransformed` plugin hook to mutate `page.toc` after markdown rendering but before layout rendering. The hook receives `{ html, toc, path, url, frontMatter }` — modify `page.toc` and return the updated object. For non-markdown pages (HTML, paginated data pages), `toc` will be empty — plugins can build it from the rendered HTML.

**Config:**

```yaml
content:
  markdown:
    toc: true     # default: true — generate page.toc for all pages
```

Set `content.markdown.toc: false` to disable TOC generation entirely (skips the AST walk for sites that don't use TOC).

**Future option:** Configurable ID generation algorithm via a custom `parser.IDs` implementation, exposed as a config option. For v1, goldmark's default slugification is used.

---

## 1g. External Data Sources

### Overview

Alloy can fetch data from external sources at build time. Fetched data is parsed into structured Go types and injected into the data cascade as `site.data.*` — templates can't tell the difference between local files and fetched data.

### Two Modes: Built-in (Simple) and Plugin (Complex)

**Built-in `rest` and `graphql` types** handle simple, single-request fetches. No pagination, no auth, no multi-step aggregation — just a single HTTP call that returns all the data.

**`plugin` type** gives the plugin full ownership of data acquisition. The plugin handles URLs, authentication, pagination, error handling, and returns the final dataset. Alloy only caches the result and injects it into the data cascade.

### Configuration

```yaml
# alloy.config.yaml
sources:
  # Simple — built-in single REST call
  posts:
    type: "rest"
    url: "https://api.example.com/posts.json"
    cache: 3600
    as: "posts"                    # Available as site.data.posts

  # Simple — built-in single GraphQL call
  products:
    type: "graphql"
    endpoint: "https://api.example.com/graphql"
    query: |
      { products { id, name, price, slug } }
    cache: 1800
    as: "products"                 # Available as site.data.products

  # Complex — plugin owns everything
  blog:
    type: "plugin"
    plugin: "cms-posts"
    cache: 3600
    as: "blog"                     # Available as site.data.blog

  # Complex — plugin handles auth, pagination, aggregation
  users:
    type: "plugin"
    plugin: "alloy-postgres"
    cache: 3600
    as: "users"
```

### Built-in Fetchers (REST / GraphQL)

Single HTTP request. Response is parsed and injected into the data cascade.

**Format parsing:**

| Content-Type | Parser | Result |
|---|---|---|
| `application/json` | `json.Unmarshal` | maps/slices |
| `text/xml` / `application/xml` | `xml.Decode` | maps/slices |
| `text/csv` | `csv.Parse` | array of maps (header row = keys) |

GraphQL responses are automatically unwrapped — the `data` envelope is stripped so you get the clean payload.

**Error handling:** If the request fails or returns invalid data, the build fails with a clear error:

```
[alloy] ERROR Source "posts": fetch failed from https://api.example.com/posts.json
        HTTP 503 Service Unavailable
        Build aborted.
```

### Plugin Sources

For anything beyond a simple single-request fetch — paginated APIs, authenticated endpoints, databases, CMS adapters — use `type: "plugin"`. The plugin registers a source handler and owns the full data acquisition lifecycle.

**Source plugins are Tier 3 (Node) by nature.** They need network access, npm packages (HTTP clients, auth libraries, database drivers), and environment variables. WASM plugins (Tier 2) are sandboxed with no network access — they're better suited for filters and data transforms, not data fetching.

```javascript
// plugins/cms-posts.js (Node — Tier 3)
export default function(alloy) {
  alloy.source("cms-posts", async () => {
    const API_URL = process.env.CMS_API_URL;
    const TOKEN = process.env.CMS_TOKEN;

    let allPosts = [];
    let page = 1;
    let hasMore = true;

    while (hasMore) {
      const response = await fetch(`${API_URL}/posts?page=${page}`, {
        headers: { Authorization: `Bearer ${TOKEN}` }
      });
      const json = await response.json();
      allPosts = allPosts.concat(json.data);
      hasMore = json.meta.nextPage !== null;
      page++;
    }

    return allPosts;
  });
}
```

Alloy calls the plugin, receives the returned data, caches it (respecting the `cache` TTL from config), and injects it into the data cascade. The plugin handles URLs, auth, pagination, retries, and error handling internally.

### Caching

All source data (built-in and plugin) is cached to `.alloy/fetch-cache/` on disk:
- Cache survives process restarts
- `cache` value in config sets TTL in seconds
- If TTL has not expired, cached data is used without fetching

### Dev Mode Behavior

- **Cache-first** — never refetch unless TTL has expired
- **File changes don't trigger refetches** — only content/template rebuilds
- **Force refetch** — `alloy dev --refetch` or `alloy serve --refetch` bypasses cache TTL and fetches fresh data on startup
- **Expired TTL** — refetch happens on next rebuild, not proactively

### Combined with Virtual Pages

Fetched data feeds directly into pagination for page generation:

```yaml
---
pagination:
  data: site.data.products
  as: product
permalink: "/products/{{ product.slug }}/"
---
<h1>{{ product.name }}</h1>
<p>{{ product.price }}</p>
```

One template + an external data source → pages generated at build time.

---

## 1h. Static Files and Passthrough

### `static/` Directory Convention

Files in `static/` are copied to the output root as-is, with no processing:

```
static/
├── favicon.ico          → _site/favicon.ico
├── robots.txt           → _site/robots.txt
├── images/
│   └── logo.png         → _site/images/logo.png
└── downloads/
    └── whitepaper.pdf   → _site/downloads/whitepaper.pdf
```

No template rendering, no fingerprinting, no transformation. These are verbatim files.

### Passthrough Copy (Config-Driven)

For files outside the project directory (or inside but in non-standard locations), passthrough mappings copy external directories into the output:

```yaml
# alloy.config.yaml
passthrough:
  # Design system dist files → output
  - from: "../design-system/dist/elements"
    to: "elements"                           # → _site/elements/

  # Fonts from a shared repo
  - from: "../shared-assets/fonts"
    to: "assets/fonts"                       # → _site/assets/fonts/

  # Internal directory passthrough
  - from: "vendor/js"
    to: "js/vendor"                          # → _site/js/vendor/
```

`from` paths are resolved relative to the project root. Absolute paths are also supported.

#### Passthrough Filtering (issue #547)

Passthrough mappings support two filtering mechanisms to control which files are copied:

**1. Glob `from`** — The `from` field accepts glob patterns (`**`, `{a,b}`, `[chars]`) via the `doublestar` library. When `from` contains glob metacharacters, only matching files are copied. The output path preserves directory structure relative to the glob root (the longest static prefix before any metacharacter).

```yaml
passthrough:
  # Copy only .js and .css files from elements/
  - from: "elements/**/*.{js,css}"
    to: "assets/packages/@rhds/elements/elements"
    # elements/rh-button/rh-button.js → _site/assets/.../rh-button/rh-button.js

  # Copy only .woff2 font files
  - from: "../shared-assets/fonts/**/*.woff2"
    to: "assets/fonts"
```

Glob root extraction: `elements/**/*.js` → root = `elements`. Matched files are resolved relative to this root. When `from` has no glob characters, it is treated as a plain directory path (existing behavior).

**2. `exclude` patterns** — An optional `exclude` array of gitignore-style patterns. Files matching any exclude pattern are skipped during copy. Works with both plain-directory and glob `from` values.

```yaml
passthrough:
  # Copy everything except demo HTML and sourcemaps
  - from: "elements"
    to: "assets/packages/@rhds/elements/elements"
    exclude:
      - "*.html"         # any .html file at any depth
      - "demo/"          # entire demo/ directory tree
      - "**/*.map"       # any .map file at any depth (explicit)

  # Glob from + exclude
  - from: "elements/**/*.{js,css}"
    to: "assets/packages/@rhds/elements/elements"
    exclude:
      - "*.min.js"       # skip minified JS
```

**Exclude pattern normalization** (gitignore-style): patterns without `/` match filenames at any depth (internally prepended with `**/`); patterns ending with `/` match entire directory trees (internally appended with `**`); patterns containing `/` (not just trailing) match against the relative path as-is. All matching uses `doublestar.Match`.

| Pattern | Normalized | Matches |
|---------|-----------|---------|
| `*.html` | `**/*.html` | `foo.html`, `sub/bar.html`, `a/b/c.html` |
| `demo/` | `demo/**` | `demo/index.html`, `demo/sub/file.js` |
| `demo/*.html` | `demo/*.html` | `demo/foo.html` (not `demo/sub/bar.html`) |
| `**/*.map` | `**/*.map` | `foo.map`, `sub/bar.map` |

**Passthrough overlap with managed directories:** If a passthrough `from:` path resolves to any of the configured structure directories (`content`, `layouts`, `assets`, `static`, `data`), it is silently ignored — those directories are already processed by the pipeline. This prevents duplicate processing and output conflicts.

### Build vs Dev

**Build mode (`alloy build`):**
- Static and passthrough files are **copied** to `_site/`

**Dev mode (`alloy dev`):**
- Static and passthrough files are **served directly** from their source locations
- The Go HTTP server maps URL paths to source directories — no copy at all
- File changes are reflected instantly (no rebuild, no copy needed — but the watcher still triggers a browser reload)

**Serve mode (`alloy serve`):**
- Static and passthrough files are **copied** to `_site/` (same as build)
- Passthrough `from:` directories must be watched for changes. On change, only the modified file is recopied to `_site/<to>/<relative-path>` — not the entire passthrough directory. A browser reload is triggered after the recopy.

**Passthrough file watching** — `WatchDirs()` must include all passthrough `from:` directories from config. Passthrough sources are directory trees — the watcher must recursively watch subdirectories, including subdirectories created after the server starts. Changes to passthrough files are classified as `PassthroughChange` and trigger a targeted file recopy instead of a full pipeline rebuild.

### Watch Directories (issue #530)

Plugin filters may read files from directories outside the standard content tree (e.g., `elements/rh-*/docs/*.md` for component documentation). These directories are invisible to the file watcher, so changes never trigger rebuilds during `alloy serve`.

The `watch:` config key registers extra directories for pipeline-triggering watches:

```yaml
watch:
  - from: "elements"
    type: content
  - from: "shared-layouts"
    type: layout
  - from: "external-data"
    type: data
```

**Fields:** `from` (required) — directory path relative to project root, glob patterns supported (same rules as passthrough `from`). `type` (required) — one of `content`, `layout`, `data`. Determines the `ChangeType` classification, which controls rebuild scope.

**Behavior:** `WatchDirs()` includes all watch `from:` directories (or glob roots) in the watch list. `ClassifyChange()` maps files under watch directories to `ContentChange`, `LayoutChange`, or `DataChange` based on their declared `type`. All three types map to `RebuildPipeline` via `RebuildScopeForChangeType()` — this is the key difference from passthrough (which triggers `RebuildRecopy`). Watch directories are checked before passthrough directories in `ClassifyChange` — a directory in both `watch:` and `passthrough:` uses the watch classification.

**Validation:** `from` must not be empty. `type` must be one of: `content`, `layout`, `data`. Invalid entries produce a validation error with the array index (e.g., `watch[1].type must be content, layout, or data`). Duplicate `from` paths are rejected — ambiguous classification (which type wins?). `from` paths matching base structure directories (`content`, `layouts`, `data`, `assets`, `static`) are rejected — those already have fixed `ChangeType` classification. Trailing slashes in `from` are normalized (stripped), not rejected.

### Pre-Build Validation

Before any content rendering begins, Alloy extracts front matter, assembles the data cascade, and computes every output path to detect conflicts. Front matter extraction is fast (reads only the metadata block, not the body) and the parsed data is reused by Phase 1 — no duplicate reads. This catches path conflicts early, before spending time on Markdown rendering and template execution.

**Sources scanned:**

1. **Content files** — Compute output paths from content discovery + permalink rules
2. **Static files** — Walk `static/` directory
3. **Passthrough mappings** — Walk each `from` directory, apply `to` prefix
4. **Auto-generated files** — sitemap.xml (if enabled via config), feed.xml templates (if present in layouts/)
5. **Aliases** — Additional output paths from front matter `aliases`
6. **Pagination** — Virtual page output paths from pagination rules
7. **Taxonomy pages** — Auto-generated taxonomy index and term pages

**Conflict detection:**

If two or more sources target the same output path, the build fails immediately with a clear error:

```
[alloy] ERROR Output path conflict detected:
        _site/elements/button.js is claimed by:
          1. static/elements/button.js
          2. passthrough "../design-system/dist/elements" → "elements"
        
        Resolve by renaming one source or adjusting the passthrough "to" path.
        Build aborted.
```

**Priority rules (no implicit wins):**
- Conflicts are always errors. There is no priority system where one source silently overwrites another.
- The user must resolve conflicts explicitly — rename the file, change the passthrough `to` path, or remove one source.

**Lifecycle hooks:**

Two plugin hooks surround the validation pass:

- **`onBeforeValidation`** — Plugins can register additional output paths (e.g., a plugin that generates a `_headers` file or `_redirects` file for Netlify). Payload: the collected output path map. Plugins append entries. Runs before conflict detection.
- **`onAfterValidation`** — Plugins can inspect the full validated output map (all sources resolved, no conflicts). Read-only. Useful for plugins that need the complete output manifest before the build starts (e.g., pre-generating a service worker file list, or logging the total output count).

```javascript
// Plugin registering additional output paths
alloy.on("onBeforeValidation", {}, (outputMap) => {
  outputMap.add("_redirects", { source: "plugin:netlify-redirects" });
  outputMap.add("_headers", { source: "plugin:netlify-headers" });
  return outputMap;
});

// Plugin inspecting the validated manifest
alloy.on("onAfterValidation", {}, (outputMap) => {
  console.log(`Build will produce ${outputMap.size} output files`);
});
```

---

## 1i. i18n / Multilingual

### Opt-In

The entire i18n system is activated by the presence of a `languages` key in config. No `languages` key = single language site, zero overhead, none of this applies.

### Config

```yaml
languages:
  en:
    title: "My Site"
    weight: 1                        # Default language (lowest weight)
    strings:                         # Optional — layout UI chrome
      read_more: "Read more"
      posted_on: "Posted on"
  fr:
    title: "Mon Site"
    weight: 2
    strings:
      read_more: "Lire la suite"
      posted_on: "Publié le"
```

`strings` is optional. You can declare languages for content routing and output paths without using the strings map at all. If your layouts are language-neutral or you handle translations another way, omit it.

### Content Structure

Each language gets its own top-level directory under `content/`:

```
content/
├── en/
│   ├── _data.yaml              # English directory data (cascades down)
│   ├── blog/
│   │   └── my-post.md          # Written in English
│   └── about.md
└── fr/
    ├── _data.yaml              # French directory data
    ├── blog/
    │   └── my-post.md          # Written in French
    └── about.md
```

Content pages own their own content — they're written in their language. They don't use translation lookups.

### Output

Each language outputs under its own prefix:

```
_site/
├── en/
│   ├── blog/my-post/index.html
│   └── about/index.html
└── fr/
    ├── blog/my-post/index.html
    └── about/index.html
```

The default language (lowest `weight`) can optionally output at the root instead of under its prefix:

```yaml
languages:
  en:
    title: "My Site"
    weight: 1
    root: true                   # Output at _site/ instead of _site/en/
```

With `root: true`, English pages output at `_site/about/index.html` while French pages output at `_site/fr/about/index.html`.

### Data Cascade Integration

When building for a specific language, the active language's config block populates `site.language`:

- `site.title` → overridden by `languages.{lang}.title`
- `site.language.strings` → the active language's `strings` map
- `site.language.code` → the language key (`"en"`, `"fr"`)

This happens at step 7 (data cascade assembly). By the time templates render, language data is in the context like any other site data. No special translation layer — just a map lookup.

### Layouts — Shared, Not Duplicated

Layouts are shared across all languages. One `layouts/` directory serves all languages. The `strings` map handles UI chrome:

```liquid
<!-- layouts/post.liquid -->
<article>
  <h1>{{ page.title }}</h1>
  <p>{{ site.language.strings.posted_on }} {{ page.date | date: "%B %d, %Y" }}</p>
  {{ page.content }}
  <a href="{{ page.next.url }}">{{ site.language.strings.read_more }}</a>
</article>
```

Both Liquid and Go templates access the same data:

- **Liquid**: `{{ site.language.strings.read_more }}`
- **Go**: `{{ .site.language.strings.read_more }}`

### Translation Linking

Pages are matched across languages by their relative path within the language tree. `content/en/about.md` and `content/fr/about.md` are the same page in different languages — no explicit front matter linking needed.

Templates can access a page's translations:

```liquid
{% for translation in page.translations %}
  <a href="{{ translation.url }}" hreflang="{{ translation.language }}">
    {{ translation.language }}
  </a>
{% endfor %}
```

`page.translations` is an array of the same page in other languages, matched by relative path. If no counterpart exists in a language, that language is absent from the array.

### Build Behavior

The build iterates over declared languages. Each language is effectively a normal build with:
- A different content tree (`content/{lang}/`)
- A different `site.language` value in the data cascade
- A different output prefix (`_site/{lang}/`)

Same pipeline, same stages, just different data input. Languages can build in parallel — they're independent content trees with shared layouts.

### Collections and Taxonomies

Collections and taxonomies are per-language. `collections.blog` for the English build contains only English blog posts. Taxonomy pages (`/en/tags/javascript/`, `/fr/tags/javascript/`) are generated per-language from each language's content.

### External Data Sources

For sites with translated content in a CMS or database, the existing `sources` + pagination system handles it. A source plugin fetches content for each language, and virtual pages are generated per-language. No special i18n fetcher needed — the `languages` config provides the language context, and the plugin uses it.

---

## 2. Build Pipeline

### Stages (in order)

**Phase 0 — Pre-Build Validation**

1. **Config Load** — Parse `alloy.config.yaml`, merge CLI flags
2. **Content Discovery** — Walk `content/` dir, collect all source files
3. **Front Matter Extraction** — Parse YAML/TOML/JSON front matter from each content file (fast: reads front matter only, not body). Files without front matter delimiters are a build error.
4. **Data Cascade Assembly** — Load `data/` globals, `_data.yaml` per-directory, merge with front matter per-file in defined order. This data is reused by Phase 1 — no duplicate reads.
5. **Output Path Computation** — Compute all output paths from content (using permalink patterns + front matter data), static, passthrough, auto-generated files, aliases, taxonomy pages, and pagination
6. **Plugin Hook: `onBeforeValidation`** — Plugins register additional output paths
7. **Conflict Detection** — Check for duplicate output paths across all sources. Fail fast with clear error if conflicts found.
8. **Plugin Hook: `onAfterValidation`** — Plugins receive the validated output manifest (read-only) and the assembled data cascade (mutable, shared pointer). This is the point where plugins can inspect what pages will be built, inject data into the cascade for templates, compute derived values, or validate the merged dataset. The cascade is trustworthy here — validation has passed, all pages are confirmed valid.

**Phase 1a — Per-Batch Processing (runs once per language batch)**

Each language batch is processed independently. The following steps run inside `applyBatchContext()` per batch.

9. **Plugin Hook: `onPagesReady`** — Plugins can inject virtual pages with front matter (including taxonomy terms) before taxonomy collection. Fires once per language batch after data cascade is applied. Payload: `{ pages, siteData }` object — `pages` is the array of discovered pages with cascaded front matter, `siteData` is the site data object. Return value: same shape with additional virtual pages appended to `pages`. Virtual pages require `path` and `url`; optional `frontMatter` (with taxonomy terms like `tags`) and `content` (raw markdown — will be rendered in Phase 1b). No `html` field — content has not been rendered yet. Injected pages flow through the full remaining pipeline: taxonomy collection, content rendering, layout rendering, and output. This is Alloy's equivalent of Hugo's content adapters — a dedicated early-pipeline mechanism for data→pages conversion that runs before taxonomy indexing. Unlike `onContentLoaded` (which fires after content rendering), pages injected here participate in taxonomy collections. Per-batch firing avoids the language routing problem described in #521.
10. **Taxonomy Collection** — Build taxonomy maps from page front matter (including virtual pages injected by `onPagesReady`).
11. **Pagination** — Expand pagination templates.

**Phase 1b — Content Rendering (Liquid + Markdown → Intermediate HTML)**

Data cascade and front matter are already assembled from Phase 0; taxonomy data is available from Phase 1a.

12. **Content Transformation** — Markdown → HTML (via goldmark with template tag auto-detection), raw HTML passthrough. `{{ }}` and `{% %}` patterns survive goldmark automatically.
13. **Plugin Hook: `onContentTransformed`** — Plugins can modify rendered content, TOC, and front matter. Fires once per page with a page-scoped object payload: `{ html, toc, path, url, frontMatter }` (see Lifecycle Events in §5 for payload contract). **This hook fires after Markdown→HTML but before layout rendering in all modes** — single-language and i18n pipelines must fire at the same stage. The i18n pipeline must not defer this hook to after layout rendering or taxonomy generation.
14. **Plugin Hook: `onContentLoaded`** — Plugins can modify `frontMatter` and `html` on existing pages after content rendering. Fires once with the full pages array. Other fields (`content`, `path`, `url`) are present for inspection but mutations are not applied back. Cannot inject virtual pages — return array must be same length and same order as input (use `onPagesReady` for injection).
15. **Template Resolution** — Match each content file to its layout (lookup order)
16. **Content Template Rendering** — Content body is rendered through the template engine with page data + site data context, producing an HTML string.
17. **Layout Rendering** — The rendered content HTML is injected into the resolved layout as `{{ content }}` (Liquid) or `{{ .content }}` (Go), then the layout is rendered. Content and layout have isolated scopes — variables defined in content do not leak into layout or vice versa.
18. **Plugin Hook: `onPageRendered`** — Plugins can post-process intermediate page HTML

**Phase 2 — SSR Transform (opt-in, only runs if `ssr:` is configured)**

Skipped entirely unless the project has an `ssr:` block in config. Without it, Phase 1b output is the final HTML — Web Components render client-side only.

19. **Component tracking** — Scan each page's intermediate HTML for custom element tags (anything with a hyphen). Record which pages use which components for cache invalidation.
20. **Per-page SSR** — For each page containing custom elements, pipe the full intermediate HTML to the configured `ssr.command` via stdin. The command reads stdin, transforms the HTML, and writes the result to stdout with Declarative Shadow DOM.

**Phase 3 — Output**

21. **Asset Copy** — Copy `assets/` files to `_site/`, trigger `onAssetProcess` plugin hooks
22. **Static + Passthrough** — Copy static files and passthrough mappings to `_site/`
23. **Output Writing** — Write all final HTML + assets to `_site/`
24. **Plugin Hook: `onBuildComplete`** — Final notification

### Parallelism

Each phase must complete before the next begins, but work within each phase is parallelized:

- **Per-file stages** (content discovery, front matter extraction, data cascade assembly, Markdown transformation, template rendering) run concurrently using a `runtime.NumCPU()` worker pool
- **Templates** are parsed once at startup and reused across all pages (liquidgo's parse-once/render-many model)
- **Phase 0 internal concurrency**: after config loads, content discovery, static file walk, passthrough directory walk, and auto-generated file paths all run concurrently. Front matter extraction and data cascade assembly run after content discovery completes. Content-dependent path computation (permalinks, aliases, pagination, taxonomy pages) waits on data cascade assembly. Validation hooks and conflict detection run sequentially after that.
- **Asset and static copy** runs as its own stage during Phase 3 (output), after all content rendering and hooks complete — not overlapping with other pipeline stages (#507). File copies within the stage use a bounded worker pool of `runtime.NumCPU()` goroutines (#511): walk the source directory, create directories synchronously, dispatch file copies to the pool via a semaphore channel. First error cancels remaining work. Running the copy stage in the background alongside rendering was attempted (#492, #501) but caused I/O contention — 31% build regression on a 431-page site (#507).
- **Plugin hooks** are synchronous barriers — all pages batch through each hook before proceeding to the next stage. This keeps the Go pipeline in control and bounds plugin latency.

### Error Handling

Errors are treated differently depending on the mode. The core principle: **`alloy build` never produces partial output. `alloy dev` keeps running and shows errors clearly.**

**`alloy build` (production) — fail fast, fail completely:**

- **Any page failure aborts the entire build.** If 1 out of 1,000 pages fails to render, no output is written. A partial deploy could publish assets, layouts, or pages that depend on the failed page being present.
- **Plugin error or crash aborts the build.** A hook returning an error and a plugin crash (process exit, panic, unhandled exception) are treated identically — both are fatal. The build stops immediately. The only non-fatal plugin failure is a **timeout**: a timed-out hook produces a warning, discards its modifications, and continues with the pre-hook payload.
- **External data source unreachable aborts the build.** Even if stale cached data exists, the build fails. The user may be deploying specifically to pick up new data — serving stale content silently could publish an incomplete or inconsistent site.
- **Error output includes:** file path, line number (when available), error message, and the pipeline stage where the failure occurred. For template errors, include the relevant source snippet.

```
[alloy] ERROR content/blog/my-post.md:14
        Unknown filter "formattDate" — did you mean "formatDate"?
        Stage: template rendering
        Build aborted. No output written.
```

**`alloy dev` (development) — keep running, show errors:**

- **Page render failure does not stop the server.** The failed page shows an error overlay in the browser (file path, line number, code snippet). Other pages continue to serve normally. The error clears on the next successful rebuild.
- **Plugin error or crash stops the server.** A hook returning an error and a plugin crash are treated identically — both stop the server. Plugins are foundational — if a plugin fails to load, returns an error from a hook, or crashes during execution, the server exits with diagnostic info (plugin name, error, stack trace where available). The user must fix the plugin and restart. The only non-fatal plugin failure is a **timeout**: a timed-out hook produces a warning and continues with the pre-hook payload.
- **External data source unreachable shows a warning.** The server continues with stale cached data (if available) or without that data source. A persistent warning displays in the terminal and browser overlay indicating which sources are unavailable.

```
[alloy] WARN Source "posts": https://api.example.com/posts.json unreachable
        Using cached data (age: 2h 15m). Rebuild will use stale data until source recovers.
```

```
[alloy] ERROR Plugin "word-count.js" crashed: TypeError: Cannot read property 'split' of undefined
        at wordCount (plugins/word-count.js:3:22)
        Server stopped.
```

### Incremental Builds — Dev Mode Only

Incremental builds are exclusive to `alloy dev` (dev mode). `alloy build` and `alloy serve` always do a **full clean rebuild** — every page is rendered, every file is written. This ensures CI/CD and production preview produce deterministic, complete output.

In dev mode, after the initial full build, the file watcher triggers incremental rebuilds on changes. Alloy tracks **actual data reads** to determine the minimum set of pages to rebuild. The pipeline function `BuildIncremental(cfg, contentMap, previousCache, changedFiles)` accepts a previous build cache and only rebuilds affected pages.

**Content-hash change detection** (SHA-256, stored in `.alloy/cache.json`):
- On incremental rebuild, skip unchanged files entirely (no re-parse, no re-render)
- Config changes trigger full rebuild
- `BuildResult.PagesSkipped` reports how many pages were skipped via cache

**Shared data changes** — When global data files (`data/`), directory data (`_data.yaml`), or collections change, all pages that could be affected are rebuilt. Per-page content-hash detection still prevents unnecessary work for pages whose own content hasn't changed.

**Template invalidation** — Template changes invalidate pages that use that specific template (tracked via the layout resolution step), not all pages in a stage.

**Component invalidation** — Handled entirely in Phase 2. A component definition change triggers re-SSR of all pages using that component (tracked via `componentToPages` in `.alloy/components.json`). Phase 1 is untouched. See Section 6.

---

## 3. Data Cascade

Simplified from 11ty's model. **6 levels, last wins:**

```
1. Global data              (data/*.yaml, data/*.json)
2. Directory data            (content/blog/_data.yaml — cascades into subdirs)
3. Front matter              (per-file YAML/TOML block)
4. Pre-taxonomy computed data (onPagesReady plugins — before taxonomy collection, before Markdown)
5. Per-page transform        (onContentTransformed plugins — after Markdown→HTML, per-page; fires at step 13)
6. Batch mutation             (onContentLoaded plugins — after Markdown→HTML, batch-level; fires at step 14, wins for frontMatter and html)
```

### Directory Data Cascading

`_data.yaml` cascades down into **all descendant directories**, not just those with their own `_data.yaml`. A child directory's `_data.yaml` can override parent values:

```
content/_data.yaml          → applies to all content
content/blog/_data.yaml     → merges over parent, applies to blog/ and all subdirs
content/blog/2024/_data.yaml → merges over parent, applies to blog/2024/ and its subdirs
```

A page at `content/blog/2024/march/post.md` inherits from the nearest ancestor `_data.yaml` — the lookup walks up through `blog/2024/march/`, `blog/2024/`, `blog/`, `content/` until a `_data.yaml` is found. Most directories will not have their own `_data.yaml`; they rely entirely on ancestor inheritance.

### Merge Rules
- Objects are **deep-merged** (nested keys merge recursively)
- Arrays are **replaced** (not concatenated) — predictable, but be aware of the implications:
  ```yaml
  # content/blog/_data.yaml
  scripts: ["analytics.js"]

  # content/blog/my-post.md front matter
  scripts: ["custom.js"]

  # Result: scripts = ["custom.js"] — NOT ["analytics.js", "custom.js"]
  # Front matter replaces the entire array, it does not merge with the parent.
  # To include both, list all values in the front matter array.
  ```
- Front matter always wins over directory data, which wins over global data
- Computed data (from plugins) has highest priority

### Performance: Layered Lookup, Not Deep Copy

Global and directory data are loaded once and **shared by pointer** across all pages. Only front matter is per-page. This avoids copying shared data for every page:

```go
type PageContext struct {
    global      *map[string]any   // shared across ALL pages
    directory   *map[string]any   // shared across directory
    frontMatter map[string]any    // per-page only
}
```

Deep merging happens **lazily** — only when a nested key is accessed at multiple cascade levels (e.g., global has `author.name`, front matter has `author.email`). Pages that never access conflicting nested keys pay zero merge cost.

**Plugin mutation model**: Hooks that receive cascade data (e.g., `onDataCascadeReady`) get the shared pointer, not a copy. Mutations apply globally — all pages see the changes. This is intentional: the hook exists to enrich the cascade before rendering. Plugin trust is the user's responsibility (same model as npm packages, 11ty plugins, Hugo modules). If a plugin corrupts the cascade, that's a bad plugin, not a framework bug.

**Memory**: 3000 pages with 50KB shared data ≈ 50KB (shared) + 1.5MB (front matter), not 150MB (deep copies).

### Collections

Not every subdirectory is a collection. Collections are created in two ways:

**Blog collections** — A section with a date-based permalink pattern (containing `:year`, `:month`, or `:day` tokens in `_data.yaml`) automatically collects its children into a section collection. The permalink declaration is what creates the collection; the date in each post's front matter drives URL structure and default sort order. For example, `content/blog/_data.yaml` with `permalink: "/:year/:month/:day/:slug/"` produces `collections.blog` containing all posts in that directory.

**Taxonomy collections** — Cross-cutting groups created by front matter tags, categories, or other declared taxonomy keys. A blog post and a docs page can both be tagged "javascript" and appear in the same taxonomy collection. Tags can be applied to all pages in a directory via `_data.yaml` without repeating the tag in every file's front matter. See the Taxonomies section below.

Regular directories without date-based permalink patterns are just pages — not collections. To group non-blog pages, use taxonomy tags.

Collections are built once during content discovery, then frozen as read-only data:
- Collections are materialized eagerly — no lazy loading
- Templates access them like any other data

**Default sort order**: date descending (newest first). When two pages share the same date, the full datetime is compared (e.g., `2026-04-10T14:30:00Z` sorts before `2026-04-10T09:00:00Z` in descending order). If the datetime is identical or no time component is provided, filename alphabetical ascending is the final tiebreaker. Pages without a date sort after all dated pages, ordered by filename alphabetical ascending. Sort is deterministic across builds.

**Custom ordering** via config:

```yaml
# alloy.config.yaml
collections:
  blog:
    sortBy: "date"          # front matter key to sort by (default: "date")
    order: "desc"           # "asc" or "desc" (default: "desc")
```

Templates can also sort inline using built-in array filters:

**Liquid:**
```liquid
{% assign alphabetical = collections.blog | sort: "title" %}
{% assign by_author = collections.blog | sort: "author" %}
{% assign recent = collections.blog | sort: "date" | reverse %}

<!-- Sort taxonomy collection by a front matter field -->
{% assign sorted_nav = taxonomies.tags.foundations | sort: "order" %}
{% for page in sorted_nav %}
  <a href="{{ page.url }}">{{ page.title }}</a>
{% endfor %}
```

**Go templates:**
```html
{{ $alphabetical := sort .collections.blog "title" }}
{{ $recent := sort .collections.blog "date" | reverse }}

<!-- Sort taxonomy collection by a front matter field -->
{{ $sorted := sort .taxonomies.tags.foundations "order" }}
{{ range $sorted }}
  <a href="{{ .url }}">{{ .title }}</a>
{{ end }}
```

The `sort` filter is numeric-aware — it compares values as whole numbers when possible, falling back to string comparison otherwise. `{{ collection | sort: "order" }}` correctly sorts `order: 1, 2, 10, 20` (not `1, 10, 2, 20`).

**Numeric detection rules:**
- `int` values (YAML `order: 10` or `order: -1`) → compared as integers (negatives included)
- `float64` with no fractional part (JSON `"order": 10` or `"order": -5.0`) → compared as integers
- String values containing only digits (`order: "10"`) → parsed and compared as integers (negative strings like `"-1"` are NOT parsed — string comparison)
- Everything else (strings, bools, floats with decimals, scientific notation) → compared as strings
- Nil/missing values → sorted to the end of the list

### Taxonomies

Taxonomies are declared in config. The declaration tells Alloy which front matter keys should organize pages into named groups. Alloy auto-generates taxonomy pages (both the index and per-term pages) but does **not** ship any predefined templates — the user must create the layout.

**Simple declaration (defaults):**

```yaml
# alloy.config.yaml
taxonomies:
  tags:
  categories:
  series:
```

Defaults: `tags` generates pages at `/tags/` and `/tags/:slug/`, uses layout `tags`. Equivalent to:

```yaml
taxonomies:
  tags:
    permalink: "/tags/:slug/"
    layout: "tags"
    render: true                   # default — generate taxonomy pages
```

**Collection-only (no pages):**

```yaml
taxonomies:
  tags:
    render: false                  # no /tags/ or /tags/:slug/ pages generated
```

When `render: false`, the taxonomy data is still built and available in templates as `taxonomies.tags.*`, but no output pages are generated and no layout is required. Useful for tags that organize content into navigation sections without needing browsable taxonomy pages.

Duplicate term slugs are not an error when `render: false` — since no pages are generated, there are no output path conflicts.

**Customized declaration:**

```yaml
taxonomies:
  tags:
    render: false                  # organizational metadata, no pages
  categories:
    permalink: "/sections/:slug/"
    layout: "term"                 # generates pages with custom layout
    render: true                   # default, explicit for clarity
```

A post with taxonomy values in front matter:

```yaml
---
title: "Building Web Components"
tags: ["javascript", "web-components", "lit"]
categories: ["tutorials"]
---
```

This populates:
- `taxonomies.tags.javascript` — all pages tagged "javascript"
- `taxonomies.tags.web-components` — all pages tagged "web-components"
- `taxonomies.tags.lit` — all pages tagged "lit"
- `taxonomies.categories.tutorials` — all pages in "tutorials"

The taxonomy index (all terms) is also available:
- `taxonomies.tags` — map of all tags, each with its list of pages
- `taxonomies.categories` — map of all categories

### Taxonomy Page Generation

When `render: true` (the default), Alloy auto-generates both the index page (`/tags/`) and a page per term (`/tags/javascript/`, `/tags/lit/`, etc.). Both use the **same layout** — one template handles both cases via the template context. When `render: false`, this entire section is skipped — no pages, no layout required, no duplicate term slug checks.

**Layout lookup order** (for `tags` with default layout):
1. `layouts/taxonomies/tags.liquid`
2. `layouts/tags.liquid`

**Layout lookup order** (for `tags` with `layout: "term"`):
1. `layouts/taxonomies/term.liquid`
2. `layouts/term.liquid`

If no layout is found, the build errors:
```
[alloy] ERROR No layout found for taxonomy "tags"
        Expected: layouts/taxonomies/tags.liquid or layouts/tags.liquid
        Build aborted.
```

**Template context:**

The template receives a `taxonomy` object that indicates whether it's rendering the index or a specific term:

```liquid
{% if taxonomy.term %}
  <!-- /tags/javascript/ — showing pages for a specific term -->
  <h1>{{ taxonomy.term }}</h1>
  {% for post in taxonomy.pages %}
    <a href="{{ post.url }}">{{ post.title }}</a>
  {% endfor %}
{% else %}
  <!-- /tags/ — showing all terms -->
  {% for term in taxonomy.terms %}
    <a href="{{ term.url }}">{{ term.name }} ({{ term.pages | size }})</a>
  {% endfor %}
{% endif %}
```

The content pages themselves are unaffected — they render with their own layouts as usual. Having `tags: ["javascript"]` in front matter doesn't change how that page renders. It only determines which taxonomy collections the page appears in.

**Undeclared taxonomy keys are ignored.** If a post has `mood: ["happy"]` in front matter but `mood` is not in the `taxonomies` config, no collection is created for it. This prevents noisy collections from arbitrary front matter arrays (image lists, related links, etc.).

**Phase 2 output hashing** — After Phase 1 renders intermediate HTML, the output is hashed. If the hash matches the cached hash, Phase 2 SSR is skipped for that page. This prevents unnecessary SSR work when a page's content hasn't actually changed.

### Template Context

Every template receives:

```liquid
{{ site.title }}          -- from alloy.config.yaml
{{ site.data.navigation }} -- from data/navigation.yaml
{{ page.title }}          -- from front matter
{{ page.content }}        -- rendered content (HTML)
{{ page.date }}           -- from front matter or file date
{{ page.url }}            -- computed permalink
{{ page.collection }}     -- the collection this page belongs to
{{ site.pages }}          -- all pages (for cross-referencing)
{{ collections.blog }}    -- blog section collection (date-based permalink)
{{ taxonomies.tags.javascript }} -- taxonomy collection (cross-cutting)
```

Paginated pages additionally receive:

```liquid
{{ pagination.pageNumber }}    -- current page (1-based)
{{ pagination.totalPages }}    -- total page count
{{ pagination.previousPage }}  -- URL of previous page (nil if first)
{{ pagination.nextPage }}      -- URL of next page (nil if last)
{{ pagination.first }}         -- URL of first page
{{ pagination.last }}          -- URL of last page
{{ pagination.items }}         -- items on current page
{{ <as> }}                     -- alias for pagination.items (name from pagination.as config)
```

The `pagination` and `as` variables are only present on pages with a `pagination` front matter block. Non-paginated pages do not receive them. The `as` variable name is user-defined (e.g., `as: articles` → `{{ articles }}`) and provides a convenient top-level alias for `pagination.items`.

---

## 4. Template Engine Architecture

### Abstraction Layer

```go
// Engine interface — Liquid is built-in, others plug in
type TemplateEngine interface {
    Parse(name string, content []byte) (Template, error)
    AddFilter(name string, fn FilterFunc) error
    AddTag(name string, fn TagFunc) error
}

type Template interface {
    Render(ctx map[string]interface{}) ([]byte, error)
}
```

**Tier 1 — Built-in (in-process Go, full speed):**
- **`liquid`** (default) — Notifuse/liquidgo. Familiar to 11ty/Jekyll/Shopify users.
- **`go`** — Go's `html/template` from the standard library. Familiar to Hugo users.

Both Tier 1 engines receive the same `map[string]any` context from the data cascade. No transformation needed. All built-in filters (slugify, date, default, etc.) are registered in both engines at startup. Shortcodes are registered internally via `RegisterShortcode`, which adds them as custom tags in Liquid (via `AddTag`) and as template functions in Go (via `FuncMap`) — a single registration call wires into both engines.

**Tier 3 — Plugin engines (Node subprocess, significant performance cost):**
- Any JS-based engine (Nunjucks, EJS, Pug, etc.) can be registered via the Node bridge.
- Every page render becomes an IPC round-trip. Expect 10-50x slower builds.
- No data read tracking (same as Tier 1 engines).

```
[alloy] WARN Template engine "nunjucks" runs via Node bridge.
        Build times will be significantly slower than built-in engines.
```

**Example: Nunjucks via Node plugin**

```yaml
# alloy.config.yaml
templates:
  engine: "nunjucks"           # Not built-in — triggers Node bridge
plugins:
  node: true
```

```javascript
// plugins/nunjucks-engine.js
import nunjucks from 'nunjucks';

export default function(alloy) {
  const env = new nunjucks.Environment(
    new nunjucks.FileSystemLoader('layouts')
  );

  alloy.engine("nunjucks", {
    // File extension for this engine's templates
    ext: ".njk",

    // Parse is a no-op — Nunjucks compiles on render
    parse(name, content) {
      return { name, content: content.toString() };
    },

    // Render a template with the page/site context
    render(template, context) {
      return env.renderString(template.content, context);
    },

    // Register custom filters (called at startup)
    addFilter(name, fn) {
      env.addFilter(name, fn);
    }
  });
}
```

Layouts use the `.njk` extension:
```
layouts/
├── default.njk
├── post.njk
└── partials/
    └── header.njk
```

Alloy sends each page's template + context to the Node bridge, the plugin renders it with Nunjucks, and returns the HTML. The same `TemplateEngine` contract — `parse`, `render`, `addFilter` — just fulfilled over IPC instead of in-process.

### Layout Lookup Order

Alloy does not auto-assign layouts based on directory structure. Layout resolution follows an explicit, predictable chain — no `single` vs `list` distinction, no section-type guessing.

At each step, the Liquid engine checks for `.liquid` first then bare extension. The Go engine uses bare extension directly.

**Blog-like section** (permalink has date tokens, e.g., `blog: "/:year/:month/:day/:slug/"`):

*Index file (`content/blog/index.html`):*
1. `layout:` from front matter / `_data.yaml` cascade (explicit override)
2. `layouts/blog.liquid` (section name from permalink config)
3. `layouts/index.liquid` (filename match)
4. `layouts/default.liquid` (fallback)
5. Build error

*Child file (`content/blog/my-post.md`):*
1. `layout:` from front matter / `_data.yaml` cascade (explicit override)
2. `layouts/post.liquid` (child of a date-based permalink section)
3. `layouts/my-post.liquid` (filename match)
4. `layouts/default.liquid` (fallback)
5. Build error

**Regular section or standalone pages** (permalink without date tokens, e.g., `docs: "/docs/:slug/"`, or no permalink entry):

*Any file (`content/docs/getting-started.md`):*
1. `layout:` from front matter / `_data.yaml` cascade (explicit override)
2. `layouts/getting-started.liquid` (filename match)
3. `layouts/default.liquid` (fallback)
4. Build error

The `post.liquid` convention only applies to children of sections with date-based permalink patterns. All other pages resolve through explicit `layout:`, filename match, or `default.liquid`.

### Layouts, Partials, Shortcodes

**Liquid engine:**

```liquid
<!-- layouts/default.liquid -->
<!DOCTYPE html>
<html>
<head><title>{{ page.title }}</title></head>
<body>
  {% include "partials/header" %}
  {{ content }}
  {% include "partials/footer" %}
</body>
</html>
```

**Go engine:**

```html
<!-- layouts/default.html -->
<!DOCTYPE html>
<html>
<head><title>{{ .page.title }}</title></head>
<body>
  {{ template "partials/header" . }}
  {{ .content }}
  {{ template "partials/footer" . }}
</body>
</html>
```

Same layout directory, same lookup order, same data context — different syntax.

**Layout chaining** — Layouts can reference a parent layout via front matter `layout:` directives. The build pipeline renders inside-out: page content → innermost layout → parent layout → ... → root layout. Each level injects `{{ content }}` from the level below. This enables multi-level composition (e.g., `page → has-toc → base`):

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

The pipeline strips layout front matter before rendering (it is not output as literal text). Layout front matter is only used for the `layout:` directive — other front matter keys in layouts are ignored.

**Circular layout detection** — Before rendering, `DetectCircularLayouts(layoutsDir)` scans all layout files for parent references and fails the build if a cycle is found (e.g., `a → b → a`). This runs once during Phase 0 validation, not per-page.

**Max depth** — Layout chains are capped at 10 levels. If a chain exceeds this depth without reaching a root layout (one with no `layout:` front matter), the build fails with an error identifying the chain. This prevents infinite loops from malformed layouts that escape cycle detection.

**Partials and includes** are delegated to each engine's native capabilities:
- **Liquid:** `{% include "partial" %}` and `{% render "partial" %}` for partials. Resolves from the layouts directory. **Plugin-registered filters must work in partials** — the same filter dispatch mechanism that applies to content templates and layouts must also apply to included files. If the engine uses template pre-processing (e.g., rewriting novel filter names for dispatch), that pre-processing must also run on partial source before parsing (issue #376).
- **Go:** `{{ block "name" . }}` / `{{ define "name" }}` for layout inheritance, `{{ template "name" . }}` for includes. Full layout chaining is built into the engine.

**Content-relative file inlining** — `{% inline "./path" %}` reads a file relative to the current content file's directory and inserts its raw contents into the output. No template processing — the file is inserted verbatim. This is an Alloy-specific tag, not part of the Liquid spec.

```markdown
<!-- content/about/index.md -->
# About

{% inline "./about-diagram.svg" %}
```

With the directory structure:
```
content/about/
├── index.md
└── about-diagram.svg
```

The SVG markup is inlined directly into the rendered HTML. This is essential for SVGs that need to respond to CSS custom properties (theming) and cannot be loaded as `<img>` tags.

**Path rules:**
- Path must start with `./` or `../` — always relative to the content file's directory
- Absolute paths are a build error
- File not found is a build error (not silent empty output)
- **Path sandboxing** — After resolving the relative path, the result must be within the content root directory. Paths that traverse outside content (e.g., `../../../../etc/passwd`) are a build error. The check: `filepath.Rel(contentRoot, resolvedPath)` must not start with `..`. This allows `../shared.svg` when it stays inside `content/` but blocks escaping to the filesystem.

**Allowed file types** (text-based only):
`.svg`, `.html`, `.htm`, `.txt`, `.css`, `.js`, `.json`, `.xml`, `.toml`, `.yaml`, `.yml`, `.md`

Binary file types (`.png`, `.jpg`, `.gif`, `.webp`, `.woff2`, `.pdf`, etc.) produce a build error with guidance: `"inline: binary file type .png not supported — use <img> instead"`.

**Future option:** The allowlist is hardcoded for v1. If users need custom text extensions (e.g., `.glsl`, `.webc`), a config-driven allowlist can be added later:
```yaml
templates:
  inline:
    allow: [".svg", ".html", ".glsl", ".webc"]
```

**Raw insertion** — The inlined content is NOT processed through Markdown or Liquid. No template tag interpretation, no Markdown rendering. The raw file bytes (as UTF-8 text) are inserted at the tag position. This prevents accidental Liquid parsing of content like `{{x}}` in SVG attributes or JavaScript files.

**Registration** — `{% inline %}` is registered via `engine.AddTag("inline", inlineTagFunc)` during engine setup, the same mechanism used for plugin shortcodes. The tag function receives the content file's directory path from the render context to resolve relative paths.

**Scope** — `{% inline %}` is content-scoped: relative paths are always resolved from the current content file's directory via the render context. If `{% inline %}` appears inside a layout while rendering a page, it still resolves against that page's content directory, not the layout file's directory. For layout-relative partials, use `{% include %}` instead.

**Shortcodes** are reusable content snippets that accept arguments and output HTML. They're used in content files to embed rich elements without writing raw HTML.

**Usage in content:**

```liquid
<!-- Liquid -->
{% youtube "dQw4w9WgXcQ" %}
{% callout "warning" %}Don't do this in production.{% endcallout %}
```

```html
<!-- Go -->
{{ youtube "dQw4w9WgXcQ" }}
{{ callout "warning" "Don't do this in production." }}
```

**Tier 3 (Node plugin):**

Tier 3 plugins are loaded via ESM `import()` — the project **must** have `"type": "module"` in its `package.json`. The bridge sends the plugin's absolute file path to the Node subprocess, which calls `await import(path)` and then `mod.default(alloy)`. Node's module cache ensures each dependency is loaded once, preventing side-effect collisions (e.g., duplicate `customElements.define` calls). CJS (`require()`) is not supported for Tier 3 plugins.

```javascript
// plugins/shortcodes.js
export default function(alloy) {
  alloy.shortcode("youtube", (args) => {
    const id = args[0];
    return `<iframe src="https://www.youtube.com/embed/${id}" frameborder="0" allowfullscreen></iframe>`;
  });

  // Block shortcodes receive inner content
  alloy.shortcode("callout", (args, content) => {
    const type = args[0];
    return `<div class="callout callout--${type}">${content}</div>`;
  });
}
```

**User registration — Tier 2 (in-process plugin):**

Tier 2 shortcodes are pure computation — no filesystem, no network. The simplest path is a plain JS file on embedded QuickJS (no build step). For maximum performance, compile to WASM.

**JS (QuickJS — no build step):**

```javascript
// plugins/shortcodes.js — drop in plugins/, immediately available
export default function(alloy) {
    alloy.shortcode("youtube", (args) => {
        const id = args[0];
        return `<iframe src="https://www.youtube.com/embed/${id}" frameborder="0" allowfullscreen></iframe>`;
    });

    alloy.shortcode("callout", (args, content) => {
        const level = args[0];
        return `<div class="callout callout--${level}">${content}</div>`;
    });
}
```

**Rust (compiled WASM — best performance):**

```rust
// plugins/shortcodes.rs (Rust → .wasm via wasm-pack)
use alloy_plugin::*;

#[alloy_shortcode("youtube")]
fn youtube(args: Vec<&str>) -> String {
    let id = args[0];
    format!(
        r#"<iframe src="https://www.youtube.com/embed/{}" frameborder="0" allowfullscreen></iframe>"#,
        id
    )
}

#[alloy_shortcode("callout")]
fn callout(args: Vec<&str>, content: &str) -> String {
    let level = args[0];
    format!(r#"<div class="callout callout--{}">{}</div>"#, level, content)
}
```

**Go (compiled WASM via TinyGo):**

```go
// plugins/shortcodes.go (Go → .wasm via TinyGo)
package main

import "fmt"

//export register
func register(alloy *Alloy) {
    alloy.Shortcode("youtube", func(args []string) string {
        return fmt.Sprintf(
            `<iframe src="https://www.youtube.com/embed/%s" frameborder="0" allowfullscreen></iframe>`,
            args[0],
        )
    })

    alloy.Shortcode("callout", func(args []string, content string) string {
        return fmt.Sprintf(`<div class="callout callout--%s">%s</div>`, args[0], content)
    })
}
```

JS plugins run on embedded QuickJS (~10-50µs per call). Compiled WASM (Rust, AssemblyScript, TinyGo) runs native WASM instructions (~1-10µs per call). All run in-process via wazero.

The only difference between tiers is execution speed and what capabilities the shortcode needs (filesystem, network, npm packages → Tier 3; pure computation → Tier 2).

---

## 5. Plugin System — Tiered Runtime

### Architecture

Alloy uses a tiered plugin runtime. Plugin authors write in their preferred language. Alloy routes execution to the appropriate tier based on what the plugin needs.

```
┌─────────────────────────────────────────────────┐
│              Alloy Plugin Runtime                │
├─────────────────────────────────────────────────┤
│                                                  │
│  Tier 1: Go Built-in           (~nanoseconds)    │
│  Compiled into binary.                           │
│  url, slugify, date, upcase, downcase, default   │
│                                                  │
│  Tier 2: In-Process Plugins    (~microseconds)   │
│  Sandboxed, no system access. Two flavors:       │
│  • JS plugins — plain .js on embedded QuickJS    │
│  • WASM plugins — .wasm from Rust/AS/TinyGo      │
│  Both run via wazero (pure Go, zero CGo).        │
│                                                  │
│  Tier 3: Node Subprocess       (~milliseconds)   │
│  For plugins needing Node APIs or native addons. │
│  PostCSS/cssnano (npm), Sharp (C++ addon),       │
│  Lit SSR (Node vm module). Opt-in, not default.  │
│                                                  │
└─────────────────────────────────────────────────┘
```

### Plugin Load Order and Conflicts

**Load order**: Plugins are loaded in alphabetical filename order within `plugins/`. Tier 1 (Go built-in) filters are registered first, then Tier 2 (`.js` and `.wasm` files), then Tier 3 (`.js` files with `runtime: "node"`).

**Name conflicts**: If two plugins register the same filter or shortcode name, the last one loaded wins. Alloy logs a warning so the user knows:

```
[alloy] WARN Filter "slugify" registered by plugins/custom-slugify.wasm
        overwrites built-in filter "slugify"
```

**Hook execution order**: Hooks execute by priority (lower runs first), then by alphabetical filename order within the same priority. Default priority is 50. Each hook receives the output of the previous one — they chain, not race.

```javascript
// alloy.hook(event, options, fn)
// options is required — declares what data the hook needs

// Runs first (priority 10) — onPageRendered receives HTML string, pages/pageFields don't apply
alloy.hook("onPageRendered", { priority: 10 }, transformsFn);

// Runs second (default priority 50)
alloy.hook("onPageRendered", {}, analyticsFn);

// Runs last (priority 100)
alloy.hook("onPageRendered", { priority: 100 }, ssrFn);
```

The `priority` option is available on all lifecycle hooks. Omitting it defaults to 50. The `alloy.on()` alias follows the same signature: `alloy.on(event, options, fn)`.

#### Hook scoping options

The options object is required on `alloy.hook()` and `alloy.on()`. It declares what subset of site data and pages the hook needs. The pipeline uses these declarations to serialize only the requested subset — avoiding full-site serialization on every hook dispatch.

| Option | Type | Default | Description |
|---|---|---|---|
| `priority` | `number` | `50` | Execution order (lower runs first) |
| `data` | `string[]` | `undefined` (skip) | Site data keys to include. `["*"]` = all keys. `undefined`/omitted = no site data. |
| `pages` | `boolean \| string \| object` | `false` | Page filtering mode. `false` = no pages. `true` or `"**"` = all pages. `"/blog/**"` = glob filter. `{ tags: ["component"] }` = taxonomy filter. |
| `pageFields` | `string[]` | `undefined` (all) | Per-page fields to include when pages are requested. `["frontMatter", "url"]` = only those fields. `undefined`/omitted = all fields. `["*"]` = all fields (explicit). |

**Pages option modes**:

| Value | Mode | Serialization |
|---|---|---|
| `false` (default) | Skip | No pages serialized |
| `true` or `"**"` | All | All pages, all requested fields |
| `"/blog/**"` | Glob | Only pages matching the path glob |
| `{ tags: ["component", "form"] }` | Taxonomy | Only pages matching taxonomy terms |

**Taxonomy filtering rules**: Multiple terms within the same taxonomy are OR'd (union) — a page tagged `component` OR `form` matches. Multiple taxonomies are AND'd (intersection) — `{ tags: ["component"], category: ["ui"] }` matches pages tagged `component` AND categorized `ui`. Taxonomy filtering is only available on hooks that fire after taxonomy collection (see hook availability matrix below).

**`addPages` return shape**: When a hook declares `pages: false` and needs to inject virtual pages (e.g., `onPagesReady` generating pages from site data), the return value uses `{ addPages: [...] }` instead of returning a full pages array. This avoids round-tripping hundreds of existing pages through the plugin bridge:

```javascript
alloy.hook('onPagesReady', { data: ["elements"], pages: false }, function(payload) {
  var elements = payload.siteData.elements || [];
  var newPages = [];
  for (var i = 0; i < elements.length; i++) {
    var el = elements[i];
    newPages.push({
      path: 'demos/' + el.slug + '.md',
      url: '/demos/' + el.slug + '/',
      frontMatter: { title: el.name + ' Demo', layout: 'demo', tags: [el.tagName] },
      content: '## ' + el.name + '\n\n' + el.description
    });
  }
  return { addPages: newPages };
});
```

#### Union scope and field visibility

When multiple hooks register for the same event with different `pageFields`, Alloy computes a single broadest-union scope and serializes that union for all hooks on the event. If hook A requests `pageFields: ["html"]` and hook B requests `pageFields: ["toc"]`, both hooks receive pages with `html` AND `toc` populated. The same union logic applies to `data` keys and page filtering modes.

This is a deliberate performance tradeoff: serializing one union payload per event is significantly cheaper than serializing a separate payload per hook. No data is withheld that a hook requested — every hook receives at least what it declared.

**Scoping is not an isolation boundary.** Plugin authors should not rely on `pageFields` or `data` scoping to prevent other plugins from seeing their requested fields. Scoping reduces serialization cost; it does not enforce privacy between plugins. A plugin that requests `pageFields: ["frontMatter"]` may still receive `html`, `toc`, or other fields if another plugin on the same event requested them.

#### Hook availability matrix

Not all page filtering modes are available on all hooks. Two constraints apply:

1. **Pageless hooks** (`onConfig`, `onBeforeValidation`, `onAfterValidation`, `onDataFetched`) do not receive pages — any page scope mode other than `PagesScopeNone` (`pages: false`) is rejected with a validation error. `data` and `pageFields` are also meaningless on these hooks.
2. **Pre-taxonomy hooks** that do receive pages (`onPagesReady`) cannot use taxonomy filtering because taxonomy indices are built during step 10 (`BuildTaxonomies`).

Per-page hooks (`onContentTransformed`, `onPageRendered`) fire once per page with a fixed payload shape — `pages`/`pageFields` scope does not apply (the pipeline already knows which page to serialize). `onPageRendered` receives an HTML string, not a page object. Scope options on per-page hooks are limited to `data` (site data subset) and `priority`.

| Hook | Pipeline step | Pages in payload | Glob filter | Taxonomy filter |
|---|:---:|:---:|:---:|:---:|
| `onConfig` | 2 | no | **error** | **error** |
| `onBeforeValidation` | 4 | no | **error** | **error** |
| `onAfterValidation` | 5 | no | **error** | **error** |
| `onDataFetched` | 6 | no | **error** | **error** |
| `onPagesReady` | 9 | yes (batch) | yes | **error** — taxonomy indices not built yet |
| `onContentLoaded` | 11+ | yes (batch) | yes | yes |
| `onDataCascadeReady` | 11+ | yes (batch) | yes | yes |
| `onContentTransformed` | 12+ | yes (per-page) | n/a | n/a |
| `onPageRendered` | 14+ | yes (per-page) | n/a | n/a |
| `onAssetProcess` | 16+ | no (per-asset) | n/a | n/a |
| `onBuildComplete` | 24 | no | n/a | n/a |
| `onDevServerStart` | — | no | n/a | n/a |
| `onFileChanged` | — | no | n/a | n/a |

Registering a page scope mode on a pageless hook, or a taxonomy filter on a pre-taxonomy hook, produces a validation error at plugin load time.

### Tier 1: Go Built-in Filters

Built-in filters covering common SSG needs. Compiled Go functions registered with both Liquid (via liquidgo) and Go templates (via `FuncMap`) — fastest possible execution:

- **String**: `upcase`, `downcase`, `capitalize`, `slugify`, `truncate`, `truncatewords`, `strip_html`, `escape`, `replace`, `replace_first`, `split`, `join`, `strip`, `append`, `prepend`, `newline_to_br`, `contains`
- **Date**: `date` (strftime format string, e.g. `{{ page.date | date: "%B %d, %Y" }}`). Powered by `github.com/lestrrat-go/strftime` for full POSIX compliance. Supports all standard directives (`%A` through `%z`, `%%`). Both Liquid and Go template engines use the same `DateFormat` implementation — overrides liquidgo's native `date` filter. Accepts `time.Time` or string input (parsed from ISO 8601, RFC 3339, `YYYY-MM-DD HH:MM:SS`, `YYYY-MM-DD`). Returns input unchanged when no format argument is provided.
- **Array**: `sort`, `reverse`, `first`, `last`, `where`, `group_by`, `size`, `map`, `uniq`, `compact`, `concat`
- **Set operations**: `intersect`, `union`, `complement`
- **URL**: `url`, `absolute_url`, `url_encode`, `url_decode`
- **Math**: `plus`, `minus`, `times`, `divided_by`, `modulo`, `ceil`, `floor`, `round`, `abs`
- **Content**: `markdownify` — renders a markdown string to HTML using the same goldmark configuration as the main content renderer, driven by the site's `content.markdown` config. Uses a shared goldmark instance, not per-call allocation. Does not run template tag protection (processes already-rendered values). Useful for rendering markdown in front matter fields (`{{ page.description | markdownify }}`), data file values, or any template string containing markdown syntax.
- **Output safety**: `safeHTML` (bypass auto-escaping for trusted content — relevant for Go templates)
- **Regex**: `findRE`, `replaceRE` (regex match and replace)
- **Data**: `json` (serialize value to JSON), `default` (fallback value if nil/empty)
- **Assets**: `fingerprint` (content-hash fingerprinting, e.g. `{{ 'css/main.css' | fingerprint }}` → `/css/main.abc123.css`)

### Tier 2: In-Process Plugins (via wazero)

For custom filters, shortcodes, and data transforms that are pure computation — takes input, returns output, no system access needed. Alloy provides two flavors that both run in-process via **wazero** (pure Go WebAssembly runtime, zero CGo):

| Flavor | Author writes | Runs on | Per-call | Build step |
|---|---|---|---|---|
| **JS plugins** | Plain `.js` files | Embedded QuickJS (shared instance) | ~10-50µs | None — drop in `plugins/` |
| **WASM plugins** | Compiled `.wasm` binaries | Native WASM instructions | ~1-10µs | Compile with language toolchain |

JS plugins are the low-friction path — write JavaScript, drop it in `plugins/`, done. WASM plugins are for authors who need maximum performance or prefer Rust/Go/AssemblyScript.

#### Discovery

Alloy scans `plugins/` at startup and loads by file extension:

```
plugins/
├── word-count.js          # JS → loaded into embedded QuickJS
├── custom-slugify.wasm    # WASM → loaded directly into wazero
└── css-minifier.js        # .js with npm imports? → picked up by Node bridge (Tier 3)
```

No config needed — drop a file in `plugins/` and it's active. To disable a plugin, remove or rename the file.

**JS vs Node disambiguation**: A `.js` file in `plugins/` runs on embedded QuickJS (Tier 2) by default. If the plugin uses Node-specific APIs (`fs`, `child_process`, `net`) or npm imports, it must export `{ runtime: "node" }` to signal that it needs the Tier 3 Node bridge instead:

```javascript
// plugins/css-minifier.js — needs npm packages, runs on Node (Tier 3)
export const runtime = "node";
import postcss from 'postcss';
// ...
```

Without this marker, `.js` plugins run sandboxed on QuickJS.

#### JS Plugins (QuickJS)

Alloy embeds a single QuickJS instance (compiled to WASM, running on wazero). Plain `.js` files are evaluated in this shared context at startup. ~10-50ms one-time startup cost, ~2-4MB memory.

```javascript
// plugins/word-count.js — drop in plugins/, immediately available
export default function(alloy) {
    alloy.filter("wordCount", (content) => {
        return content.split(/\s+/).filter(w => w.length > 0).length;
    });

    alloy.shortcode("youtube", (args) => {
        const id = args[0];
        return `<iframe src="https://www.youtube.com/embed/${id}" frameborder="0" allowfullscreen></iframe>`;
    });
}
```

Now `{{ page.content | wordCount }}` and `{% youtube "dQw4w9WgXcQ" %}` work in templates. No compilation, no config.

**Site data access** — Plugins can access global data from `data/` files via `alloy.data`:

```javascript
// plugins/status-tag.js
export default function(alloy) {
    alloy.shortcode("statusTag", (args) => {
        const key = args[0];
        const legend = alloy.data.statusLegend;  // from data/statusLegend.yaml
        const entry = legend[key];
        return `<rh-tag color="${entry.color}" icon="${entry.icon}">${entry.pretty}</rh-tag>`;
    });
}
```

`alloy.data` is a read-only snapshot of `site.data` injected into the QuickJS context after data files are loaded. It's the same data available in templates as `site.data.*`. The data is JSON-serialized from Go and parsed in JS — complex types (maps, arrays, strings, numbers, booleans) are preserved. Changes to `alloy.data` inside a plugin do not affect other plugins or the template data cascade.

**Important:** `alloy.data` is set after plugin files are evaluated. Access it inside filter, shortcode, and hook functions — not at the top level of your plugin file. Top-level `alloy.data` access during eval will be `undefined`.

**Modifying data:** `alloy.data` is read-only — mutations within JS don't propagate back to Go or affect templates. To add or modify data that templates see, use hooks like `onDataFetched` or `onAfterValidation` which receive the data, let you modify it, and return it to the pipeline. See "Data mutation via hooks" in the Lifecycle Events section.

For Node (Tier 3) plugins, `alloy.data` is sent as part of the bridge initialization message after data loading completes.

#### WASM Plugins (Compiled)

For maximum performance, plugin authors can compile to native WASM instructions. These run ~5-10x faster than QuickJS-interpreted JS — worth it for plugins called thousands of times per build (e.g., a filter applied to every page).

| Language | Compiler | Build command |
|---|---|---|
| AssemblyScript | asc | `asc src/word-count.ts -o plugins/word-count.wasm` |
| Rust | wasm-pack | `wasm-pack build --target bundler -d ../plugins/` |
| Go | TinyGo | `tinygo build -o plugins/word-count.wasm -target wasi .` |

#### WASM Calling Convention (ABI)

WASM modules operate on linear memory — they cannot access Go's heap directly. All data exchange happens through the module's own memory via a pointer/length convention:

**Required exports:**

```
alloc(size i32) -> ptr i32              # Allocate memory for host to write input
filter(ptr i32, len i32) -> (ptr i32, len i32)   # String filter: read input at ptr, return result ptr+len
```

**Calling sequence (host → WASM):**

1. Host calls `alloc(inputLen)` to get a safe write offset in WASM memory
2. Host writes input bytes to WASM memory at the returned pointer
3. Host calls `filter(ptr, len)` — WASM reads input, processes, writes result
4. WASM returns `(resultPtr, resultLen)` — host reads result bytes from WASM memory

The `alloc` export is required to avoid writing at hardcoded memory offsets that could collide with the module's data section, stack, or heap. Modules compiled with Rust (wasm-bindgen), TinyGo, or AssemblyScript can implement `alloc` as a simple bump allocator or delegate to their runtime's allocator.

**Data format per export:**
- **`filter`**: Input and output are raw UTF-8 strings. The filter receives the value to transform and returns the transformed value.
- **`shortcode`**: Input is a JSON object: `{ "name": "youtube", "args": ["abc123"], "content": "" }`. Output is a UTF-8 HTML string.
- **`hook`**: Input is a JSON payload (shape depends on the hook — see Lifecycle Events). Output is the modified JSON payload.

**Error handling:** If any export returns `(0, 0)`, the host treats it as a plugin execution error and propagates the failure according to the normal pipeline error policy (build aborts in `alloy build`/`alloy serve`, error overlay in `alloy dev`). If the module exports `last_error()`, the host reads and surfaces the error details. No silent fallback to original input — consistent with the error handling policy in §2.

**Optional exports:**

```
shortcode(ptr i32, len i32) -> (ptr i32, len i32)   # Shortcode: input is JSON { name, args, content }
hooks() -> (ptr i32, len i32)                        # Hook discovery: returns JSON array of hook name strings
hook(ptr i32, len i32) -> (ptr i32, len i32)         # Hook execution: input is JSON payload with event name
last_error() -> (ptr i32, len i32)                   # Error details; host calls after any export returns (0, 0)
```

**Hook discovery and execution:** The `hooks` export is called once during `LoadModule` (no input arguments, same pattern as `last_error`). It returns a JSON array of hook name strings (e.g., `["onContentTransformed", "onBuildComplete"]`). If no `hooks` export exists, the module has no hooks. The `hook` export receives a JSON object with an `"event"` key containing the hook name. **Payload wrapping:** When the hook payload is a map/object (e.g., `onContentTransformed` with `{html, toc, path, ...}`), the `"event"` key is merged into it: `{"event": "onContentTransformed", "html": "<p>test</p>", ...}`. When the payload is a primitive (e.g., `onPageRendered` with an HTML string), it is wrapped: `{"event": "onPageRendered", "payload": "<p>test</p>"}`. The module dispatches internally by event name and returns the (possibly modified) payload JSON. The host strips the `"event"` key before applying changes. WASM hooks get default priority 50 — no mechanism for per-hook priority in the export ABI.

**Hook error handling:** The `(0, 0)` error convention applies to both `hooks()` and `hook()` exports — if either returns `(0, 0)`, the host checks `last_error()` and surfaces the error. Additionally: (1) If `hooks()` returns data that is not a valid JSON array of strings (malformed JSON, a non-array JSON value, or an array containing non-string elements), `LoadModule` must return an error — do not silently treat the module as hook-less, because that masks a broken `hooks()` export the plugin author needs to fix. (2) If `hook()` returns a valid `(ptr, len)` but the bytes are not valid JSON, `CallHook` must return an error — do not silently fall back to the original payload.

#### Sandboxing

Both JS and WASM plugins run in isolated memory spaces via wazero. They cannot access the filesystem, network, or any system resources unless explicitly granted by Alloy. Safe to run untrusted community plugins.

#### Error Surfacing

Tier 2 plugins run through multiple layers (Go → wazero → QuickJS → user code for JS plugins, Go → wazero → WASM for compiled plugins). Errors must be translated into clear, actionable messages for the user — not raw stack traces from the WASM runtime. Alloy must:
- Map QuickJS errors back to the plugin filename and line number
- Include the filter/shortcode name and input value that caused the failure
- Surface WASM trap messages (out of bounds, unreachable) with the plugin name and which call triggered it
- In dev mode, display plugin errors in the browser error overlay alongside template and content errors

#### Warning Propagation via EvalWarner

Runtimes that collect non-fatal warnings during plugin evaluation (duplicate hook registrations, deprecated API usage) implement the `EvalWarner` interface (`EvalWarnings() []string`). When `registerRuntime` processes a runtime, it checks for `EvalWarner` and forwards all accumulated warnings to `HookRegistry.Warnings()` with a `"plugin <name>: "` prefix. This surfaces runtime-level warnings through the same `HookRegistry.Warnings()` API used for timeout warnings, giving the build pipeline a single warning channel. Both QuickJS and Node runtimes implement `EvalWarner`; WASM runtimes surface errors through the trap/`last_error()` mechanism instead.

#### Hook Serialization Boundary — Type Preservation

When hook payloads pass through the plugin serialization boundary (QuickJS VM or Node subprocess), JSON round-trip strips Go-specific types. In particular, `*ordered.Map` values (from JSON data files) become `map[string]interface{}`, losing insertion-order iteration (`Each()`) and property access (`LiquidMethodMissing()`). The host must deserialize hook return values through `ordered.UnmarshalJSONValue` (not standard `json.Unmarshal`) so JSON objects are restored as `*ordered.Map`. This applies to all runtimes (QuickJS, Node, WASM) at their respective deserialization points. Without this, any hook that returns structured data (e.g., `onDataFetched` returning the full `siteData`) silently destroys ordered map types for all keys — even those the plugin did not modify.

### Tier 3: Node Subprocess (opt-in)

For plugins that need Node-specific capabilities:
- **Node APIs**: `fs`, `child_process`, `vm`, `net`
- **Native addons**: C++ bindings via N-API (Sharp)
- **npm packages** that depend on the above

**Bring your own Node.** Alloy does not ship Node.js, manage `package.json`, or run `npm install`. The user is responsible for their Node version, dependencies, and package management. Alloy spawns whatever `node` is in PATH. If Node plugins exist but `node` is not found, the build fails with a clear error.

**Module resolution**: The bridge script is written to `.alloy/bridge.js` in the project root (not a temp directory) and the Node subprocess runs with `cwd` set to the project root. This ensures both ESM `import()` (which resolves relative to the importing module's URL) and CJS `require()` (which resolves relative to `cwd`) find packages in the project's `node_modules/`. Plugins can use any installed npm package — `@lit-labs/ssr`, `postcss`, `sharp`, etc. — without special configuration. The user installs packages with `npm install` in the project root, and plugins import them normally.

**Security: Tier 3 plugins run with the same permissions as the user.** They have full access to the filesystem, network, and environment variables. A plugin can read files, make HTTP requests, or spawn child processes — there is no sandbox. This is the same trust model used by 11ty, Jekyll, and every other plugin-based SSG: installing a plugin is an explicit statement of trust. Only install plugins you have reviewed or that come from trusted sources.

Alloy scans `plugins/` for `.js` and `.ts` files that export `runtime: "node"`. If any are found, it spawns subprocess workers that load all of them. If none are found, no Node process is started — zero overhead. Communicates via stdin/stdout using length-prefixed JSON-RPC (LSP-style framing). Plugin stderr is redirected to `.alloy/plugin.log`.

**Worker pool for per-page hooks (issue #491)**: For hooks that fire per page (`onPageRendered`, `onContentTransformed`), Alloy distributes pages across multiple subprocess workers. Each worker loads the same plugins and processes a contiguous chunk of pages. This parallelizes the single largest build cost — SSR and HTML transforms.

```yaml
plugins:
  workers: auto    # default — auto-scale based on CPU count
  # workers: 4    # explicit override
```

Auto-scaling: `min(runtime.NumCPU() / 2, 8)` with a floor of 2. `/2` because workers compete with Go's own goroutines. Cap at 8 to avoid IPC overhead diminishing returns. Workers are spawned async during pipeline init and shut down after the build completes. Only Tier 3 (subprocess) runtimes use the pool — Tier 2 (QuickJS/WASM) runs in-process.

**Unified Runtime interface**: Node plugins implement the same `Runtime` interface as Tier 2 plugins (QuickJS and WASM). `Registry.LoadPlugins()` treats all tiers identically — it iterates `RegisteredFilters`, `RegisteredShortcodes`, and `RegisteredHooks` from each runtime and bridges them to the template engine and `HookRegistry`. The `NodeRuntime` implementation routes these calls over JSON-RPC to the subprocess. The pipeline (via `Registry.LoadPlugins`) never knows whether a filter, shortcode, or hook is running in-process or in a subprocess.

```
┌──────────┐  Length-prefixed JSON / stdin+stdout  ┌──────────────┐
│  Alloy   │ ◄───────────────────────────────────► │  Node Runner  │
│  (Go)    │                                       │  (child proc) │
└──────────┘       stderr → .alloy/plugin.log      └──────────────┘
```

**Plugin API (Node side)**:

```javascript
// plugins/css-minifier.js — uses PostCSS + cssnano (npm packages)
import postcss from 'postcss';
import cssnano from 'cssnano';

export default function(alloy) {
  alloy.on("onAssetProcess", {}, async (file) => {
    if (file.path.endsWith('.css')) {
      const result = await postcss([cssnano]).process(file.content, { from: file.path });
      return { ...file, content: result.css };
    }
    return file;
  });
}
```

**Message protocol** — length-prefixed JSON over stdin/stdout (LSP-style):

Each message is framed with a `Content-Length` header followed by `\r\n\r\n` and the JSON body. This avoids the newline-delimited JSON problem where HTML payloads containing literal newlines would break the framing. Plugin `console.log` output goes to stderr (redirected to `.alloy/plugin.log`), keeping stdout clean for the protocol.

```
Content-Length: 82\r\n
\r\n
{"id": 1, "type": "hook", "name": "onContentTransformed", "payload": [...]}
```

**Message types:**

```json
// Go → Node: Event hook
{"id": 1, "type": "hook", "name": "onContentTransformed", "payload": [...]}

// Node → Go: Response
{"id": 1, "result": [...]}

// Go → Node: Filter call (proxy for Node-registered filters)
{"id": 3, "type": "filter", "name": "customFilter", "input": "value"}

// Node → Go: Filter response
{"id": 3, "result": "transformed value"}
```

### Lifecycle Events (all tiers)

Hooks receive JSON-serializable payloads so they work across all plugin tiers (Go built-in, QuickJS, WASM, Node). Go struct pointers are not visible to JS or WASM — the pipeline must serialize before calling and deserialize the return value.

#### Per-page hooks (HTML string)

These fire **once per page**. `onContentTransformed` receives a page-scoped object (mutable — fires before layout, page data still matters). The payload contains only page data — no `site`, `collections`, or `taxonomies`. Site-level mutations belong in `onConfig` or `onAfterValidation`. `onPageRendered` receives an HTML string (post-processing only — page is already rendered).

| Event | Payload | Returns | When |
|---|---|---|---|
| `onContentTransformed` | `{ html, toc, path, url, frontMatter }` | Same shape (mutable) | After Markdown→HTML, before layout. Plugin modifies rendered content, TOC, or front matter. |
| `onPageRendered` | HTML string (complete page after layout) | HTML string | After template rendering. Plugin post-processes final output. |

```javascript
// Example: add lazy loading + build TOC for non-markdown pages
alloy.hook("onContentTransformed", {}, (page) => {
  page.html = page.html.replace(/<img /g, '<img loading="lazy" ');
  
  // Build TOC from HTML for pages that didn't go through goldmark
  if (!page.toc || page.toc.length === 0) {
    page.toc = extractHeadingsFromHTML(page.html);
  }
  
  return page;
});

// Example: minify final HTML (post-processing, no page data needed)
// onPageRendered receives an HTML string, not a page object — pages/pageFields do not apply
alloy.hook("onPageRendered", {}, (html) => {
  return html.replace(/\s+/g, ' ').trim();
});
```

#### Per-asset hook (path + content object)

Fires **once per asset file**. Payload is a JSON object with `path` and `content`. Return value replaces the asset content.

| Event | Payload | Returns | When |
|---|---|---|---|
| `onAssetProcess` | `{ path: string, content: string }` | `{ content: string }` | During asset copy. Plugin transforms asset content. |

```javascript
// Example: CSS minification
alloy.hook("onAssetProcess", {}, (asset) => {
  if (asset.path.endsWith('.css')) {
    return { content: minifyCSS(asset.content) };
  }
  return asset;
});
```

#### Pre-taxonomy hook (JSON objects)

Fires **once per language batch** after data cascade but before taxonomy collection. This is the injection point for data-driven pages that need taxonomy participation — Alloy's equivalent of Hugo's content adapters.

| Event | Payload | Returns | When |
|---|---|---|---|
| `onPagesReady` | `{ pages: [{ path, url, frontMatter: { ... }, content: "..." }, ...], siteData: { ... } }` | Same shape (may include additional virtual pages appended to `pages`) | After data cascade, before taxonomy collection. Plugin injects virtual pages with front matter (including taxonomy terms). Per-batch firing avoids #521. |

```javascript
// plugins/data-pages.js — Generate per-element demo pages from data
export default function(alloy) {
  alloy.hook('onPagesReady', { data: ["elements"], pages: false }, function(payload) {
    var elements = payload.siteData.elements || [];
    var newPages = [];
    for (var i = 0; i < elements.length; i++) {
      var el = elements[i];
      newPages.push({
        path: 'demos/' + el.slug + '.md',
        url: '/demos/' + el.slug + '/',
        frontMatter: {
          title: el.name + ' Demo',
          layout: 'demo',
          tags: [el.tagName + '-tabs']
        },
        content: '## ' + el.name + '\n\n' + el.description
      });
    }
    return { addPages: newPages };
  });
}
```

**Distinction from `onContentLoaded`**: `onPagesReady` fires before taxonomy collection and content rendering — injected pages get taxonomy terms indexed and raw `content` rendered through the markdown pipeline. `onContentLoaded` fires after content rendering and is limited to **modifying existing pages** (`frontMatter` and `html` are mutable; `content`, `path`, `url` are present for inspection but mutations are not applied back). It cannot inject virtual pages — the return array must be the same length as the input and preserve the original order (indexed by position). If extra pages are appended, the pipeline produces a validation error. Use `onPagesReady` for all virtual page injection; use `onContentLoaded` for post-render front matter enrichment and HTML post-processing.

This also resolves #521 (virtual pages appended to wrong language batch): since `onContentLoaded` no longer supports virtual page injection, the batch routing problem is eliminated. `onPagesReady` fires per language batch inside `applyBatchContext()`, so injected pages are always in the correct batch.

#### Content hooks (JSON objects)

| Event | Payload | Returns | When |
|---|---|---|---|
| `onContentLoaded` | `[{ path, url, frontMatter: { ... }, content: "...", html: "..." }, ...]` | Same shape, same length, same order (modify `frontMatter` and `html` on existing pages) | After content rendering. Fires once with full pages array. Plugin modifies `frontMatter` and `html` on existing pages — `content`, `path`, `url` are present for inspection but mutations are not applied back. Return array must equal input length and preserve order (indexed by position). Extra pages produce a validation error — use `onPagesReady` for injection (#525). |
| `onDataCascadeReady` | `[{ path, data: { ... } }, ...]` | Same shape, same length | After cascade resolved. Fires once with full pages array (each entry has per-page cascade data). Plugin enriches cascade data. |

#### Per-build hooks (JSON objects)

Fire **once per build**. Payload is a JSON-serializable representation of the Go type. The pipeline converts Go structs to `map[string]interface{}` before calling, and applies returned changes back.

| Event | Payload | Returns | When |
|---|---|---|---|
| `onConfig` | `{ title, baseURL, build: { output, clean }, ... }` | Same shape | After config loaded. Plugin mutates config. |
| `onBeforeValidation` | `{ paths: ["/about/", "/blog/", ...] }` | Same + additions | Before conflict detection. Plugin adds output paths. |
| `onAfterValidation` | `{ paths: [...], cascade: { ... } }` | Cascade portion only | After validation. Plugin injects cascade data. |
| `onDataFetched` | `{ <sourceName>: <data>, ... }` | Same shape | After external data fetched. Plugin modifies fetched data. |

**Data mutation via hooks** — To modify site data that templates see, use per-build hooks. The hook receives the data object, modifies it, and returns it. The pipeline applies the returned value. This is the only way to add or change data that flows into templates — `alloy.data` in filters/shortcodes is read-only.

**Virtual page injection (issues #518, #525)** — `onPagesReady` is the only hook that supports virtual page injection. Virtual pages are appended to the `pages` array in the returned payload. Required fields: `path` (source-relative identifier, e.g. `demos/button.md` — used as `RelPath` and `RenderedContent` key) and `url` (permalink, e.g. `/demos/button/` — used for output path computation). Optional: `frontMatter` (including `layout` and taxonomy terms like `tags`), `content` (raw markdown — rendered through the content pipeline). Virtual pages flow through the full remaining pipeline: taxonomy collection, content rendering, layout resolution, template rendering, and output writing. `layout: false` skips layout wrapping. Output-path collisions between a virtual page and a real page produce a build error (e.g., `/demos/button/` and an existing page that writes to the same output file). Missing `path`/`url` produces a validation error. Virtual pages are included in `PageCount`. `onContentLoaded` cannot inject virtual pages — it is limited to modifying `frontMatter` and `html` on existing pages.

```javascript
// plugins/enrich-data.js
export default function(alloy) {
    // Add computed data after external sources are fetched
    alloy.on("onDataFetched", { data: ["team"] }, (data) => {
        if (data.team) {
            data.teamCount = data.team.length;
            data.teamByDepartment = {};
            for (const member of data.team) {
                const dept = member.department || "unassigned";
                if (!data.teamByDepartment[dept]) data.teamByDepartment[dept] = [];
                data.teamByDepartment[dept].push(member);
            }
        }
        return data;  // returned value replaces the pipeline's data
    });

    // Inject data into the cascade after validation
    alloy.on("onAfterValidation", {}, (payload) => {
        payload.cascade.buildTimestamp = new Date().toISOString();
        return payload;  // cascade changes are applied
    });
}
```

Templates can then use `{{ site.data.teamCount }}`, `{{ site.data.teamByDepartment }}`, and `{{ site.data.buildTimestamp }}`.

#### Read-only hooks (return value ignored)

Fire **once per event**. Plugins observe but cannot modify.

| Event | Payload | When |
|---|---|---|
| `onBuildComplete` | `{ pageCount: 42, duration: "127ms", errors: [] }` | After output written. |
| `onDevServerStart` | `{ port: 3000, url: "http://localhost:3000" }` | Dev server ready. |
| `onFileChanged` | `"content/blog/post.md"` (string) | File changed in watch mode. |

### Performance Safeguards

- **Timeout**: Each plugin hook has a configurable timeout (default 5s). If exceeded, build continues without that plugin's modifications and logs a warning. Hook functions receive a `context.Context` as their first parameter carrying the timeout deadline. Hooks should check `ctx.Done()` for cooperative cancellation — a timed-out hook's goroutine will not be forcefully killed, but the context signals that its result will be discarded.
- **Batching**: Pages are sent to Node plugins in batches (not one-at-a-time) to amortize IPC overhead. WASM plugins process in-process so batching is unnecessary.
- **Caching**: Node plugin results are cached by content hash. If content hasn't changed, skip the IPC call.
- **Lazy start**: Node process only spawns if Node plugins exist. WASM plugins load in ~1ms.
- **Process reuse**: In dev mode, Node process stays alive between rebuilds.

---

## 6. Content Processing — Two-Phase Rendering

### The Problem This Solves

In 11ty (and similar SSGs), changing a shared component invalidates every page that uses it, causing a full rebuild + full SSR pass. With 720+ pages and design-system components used everywhere, this makes incremental builds useless. The root cause: SSR is coupled to page rendering, and the dependency graph doesn't distinguish data-flow edges from build-ordering edges.

Alloy solves this with **two-phase rendering** and **deduplicated SSR**.

### Phase 1: Content → HTML (Markdown + Template Engine, Two-Pass Render)

```
Source File → Front Matter Extract → Format Detect → Markdown Parse → Merge into Layout → Template Render → Intermediate HTML
```

1. **Front matter extraction**: Split YAML/TOML header from body (delimited by `---` or `+++`)
2. **Format detection**: By file extension (`.md`, `.html`, `.txt`)
3. **Markdown parsing** (`.md` files only):
   - goldmark (CommonMark + extensions: tables, footnotes, task lists, typographer, auto heading IDs, heading attributes)
   - `html.WithUnsafe()` enabled — raw HTML blocks pass through untouched
   - Template tag extensions — `{{ ... }}` and `{% ... %}` patterns are emitted as custom AST nodes (inline or block) and rendered verbatim regardless of the `unsafe` setting. No special delimiters needed.
   - `.html` → no Markdown processing (already HTML)
   - `.txt` → wrap in `<pre>` or passthrough based on config
4. **Content template rendering**: The content body (Markdown-rendered or raw HTML) is rendered through the template engine with page data + site data context, producing a rendered HTML string. Variables assigned in content are scoped to this pass.
5. **Layout rendering**: The rendered content HTML is injected into the resolved layout as `{{ content }}` (Liquid) or `{{ .content }}` (Go). The layout is then rendered with the same page data + site data context. Content and layout have isolated scopes — no variable leakage between them. Error line numbers map cleanly to their source file.

**Template tags in Markdown** work automatically — no special syntax or delimiters needed:

```markdown
# My Article

Published on {{ page.date | date: "%B %d, %Y" }}.

{% for post in collections.blog %}
- [{{ post.title }}]({{ post.url }})
{% endfor %}

<div class="summary" hidden>
  <p>{{ page.description }}</p>
</div>
```

Two custom goldmark extensions handle template tags in markdown:

**Inline TemplateTags extension** — An inline parser that recognizes `{{ }}` and `{% %}` patterns and emits them as custom `TemplateTagInline` AST nodes (not `ast.RawHTML` — template tags must be preserved regardless of the `unsafe` setting). A custom renderer outputs the tag text verbatim, bypassing goldmark's HTML sanitization. Works for inline shortcodes (`{% youtube "id" %}`) and output tags (`{{ page.title }}`).

**Block TemplateBlocks extension** — A block parser that treats any `{% ... %}` tag occupying a line by itself as a block-level node. Each such line becomes an independent single-line block — the parser does not track open/close pairing. Tag pairing (`{% tagname %}` / `{% endtagname %}`) is semantic and resolved by the Liquid template engine, not by goldmark. The tags are emitted as custom AST block nodes (not `ast.RawHTML` — template tags must be preserved regardless of the `unsafe` setting). A custom renderer outputs the tag text verbatim, bypassing goldmark's HTML sanitization. Content between paired tags is normal markdown (bold, lists, etc. work). This prevents block shortcodes that produce `<div>`, `<section>`, or other block-level HTML from being invalidly nested inside `<p>` tags.

```markdown
<!-- Each {% %} line is an independent block node — goldmark doesn't pair them -->
{% callout "warning" %}
This has **bold** text.

- List item one
- List item two
{% endcallout %}

<!-- This inline shortcode is fine inside a paragraph -->
Watch this video: {% youtube "abc123" %}
```

The block extension activates when:
1. A line starts with `{% tagname` (with optional arguments)
2. The tag is on its own line (not mixed with other inline content)

When the tag is embedded in a line with other text, the inline TemplateTags extension handles it instead — the tag stays inline and `<p>` wrapping is correct.

**Escaping:** To show literal `{{ }}` or `{% %}` in prose without template engine processing:
- **Liquid engine:** wrap in `{% raw %}...{% endraw %}`
- **Go engine:** use `{{ "{{" }}` to output a literal `{{` (standard Go template escaping)
- **Both engines:** goldmark's inline code (backticks) and fenced code blocks protect their contents from the auto-detection extension — goldmark's parsers take precedence.

### Markdown Render Hooks

Render hooks override how specific markdown element types are rendered to HTML. Instead of goldmark's default output, a Liquid template controls the HTML for that element type. Templates live in `layouts/_markup/`:

```
layouts/_markup/
├── render-blockquote.liquid     # blockquotes (>)
├── render-codeblock.liquid      # fenced code blocks (```)
├── render-codeblock-mermaid.liquid  # language-specific: mermaid code blocks
├── render-heading.liquid        # headings (#, ##, ###)
├── render-image.liquid          # images (![alt](src))
├── render-link.liquid           # links ([text](url))
└── render-table.liquid          # tables (| ... |)
```

If a render hook template exists, Alloy registers a custom goldmark node renderer that delegates to the template instead of emitting default HTML. If no template exists, default goldmark rendering applies. Render hooks run during Phase 1 (markdown rendering) — before template tag processing and layout rendering. Alloy scans `layouts/_markup/` at startup and registers renderers for any templates found.

**Engine selection** — Render hook templates follow the configured template engine. With `templates.engine: "liquid"` (default), hooks are `.liquid` files. With `templates.engine: "gotemplate"`, hooks are `.html` files (e.g., `render-codeblock.html`). The hook template syntax matches the engine — `{{ markup.language }}` in Liquid, `{{ .markup.language }}` in Go templates.

**Template context** — Each render hook template receives a `markup` object with element-specific properties:

| Template | `markup.*` properties |
|---|---|
| `render-blockquote` | `inner` (rendered inner HTML), `attributes` |
| `render-codeblock` | `inner` (raw code text), `language`, `attributes` |
| `render-heading` | `inner` (rendered inner HTML), `level` (1-6), `id` (auto-generated slug via `slugify` — e.g., "My Section" → `my-section`), `text` (plain text, no HTML) |
| `render-image` | `src`, `alt`, `title`, `attributes` |
| `render-link` | `destination`, `text` (rendered inner HTML), `title`, `is_external` (boolean: starts with `http://` or `https://`) |
| `render-table` | `inner` (rendered inner HTML — thead/tbody/tr/td), `attributes` |

The full `page.*` and `site.*` context is also available — render hooks can access front matter, site data, collections, etc.

**Language-specific code block hooks** — `render-codeblock-{language}.liquid` overrides rendering for a specific fenced code block language. For example, `render-codeblock-mermaid.liquid` renders mermaid blocks as `<div class="mermaid">` instead of `<pre><code>`. The generic `render-codeblock.liquid` is the fallback when no language-specific template matches. Lookup order: language-specific → generic → default goldmark rendering.

**Example: custom code block rendering**

```liquid
<!-- layouts/_markup/render-codeblock.liquid -->
<rh-code-block language="{{ markup.language }}">
  <script type="text/{{ markup.language }}">{{ markup.inner }}</script>
</rh-code-block>
```

**Example: external link detection**

```liquid
<!-- layouts/_markup/render-link.liquid -->
{% if markup.is_external %}
  <a href="{{ markup.destination }}" target="_blank" rel="noopener">{{ markup.text }} <svg class="external-icon">...</svg></a>
{% else %}
  <a href="{{ markup.destination }}">{{ markup.text }}</a>
{% endif %}
```

**Template tag escaping** — The pipeline's `escapeTemplateTagsInCode` step protects `{{ }}`/`{% %}` inside `<code>` elements from Liquid processing. When a render hook replaces `<pre><code>` with a different structure (e.g., `<rh-code-block><script>`), the hook's `markup.inner` content is already escaped by goldmark. The hook template receives pre-escaped code content — no additional escaping needed.

**Implementation** — Custom goldmark node renderers are registered for each supported element type. At startup, the pipeline scans `layouts/_markup/` for render hook templates. For each found template, a custom renderer is registered that calls the Liquid engine with the node's context instead of emitting default HTML. The template is parsed once at startup and reused for every matching node.

**Cache key**: `hash(source_content + front_matter + layout + data_cascade_snapshot)`

Phase 1 output is **intermediate HTML** — it contains raw Web Component tags (`<ds-button>`, `<ds-card>`, etc.) that haven't been SSR'd yet.

### Phase 2: SSR Transform (opt-in — Web Components → Declarative Shadow DOM)

**This phase only runs if `ssr:` is configured.** Without it, Phase 1 output is the final HTML and Web Components render client-side only. Most sites won't need this — it's specifically for projects using Web Components that want server-side rendered Declarative Shadow DOM.

```
Intermediate HTML → Per-Page SSR Command → Final HTML (with DSD)
                 → Scan for Components → Track page-to-component mapping (for cache)
```

1. **Per-page SSR**: For each page, extract the inner content of `<body>` (not the tag itself) and pipe it to the configured `ssr.command` via stdin. The SSR engine (golit) reads stdin, handles all component discovery, rendering, and DSD injection internally, and writes the transformed body content with Declarative Shadow DOM to stdout. Alloy re-inserts the SSR'd body content into the original document skeleton (`<!DOCTYPE>`, `<html>`, `<head>`, `<body>`).
2. **Component tracking**: Before SSR, scan each page's intermediate HTML for custom element tags (anything with a hyphen in the tag name per the HTML spec). Record which pages use which components in `.alloy/components.json` for cache invalidation.

### SSR Command

Alloy does **not** import any SSR engine as a Go dependency. The SSR engine is an external CLI binary that must be installed separately. The `ssr:` config tells Alloy what command to invoke and how to communicate with it:

```yaml
# alloy.config.yaml — exec mode (default)
ssr:
  command: "golit render --defs ./bundles"

# stream mode — persistent process, NUL-delimited
ssr:
  command: "golit serve --stdio"
  mode: "stream"
```

**`mode`** controls how Alloy communicates with the SSR engine:

- **`exec`** (default, can be omitted) — Alloy spawns a new process per page. Pipes page HTML to stdin, reads transformed HTML from stdout, process exits. Simple, no state between pages.
- **`stream`** — Alloy starts the process once and keeps it alive. For each page, writes HTML + `\0` (NUL byte) to the process's stdin, reads until `\0` on stdout. The process stays warm across all pages, amortizing startup cost. Significantly faster for sites with many pages.

**Per-page rendering** — Regardless of mode, the contract is the same: body inner content goes in via stdin, SSR'd body content comes out via stdout. Alloy extracts the content between `<body>` and `</body>` before piping, and re-inserts the result into the original document skeleton after receiving the response. The SSR engine never sees `<!DOCTYPE>`, `<html>`, `<head>`, or `<body>` tags — only the body's inner HTML containing the custom elements.

```
# exec mode — one process per page (body content only)
echo '<h1>Hello</h1><ds-card title="Hi">content</ds-card>' | golit render --defs ./bundles

# stream mode — persistent process, NUL-delimited (body content only)
# Alloy writes: <h1>Hello</h1><ds-card>...</ds-card>\0
# Alloy reads:  <h1>Hello</h1><ds-card><template shadowrootmode="open">...</template>...</ds-card>\0
# (process stays alive until build completes or dev server stops)
```

This separation ensures:
- The document skeleton (`<head>`, `<script>`, import maps) is preserved by Alloy, not the SSR engine
- The SSR engine only processes what it needs — body HTML with custom elements
- SSR engine authors don't need to worry about preserving document structure

The NUL byte (`\0`) is used as the stream delimiter because valid HTML cannot contain it — per the HTML spec (§13.2.2), NUL characters in HTML input are parse errors replaced with U+FFFD. This makes `\0` an unambiguous, zero-overhead message boundary.

The SSR engine owns all component-level concerns: element discovery, deduplication, shadow root rendering, and DSD insertion. Alloy treats it as a black box — body content goes in, SSR'd body content comes out. The document skeleton is Alloy's responsibility.

If the engine binary is not found:

```
[alloy] ERROR SSR command failed: "golit" not found in PATH.
        Install golit or remove the ssr: block from alloy.config.yaml.
        Build aborted.
```

Everything else — component discovery, ignore lists, import maps, concurrency — is configured by the engine's own config (e.g., `golit.yaml`). Alloy doesn't validate or proxy engine settings — if the command returns an error, Alloy surfaces it and aborts.

### SSR Timeout

Per-page rendering is subject to a timeout. If the SSR engine does not return a result within the timeout, Alloy kills the process and reports the failure.

```yaml
ssr:
  command: "golit serve --stdio"
  mode: "stream"
  timeout: "30s"    # per-page timeout, default 30s
```

**`timeout`** is a duration string (e.g., `"30s"`, `"1m"`, `"500ms"`). Default is `30s`. Applies to both exec and stream modes. In exec mode, the process is killed on timeout. In stream mode, the process is killed and recovery is attempted (see below).

### Stream Error Recovery

In stream mode, the persistent process can fail in several ways: crash (unexpected exit), timeout (no response within `ssr.timeout`), or malformed output (no NUL terminator). Alloy handles all three the same way:

1. **Detect**: The process exited, the read timed out, or the output is malformed.
2. **Restart**: Start a new instance of the same command.
3. **Retry**: Re-send the failed page to the new process.
4. **Limit**: If the same page fails twice, skip it with an error and continue with remaining pages. One bad page should not abort a 500-page build.

```
[alloy] ERROR SSR timeout: "content/blog/huge-post.md" did not complete within 30s.
        Restarting SSR process and retrying...
[alloy] ERROR SSR retry failed for "content/blog/huge-post.md". Skipping page.
        (249 of 250 pages rendered successfully)
```

In exec mode, there is no recovery — each page is an independent process. A timeout or non-zero exit fails that page. The build continues with remaining pages and reports all failures at the end.

### Component Tracking for Cache Invalidation

Before passing each page to the SSR command, Alloy scans the intermediate HTML for custom element tags and records which pages use which components. This enables targeted rebuilds when a component source file changes.

**Tracking map** stored in `.alloy/components.json`:

```json
{
  "pageToComponents": {
    "content/blog/post-1.md": ["ds-card", "ds-button"],
    "content/about.html": ["ds-card"]
  },
  "componentToPages": {
    "ds-card": ["content/blog/post-1.md", "content/about.html"],
    "ds-button": ["content/blog/post-1.md"]
  },
  "definitionHashes": {
    "ds-card": "abc123",
    "ds-button": "def456"
  }
}
```

When a component source file changes (detected by file watcher or definition hash comparison), Alloy rebuilds all pages listed in `componentToPages` for that component. Pages not using the changed component are skipped entirely.

### Why This Matters for Incremental Builds

| What changed | Phase 1 | Phase 2 | Pages rebuilt |
|---|---|---|---|
| A blog post's content | Re-render that post | Re-SSR that page | 1 |
| A Liquid layout | Re-render pages using that layout | Re-SSR affected pages | N (layout users) |
| `<ds-button>` component def | Skip entirely | Re-SSR all pages using `<ds-button>` (via component tracking) | 0 re-rendered, M re-SSR'd |
| Global data file | Re-render all pages that could be affected | Re-SSR only if Phase 1 output changed | N (all potential readers) |

### Markdown Library: goldmark

- Pure Go, fast, CommonMark compliant
- Extensible (custom renderers, AST transforms)
- Used by Hugo — proven at scale
- **Single shared instance** — goldmark is configured once from the site's `content.markdown` config, then reused across page body rendering (`RenderMarkdown`), TOC extraction (`RenderMarkdownWithTOC`), and the `markdownify` template filter. All three use the same config-driven extensions and parser options. If the site sets `autoHeadingID: false`, `markdownify` also skips heading IDs. `markdownify` does NOT run template tag protection (`protectTemplateTags`) or code escaping (`escapeTemplateTagsInCode`) — these are pipeline-level steps for raw content, not for already-rendered values that `markdownify` processes. `goldmark.Markdown.Convert` is safe for sequential reuse — no mutable state between calls.

---

## 7. Assets

Alloy has no built-in asset pipeline. Files in `assets/` are copied to `_site/` like `static/` — no transformation, no fingerprinting, no optimization out of the box.

All asset processing is handled by plugins via the `onAssetProcess` lifecycle hook. This keeps the core simple and avoids half-implementing what dedicated tools (PostCSS, esbuild, Sharp) do better.

**Example plugin use cases:**

| Task | Plugin approach |
|---|---|
| CSS minification | PostCSS + cssnano (Node plugin) |
| Tailwind CSS | Tailwind CLI (Node plugin) |
| JS bundling | esbuild/Rollup (Node plugin) |
| Image optimization | Sharp (Node plugin) |
| Fingerprinting | Content hash plugin (WASM or Node) |

### Asset references in templates

```liquid
<link rel="stylesheet" href="{{ 'css/main.css' | url }}">
<img src="{{ 'img/hero.jpg' | url }}">
```

The `url` filter resolves paths relative to `baseURL`.

---

## 8. Dev Server — Two Modes

Alloy's server has two modes. Both use the same built-in Go HTTP server and watch for changes. `alloy serve` writes to and serves from `_site/`, while `alloy dev` serves rendered pages from memory and static/passthrough files directly from source. The difference is what gets written and how updates reach the browser.

### `alloy dev` — Dev Mode

The daily driver for active development.

- **Phase 1 only** — Liquid + Markdown → HTML with raw component tags. No SSR, no DSD, regardless of config. `Build()` is called with `BuildOptions{SkipSSR: true}` so Phase 2 is skipped even when `ssr:` is configured.
- **Components render client-side** — Browser loads component JS and renders them normally.
- **Full page reload** — A small dev client (injected in dev mode only) connects via WebSocket. Any file change triggers a rebuild of affected pages and sends `{"type": "reload"}` to the browser.

**Future: Signals-Based HMR.** The architecture should preserve surface area for granular hot module replacement in a future version — binding markers in template output, per-property DOM patching, CSS hot-swap, and component reconstruction without full reload. For v1, full page reload is sufficient. The WebSocket infrastructure and file watcher are the same foundation HMR would build on.

### `alloy serve` — Production Server

For verifying production output locally. Runs the same pipeline as `alloy build` but keeps serving.

- **Phase 1 + Phase 2 (if SSR configured)** — Same output as `alloy build`. If no `ssr:` in config, Phase 2 is skipped.
- **Full page reload** — No HMR, no signals. WebSocket sends `{"type": "reload"}` on any change.
- **Production-like output** — DSD, SSR'd components, no dev markers or injected scripts (other than the reload script).
- **Watches for changes** — Rebuilds on file changes and reloads the page.

### `alloy build` — No Server

Same pipeline as `alloy serve`: Phase 1 + conditional Phase 2. Writes to `_site/` and exits. No server, no watching. This is what you deploy.

### SSR Is Always Opt-In

SSR only runs (in `alloy serve` and `alloy build`) when explicitly configured:

```yaml
# alloy.config.yaml
ssr:
  command: "your-ssr-engine render"

# No ssr: key → no SSR ever, even in serve or build
```

Without an `ssr:` config block, `alloy serve` still works — it just serves Phase 1 output with full reload (useful for verifying templates, data cascade, and layouts without dev tooling).

### Shared Server Features (both modes)

- **File watcher**: `fsnotify` with 50ms debounce. Watches `content/`, `layouts/`, `data/`, `assets/`, `static/`, passthrough `from:` directories, and component source dirs. Both `alloy dev` and `alloy serve` must set up watchers — `alloy serve` is NOT a one-shot build.
- **Bulk change protection**: If many files change at once (e.g., `git checkout`), trigger a full rebuild instead of N incremental ones.
- **Rebuild handler by change type** (applies to both `alloy dev` and `alloy serve`):
  - `ContentChange`, `LayoutChange`, `DataChange` → pipeline rebuild (incremental in dev, full in serve)
  - `AssetChange` → recopy assets to `_site/`
  - `StaticChange` → recopy static files to `_site/`
  - `PassthroughChange` → targeted recopy: determine which passthrough mapping the file belongs to, compute relative path within `from:`, copy only that file to `_site/<to>/<relative-path>`. In dev mode, no recopy needed (served from source) — just browser reload.
  - `ComponentChange` → SSR re-render of affected pages
  - All change types trigger a browser reload via WebSocket after the rebuild/recopy completes.
- **Passthrough targeted recopy** — `RecopyPassthroughFile(changedPath, cfg)` finds the matching passthrough mapping, computes the output path, and copies only the changed file. Does not re-run the pipeline or recopy the entire passthrough directory.
- **Dev mode (`alloy dev`)**: Rendered pages are held in an in-memory map — no `_site/` output written to disk, lower latency, no SSD wear. Source files (content, layouts, data, assets, static) are still read from disk normally. Static and passthrough files are served directly from their source locations (no copy). Content-colocated non-content files (SVGs, images, JS, etc. in `content/`) are also served directly from `content/` — the dev server's request handler falls back to the content directory for URLs that don't match a rendered page in memory. This ensures relative references like `<img src="./photo.png">` work without writing to `_site/`.
- **Serve mode (`alloy serve`)**: Writes to `_site/` and serves from disk. Production-like output including SSR. Must have the same file watcher setup as `alloy dev` — watches all directories, dispatches rebuilds by change type, triggers browser reload.
- **Build mode (`alloy build`)**: Always writes to `_site/`.
- **Port auto-increment**: If the requested port is occupied, the server tries up to 10 consecutive ports (e.g., 3000 → 3001 → … → 3009) before giving up with an error. A warning is logged for each skipped port (e.g., `[alloy] WARN Port 3000 in use, using 3001`). The startup message always shows the actual port. This matches the behavior of modern dev servers (Vite, Next.js) and reduces friction when multiple projects run simultaneously.
- **Auto-opens browser** (optional)
- **Colored terminal output** with build timing
- **Error overlay in browser** on build errors (file path, line number, code snippet). Disappears on next successful build.
- **Custom 404 page**: When a request doesn't match any file in the output directory, serve `404.html` from the output root (with a 404 status code) if it exists. Falls back to Go's default plain-text response when no `404.html` is present. The build requires no special handling — users create `content/404.md` with `permalink: /404.html` and it renders through a layout into `_site/404.html`. In dev mode, the 404 page receives the WebSocket reload script like any other page. This matches the behavior of GitHub Pages, Netlify, Cloudflare Pages, and every major hosting platform.

```
[alloy] 12:34:56 Serving at http://localhost:3000
[alloy] 12:34:58 Rebuilt 3 pages in 47ms (417 cached)
[alloy] 12:35:02 ERROR in layouts/blog/single.liquid:14
         Unknown filter "formattDate" — did you mean "formatDate"?
```

---

## 9. CLI Design

### Entry Point

`main.go` must call `cmd.Execute()` and exit non-zero on error. All command logic lives in `cmd/` — `main.go` is a thin entry point:

```go
package main

import (
	"os"

	"github.com/zeroedin/alloy/cmd"
)

func main() {
	if err := cmd.Execute(); err != nil {
		os.Exit(1)
	}
}
```

### Exit Codes

All CLI commands follow standard Unix conventions:
- **Exit 0** — command completed successfully
- **Exit 1** — command failed (invalid config, build error, missing resource, unknown command, etc.)

Scripts and CI pipelines rely on exit codes. The error return from `cmd.Execute()` must never be discarded.

### Commands

```
alloy init               # Create default alloy.config.yaml (fails if one already exists)
alloy build              # Run pipeline (Phase 1 + Phase 2 if SSR configured), write _site/, exit
alloy dev                # Dev mode: Phase 1, full page reload, client-side components, drafts visible
alloy serve              # Production server: same pipeline as build, served locally, full reload
alloy version            # Print version
alloy help               # Help text
```

#### `alloy init [directory]`

Creates a default `alloy.config.yaml` in the target directory (default: current directory).

- **Creates target directory** if it does not exist (`os.MkdirAll`).
- **Fails with exit 1** if `alloy.config.yaml` already exists. Error message must contain `"already exists"`.
- **Prints success message** on creation: `Created alloy.config.yaml` (or `Created <dir>/alloy.config.yaml` when a directory argument is given).
- **Generated config** must be valid for `config.Validate` — at minimum `title` and `baseURL`:

```yaml
title: "My Alloy Site"
baseURL: "http://localhost:3000"
```

#### `alloy build`

Runs the full build pipeline and writes output to `_site/` (or the configured output directory).

1. Detect and load config file (via `config.DetectConfigFile` + `config.LoadWithDefaults`). If no config file is found, fall back to built-in defaults so an empty project produces a successful zero-page build.
2. Apply CLI flag overrides via `config.MergeFlags` (`--output`, `--verbose`, `--quiet`).
3. If `--profile`, start pprof CPU profiling via `pipeline.StartProfiling(profileDir)`. Default `profileDir` is `.alloy/profiles`.
4. Call `pipeline.Build(cfg, pipeline.BuildOptions{Profile: profile})`. When `Profile` is true, `Build` records per-stage wall-clock timings in `BuildResult.StageTimings`.
5. If `--profile`, stop profiling (writes `cpu.prof` and `mem.prof` to `profileDir`), print stage timing table.
6. Print build summary: page count and duration (e.g., `Built 42 pages in 127ms`).
7. Exit 0 on success, exit 1 on any error.

#### `alloy dev`

Starts the development server with live reload. Phase 1 only, in-memory, drafts visible.

1. Load config (same as build).
2. Run initial build via `pipeline.Build(cfg, pipeline.BuildOptions{SkipSSR: true})` with `cfg.IncludeDrafts = true` (unless `--no-drafts`). Build persists cache to disk for subsequent incremental rebuilds.
3. Start HTTP server on `--port` (default 3000). If the port is occupied, auto-increment up to 10 consecutive ports. If all 10 are occupied, exit 1 with an error listing the range tried.
4. Start file watcher for live reload. On file changes, rebuild incrementally — only re-render changed/invalidated pages using the persisted build cache. Falls back to full rebuild for bulk changes (10+ files) or component changes. Always passes `SkipSSR: true`.
5. Print startup message: `Serving at http://localhost:<actual-port>` (always shows the actual port, which may differ from `--port` if auto-increment kicked in).
6. Block until interrupted (Ctrl+C).

#### `alloy serve`

Starts the production server. Same pipeline as `alloy build` but keeps serving with file watching. Writes to `_site/`, runs SSR if configured, excludes drafts.

1. Load config (same as build).
2. Run initial build via `pipeline.Build(cfg)` with `cfg.IncludeDrafts = false`.
3. Start HTTP server on `--port` (default 3000) with port auto-increment.
4. Start file watcher for live reload. On file changes, call `pipeline.Build(cfg)` — always full rebuild (no incremental in serve mode). Dispatch by change type: content/layout/data → pipeline rebuild, asset/static → recopy, passthrough → targeted recopy, component → full rebuild with SSR.
5. Print startup message.
6. Block until interrupted (Ctrl+C).

### Flags

```
--config, -c       Path to config file (default: alloy.config.yaml)
--root, -r         Project root directory (default: config file's directory)
--output, -o       Output directory (default: _site)
--verbose, -v      Verbose logging
--quiet, -q        Suppress output
--port, -p         Server port (default: 3000) — alloy dev and alloy serve
--no-drafts        Hide draft content (alloy dev only, drafts visible by default)
--refetch          Bypass source cache TTL, fetch fresh data on startup — alloy dev and alloy serve
--profile          Enable per-stage timing breakdown and pprof CPU/memory profiling — alloy build only
--profile-dir      Directory for profile output (default: .alloy/profiles) — requires --profile
```

### Build Progress Output

Build progress is displayed differently based on the output context and verbosity flags:

**Default (TTY — interactive terminal):**

A progress bar showing the current pipeline stage, percentage, page count, and the file being processed. The progress line overwrites itself (carriage return) for a clean single-line display. The final summary line persists after the build completes.

```
[alloy] Discovering content... 480 pages found
[alloy] Rendering    [=========================] 100% (480/480)
[alloy] Layouts      [=========================] 100% (480/480)
[alloy] Transforms   [=========================] 100% (480/480)
[alloy] Writing      [=========================] 100% (480/480)
[alloy] Built 480 pages in 12.3s
```

Pipeline stages shown in order:
1. **Discovering** — content discovery + front matter extraction. Total is unknown at start (discovery produces the count). Shown as a single-line message: `[alloy] Discovering content... 480 pages found` — not a progress bar.
2. **Rendering** — Markdown → HTML + content template rendering (Pass 1b)
3. **Layouts** — layout chain resolution and rendering (Pass 2)
4. **Transforms** — post-render hooks (`onPageRendered`). Reports per-page even with worker pools (#491).
5. **SSR** — Phase 2 SSR (only shown when `ssr:` is configured and pages have custom elements)
6. **Writing** — output files to `_site/`

For stages with a known total, each shows `[stageName] [progress bar] percentage (current/total) currentFile`. For stages where the total is unknown (discovery), a message-style output is used instead. The progress bar width adapts to terminal width. The current file name is truncated if it would exceed the terminal width.

**Default (non-TTY — piped output, CI/CD):**

No progress bar, no carriage returns. Only the final summary line:

```
[alloy] Built 420 pages in 1.8s
```

This keeps CI logs clean. Errors still print normally.

**`--verbose` flag:**

Per-file output replaces the progress bar. Each line shows the stage, file path, and per-file timing:

```
[alloy] render content/index.md (12ms)
[alloy] render content/blog/first-post.md (8ms)
[alloy] render content/blog/second-post.md (6ms)
[alloy] ssr    content/components/card.md (45ms)
[alloy] write  _site/index.html
[alloy] write  _site/blog/first-post/index.html
[alloy] Built 420 pages in 1.8s
```

Useful for identifying slow pages or debugging build issues. No progress bar — per-file lines and carriage-return progress bars would produce messy output.

**`--quiet` flag:**

No output at all except errors. Not even the summary line. Exit code communicates success/failure.

**Dev/serve mode — initial build:**

The initial `pipeline.Build()` called by `alloy dev` or `alloy serve` must attach a progress reporter using the same flag-based logic as `alloy build`:
- `--quiet` → nil
- `--verbose` → `VerboseProgress`
- default → `TTYProgress` if terminal, nil if piped

This is where the progress bar is most valuable — the user is watching the terminal waiting for the server to start. Without it, there is no output between running the command and seeing `Serving at http://localhost:3000`.

```
[alloy] Discovering content... 420 pages found
[alloy] Rendering  [========>                ] 34% (142/420) content/blog/my-post.md
[alloy] Built 420 pages in 1.8s
Serving at http://localhost:3000
```

**Dev mode — incremental rebuilds:**

Incremental rebuilds via `BuildIncremental()` (used by `alloy dev`) are typically 1-3 pages and complete in under 100ms. A multi-stage progress bar would be visual noise. `BuildIncremental()` only calls `Summary` on the reporter — no `StartStage`, `Update`, or `EndStage`:

```
[alloy] 12:34:58 Rebuilt 3 pages in 47ms (417 cached)
```

The timestamp prefix is added by the reporter's serve-mode `Summary` implementation, not by the pipeline.

Full rebuilds triggered by config changes or bulk file changes (10+ files) go through `Build()`, not `BuildIncremental()`, and show the full multi-stage progress bar.

#### `--root` flag behavior

By default, `ProjectRoot` is the directory containing the config file. The `--root` flag overrides this, setting `ProjectRoot` to the specified directory instead. All relative `structure:` paths and `build.output` resolve against `ProjectRoot`.

This is essential for CI/CD environments where the config file may live in a subdirectory (`deploy/production.yaml`) but the working directory is the project root:

```yaml
# Without --root: requires ../content, ../layouts, etc. in structure: overrides
alloy build --config deploy/production.yaml

# With --root: clean config, paths resolve from CWD
alloy build --config deploy/production.yaml --root .
```

When `--root` is not provided, the existing behavior is preserved: `ProjectRoot = filepath.Dir(configPath)`.

---

## 10. Performance Architecture

### Concurrency Model
- **Worker pool**: `runtime.NumCPU()` goroutines for file processing
- **Pipeline stages**: Each stage uses channels to pass work to the next
- **Template caching**: Parse templates once at startup, reuse compiled templates for every page render (liquidgo's parse-once/render-many)
- **Data caching**: Global and directory data loaded once, shared across all page renders (read-only after assembly)

### Caching Strategy
- `.alloy/` directory stores build cache
- `cache.json`: Content hashes for incremental detection
- `templates.cache`: Pre-parsed template ASTs (if serializable)
- `assets.cache`: Processed asset fingerprints

### Memory Management
- Stream large files rather than loading entirely into memory
- Limit concurrent file I/O to prevent fd exhaustion
- Reuse byte buffers via `sync.Pool` for template rendering

### Performance Targets (Aspirational)

These are goals, not guarantees. Actual performance depends on page complexity, number of components, plugin overhead, and SSR cost. Prior experience with golit as an 11ty plugin showed heavier pages taking 2-3 seconds — though much of that overhead was likely 11ty's data cascade and build pipeline, which Alloy's architecture is designed to eliminate.

- **1,000 pages (no SSR)**: < 5 seconds full build
- **1,000 pages (with SSR)**: < 10 seconds full build
- **Single file change (dev)**: < 200ms incremental rebuild
- **Cold start to first serve**: < 3 seconds

**Per-stage budgets** (diagnostic targets for 1,000 pages, no SSR):

| Stage | Budget | Notes |
|---|---|---|
| Config + discovery | < 200ms | Walk dirs, collect files, parse config |
| Front matter extraction | < 200ms | Parallel across workers |
| Data cascade assembly | < 100ms | Shared data loaded once, front matter per-file |
| Markdown rendering | < 500ms | goldmark, parallel across workers |
| Template rendering | < 1s | Parse once, render many — bulk of the work |
| Plugin hooks | < 2s | Dominated by Tier 3 IPC if present |
| Output writing | < 500ms | Parallel file writes |

These are guidelines for profiling, not hard limits. If a build exceeds the overall target, per-stage timing helps identify the bottleneck.

---

## 11. Testing Strategy — TDD (Red-Green-Refactor)

### Framework: Ginkgo + Gomega + testify mocks

- **Ginkgo**: BDD-style test structure (Describe/Context/It) that mirrors the spec document
- **Gomega**: Expressive matchers (Expect/To/Equal/ContainSubstring)
- **testify/mock**: Mock interfaces (SSREngine, TemplateEngine, Node bridge)
- **ginkgo watch**: Auto-runs tests on file save for tight red-green feedback loop

### TDD Workflow

Every spec section becomes a test suite. Write tests FIRST from the spec, then implement until green:

```
1. Read a requirement from PLAN.md
2. Write a failing test (red)
3. Implement the minimum code to pass (green)
4. Refactor
5. Repeat
```

### Test Organization

```
alloy/
├── internal/
│   ├── config/
│   │   ├── config.go
│   │   └── config_test.go          # Ginkgo suite
│   ├── content/
│   │   ├── discovery.go
│   │   ├── discovery_test.go
│   │   ├── frontmatter.go
│   │   ├── frontmatter_test.go
│   │   ├── markdown.go
│   │   └── markdown_test.go
│   ├── cascade/
│   │   ├── merge.go
│   │   └── merge_test.go
│   ├── template/
│   │   ├── engine.go
│   │   ├── engine_test.go
│   │   ├── layout.go
│   │   └── layout_test.go
│   ├── validation/
│   │   ├── conflicts.go            # Output path conflict detection
│   │   └── conflicts_test.go
│   ├── pipeline/
│   │   ├── build.go
│   │   └── build_test.go
│   ├── ssr/
│   │   ├── scanner.go              # Component scan + dedup
│   │   ├── scanner_test.go
│   │   ├── engine.go               # SSREngine interface
│   │   ├── depgraph.go             # Component dependency graph
│   │   └── depgraph_test.go
│   ├── plugin/
│   │   ├── wasm.go                 # Tier 2 WASM runtime
│   │   ├── wasm_test.go
│   │   ├── node.go                 # Tier 3 Node bridge
│   │   └── node_test.go
│   └── server/
│       ├── server.go
│       └── server_test.go
└── test/
    └── fixtures/                   # Test sites for integration tests
        ├── minimal/                # 3-page site
        ├── cascade/                # Tests data cascade merging
        ├── components/             # Tests SSR pipeline
        └── large/                  # Generated 1000+ pages for benchmarks
```

### Test Categories

**Unit tests** (per package, fast, mocked dependencies):
```go
var _ = Describe("Data Cascade", func() {
    Describe("merge rules", func() {
        It("deep-merges objects", func() { ... })
        It("replaces arrays", func() { ... })
        It("front matter wins over directory data", func() { ... })
    })
})
```

**Integration tests** (cross-package, use test fixture sites):
```go
var _ = Describe("Build Pipeline", func() {
    It("builds a minimal site end-to-end", func() {
        result := Build("test/fixtures/minimal")
        Expect(result.OutputDir).To(BeADirectory())
        Expect(filepath.Join(result.OutputDir, "index.html")).To(BeARegularFile())
    })
})
```

**Benchmark tests** (performance targets from spec):
```go
func BenchmarkBuild1000Pages(b *testing.B) {
    for i := 0; i < b.N; i++ {
        Build("test/fixtures/large")
    }
}
// Target: < 1 second for 1000 pages
```

### Key Dependencies

| Dependency | Purpose | License |
|---|---|---|
| `github.com/onsi/ginkgo/v2` | BDD test framework | MIT |
| `github.com/onsi/gomega` | Matcher library | MIT |
| `github.com/stretchr/testify/mock` | Interface mocking | MIT |

---

## 12. Implementation Phases

### Phase 1 — Foundation
- [ ] Initialize Go module (`go mod init`)
- [ ] CLI skeleton (`alloy init`, `alloy build`, `alloy dev`, `alloy serve`, `alloy version`, `alloy help`)
- [ ] Config file loading (YAML, TOML, JSON — detected by file extension)
- [ ] Content discovery (walk `content/` directory, collect files)
- [ ] Front matter extraction (YAML `---`, TOML `+++`, JSON `{` — detected by delimiter)
- [ ] Markdown rendering (goldmark with template tag auto-detection extension for `{{ }}`/`{% %}` patterns)
- [ ] Liquid compatibility test suite (verify liquidgo covers needed features: `forloop.parentloop`, whitespace control `{%-`/`-%}`, `{% render %}` scoping, `tablerow`, error modes — low overhead, run early to surface gaps before building on the library)
- [ ] Basic Liquid template rendering (liquidgo)
- [ ] Go template rendering (`html/template`)
- [ ] Output writing to `_site/`
- [ ] Static file handling (`static/` → `_site/` via file copy)
- [ ] Pre-build validation (output path conflict detection across all sources)

### Phase 2 — Content Model
- [ ] Data cascade (global → directory → front matter → computed)
- [ ] Data file loading (YAML, TOML, JSON, CSV in `data/` and `_data.*`)
- [ ] Layout lookup and resolution (front matter → permalink section convention → filename match → `default.liquid` fallback)
- [ ] Collections (sections, taxonomies)
- [ ] Taxonomy page generation (index + per-term pages, shared layout, `taxonomy` context object)
- [ ] Permalinks and URL generation (token system, front matter overrides, Liquid fallback, aliases)
- [ ] Pagination (paginated lists with `perPage > 1`, virtual pages with `perPage: 1`)
- [ ] Content lifecycle (draft — dev only, publishDate, expiryDate, summaries)
- [ ] Output formats (HTML, JSON, XML via template file extension)
- [ ] Auto-generated files (sitemap.xml with `sitemap: false` disable, feed.xml as opt-in templates)
- [ ] External data sources (built-in REST/GraphQL fetch, plugin source handlers, cache to `.alloy/fetch-cache/`)
- [ ] Passthrough copy (config-driven external directory mapping)
- [ ] i18n / multilingual (opt-in via `languages:` config, per-language content trees, shared layouts, translation linking)

### Phase 3 — SSR Pipeline
- [ ] SSR config parser (`ssr.command`, `ssr.mode`, `ssr.timeout`)
- [ ] Per-page SSR exec mode: spawn process per page, pipe HTML via stdin, read stdout
- [ ] Per-page SSR stream mode: persistent process, NUL-delimited stdin/stdout
- [ ] Per-page timeout: kill process after `ssr.timeout` (default 30s), report failing page
- [ ] Stream error recovery: restart process on crash/timeout, retry failed page once, skip on second failure
- [ ] Exec error isolation: failed page does not abort build, continue with remaining pages, report all failures
- [ ] Component tracking: scan pre-SSR HTML for custom element tags, record page-to-component mapping
- [ ] Component map persistence (`.alloy/components.json` — `pageToComponents`, `componentToPages`, `definitionHashes`)
- [ ] SSR cache: skip re-SSR for pages whose Phase 1 output hash is unchanged
- [ ] Component invalidation: rebuild pages using changed components (via `componentToPages` lookup)

### Phase 4 — Dev Experience
- [ ] Dev server (`alloy dev`) with file watching (fsnotify, 50ms debounce)
- [ ] WebSocket dev client for full page reload on file changes
- [ ] Production server (`alloy serve`) — same pipeline as build, served locally, full reload
- [ ] SSR engine process management in serve mode (spawn on start, kill on exit)
- [ ] Incremental builds with content-hash change detection
- [ ] Fine-grained invalidation (Phase 1 vs Phase 2 independence)
- [ ] Bulk change protection (many files at once → full rebuild instead of N incremental)
- [ ] `--no-drafts` flag (hide drafts in `alloy dev`)
- [ ] Error overlay in browser (file path, line number, code snippet)
- [ ] Error reporting in terminal (clear messages with file/line, colored output)

### Phase 5 — Plugin System
- [ ] Plugin directory scanning and file-extension routing (`.js` → QuickJS, `.wasm` → native WASM, `.js` with `runtime: "node"` → Node)
- [ ] Embedded QuickJS instance via wazero for JS plugins (Tier 2)
- [ ] WASM plugin loading via wazero for compiled plugins (Tier 2)
- [ ] Node bridge subprocess + length-prefixed JSON-RPC protocol (Tier 3, bring-your-own-Node, stderr → `.alloy/plugin.log`)
- [ ] Event hook system (all lifecycle events: `onConfig` through `onBuildComplete`)
- [ ] Filter/shortcode registration from plugins (all tiers)
- [ ] Plugin timeout and caching (Node tier)
- [ ] Lit SSR Node plugin example (SSR via `onPageRendered` hook)
- [ ] Example plugins: word-count (JS/QuickJS), css-minifier (Node), custom-slugify (WASM)

### Phase 6 — Assets
- [ ] `assets/` directory copy to `_site/` (no built-in transforms)
- [ ] `onAssetProcess` hook integration (plugin-driven transforms)
- [ ] `url` filter for asset path resolution relative to `baseURL`

### Phase 7 — Production Hardening
- [ ] Benchmarking suite (1k and 10k page test sites, with and without SSR)
- [ ] Memory profiling and optimization (shared data by pointer, `sync.Pool` for buffers)
- [ ] CI/CD pipeline
- [ ] Release versioning via [changesets](https://github.com/changesets/changesets) (semver, changelogs, `THIRD_PARTY_LICENSES` generation)
- [ ] Documentation site (built with Alloy)
- [ ] WASM plugin authoring guide — complete examples in Rust, TinyGo, and AssemblyScript showing the `alloc`/`filter` ABI contract, compile commands, and working plugin tests

---

## Verification

After each phase, verify by:
1. `alloy build` on a test site with increasing page counts (10, 100, 1000)
2. `alloy dev` with live reload — change a file, confirm < 200ms incremental rebuild
3. Run `go test ./...` — all tests pass
4. Run `go build` — single binary, no CGo dependencies
5. Benchmark: `time alloy build` on 1000-page test site, target < 5s (no SSR), < 10s (with SSR)

---

## Key Dependencies

| Dependency | Purpose | License |
|---|---|---|
| `github.com/Notifuse/liquidgo` | Liquid template engine | MIT |
| `github.com/yuin/goldmark` | Markdown → HTML | MIT |
| `github.com/fsnotify/fsnotify` | File watching (dev server) | BSD-3 |
| `github.com/spf13/cobra` | CLI framework | Apache-2.0 |
| `gopkg.in/yaml.v3` | YAML parsing | MIT |
| `github.com/BurntSushi/toml` | TOML parsing | MIT |
| `github.com/gorilla/websocket` | Live reload WebSocket | BSD-3 |
| `github.com/tetratelabs/wazero` | WASM runtime (Tier 2 plugins + embedded QuickJS) | Apache-2.0 |

### Licensing

Alloy is licensed under **MIT**. All dependencies use permissive licenses (MIT, BSD-3, Apache-2.0) that are compatible with MIT distribution.

**Requirements**: The compiled binary must bundle third-party license notices. Use `go-licenses` to collect and embed all dependency licenses into a `THIRD_PARTY_LICENSES` file included in every release. Apache-2.0 dependencies (cobra, wazero) also require including their NOTICE files if present. This should be automated as part of the release build process in CI.

---

## Future Features (Post-v1)

### Signals-Based HMR (Dev Mode)

Replace full page reload in `alloy dev` with granular hot module replacement. The v1 WebSocket and file watcher infrastructure is designed to support this upgrade path.

**Binding markers** — During dev-mode rendering, template outputs are tagged with data attributes:

```html
<h1 data-alloy="page.title">My Blog Post</h1>
<div class="content" data-alloy="page.content">
  <p>Rendered markdown...</p>
</div>
```

**Dev client** (~2-3KB, injected in dev mode only) connects via WebSocket and uses Preact Signals to reactively update bound DOM nodes:

```json
// Content change → patch in-place (no reload)
{"type": "patch", "updates": [{"bind": "page.content", "html": "<p>Updated...</p>"}]}

// Component source change → re-import module + reconstruct elements
{"type": "component-update", "tag": "ds-button"}

// CSS change → hot-swap stylesheet
{"type": "css", "path": "/assets/css/main.css"}

// Structural change (layout, config) → fallback to full reload
{"type": "reload"}
```

**Component updates** — When a component source file changes, the client re-imports the module with a cache-busted URL and reconstructs affected elements (remove from DOM, re-add with `cloneNode` to preserve light DOM/slots). No DSD or shadow root patching needed — the browser constructs the component fresh from its updated definition.

**Target rebuild costs by change type:**

| Change | Work done | Target time |
|---|---|---|
| Content file (.md) | Re-render 1 page, patch via WebSocket | ~10-50ms |
| Front matter | Re-render 1 page, patch bindings | ~10-50ms |
| Component source | No rebuild, just notify client | ~5-10ms |
| Stylesheet | No rebuild, hot-swap | ~5ms |
| Layout/template | Re-render affected pages, full reload | ~50-200ms |
| Data file | Re-render pages that read it, patch | ~20-100ms |
| Config | Full rebuild + reload | ~500ms+ |

### Data Read Tracking (TrackedDrop)

Track which data keys each page actually reads during template rendering, enabling precise incremental rebuilds when shared data changes. Without this, shared data changes (global data files, `_data.yaml`, collections) rebuild all potentially affected pages.

**Concept:** Wrap the template context in a proxy object that intercepts property access, records the key, and returns the value. After rendering, each page has a read set (e.g., `["site.title", "collections.blog"]`). On rebuild, only pages whose read set includes the changed data key are re-rendered.

**Challenge:** Requires liquidgo to route all property resolution through an interceptable interface rather than direct map access. liquidgo does not currently support this. Implementation would require forking liquidgo or contributing an upstream change to add a `Drop`-like interface with property access interception (similar to Ruby Liquid's `method_missing`). Go has no language-level equivalent of `method_missing`, so this must be an explicit library design choice.

**Impact:** At v1 performance targets (1,000 pages < 5s), full rebuilds on shared data changes are acceptable since they're infrequent. This becomes more valuable at scale (10k+ pages) or for sites with frequently-changing external data sources.

### Parallel-Safe Plugin Hooks

v1 plugin hooks are synchronous barriers — all pages batch through each hook before the next stage. This is simple and safe but can become a bottleneck when multiple Node plugins hook the same event across thousands of pages. Explore allowing hooks to declare themselves parallel-safe, enabling pipelining — start the next stage for pages that have already passed all hooks while earlier pages are still being processed.

### `alloy check` Command

A validation-only command that reuses Phase 0 logic without running a full build. Useful in CI to catch errors early. Could validate: front matter schemas, broken internal links, missing layouts, unused data files, output path conflicts, and taxonomy/collection integrity.


### ~~`alloy serve --preview` Flag Naming~~ (Resolved)

Resolved in #256: split into `alloy dev` (development) and `alloy serve` (production). The `--preview` flag is removed.

### `alloy init` Scaffolding

Extend `alloy init` beyond creating a bare `alloy.config.yaml` to optionally scaffold directory structure, starter templates, and `.gitignore`. Could use Go's `//go:embed` to bundle starter files in the binary (no network needed). Deferred because users have their own opinions about directory naming and structure — less is more for v1.

### Asset Exclude Globs

Section 7 defines no built-in ignore/exclude mechanism for assets — all filtering is delegated to the `onAssetProcess` plugin hook. In practice, every project has files that should never reach production output: OS metadata (`.DS_Store`, `Thumbs.db`), source maps (`*.map`), partial SCSS files (`_*.scss`), and build artifacts. Requiring a plugin for this is unnecessary friction.

**Proposed config:**

```yaml
build:
  assets:
    exclude: ["_*", ".DS_Store", "*.map", "*.scss"]
```

Glob patterns matched against the relative path within `assets/`. Matched files are skipped before `onAssetProcess` hooks fire — plugins never see them. This keeps the plugin hook focused on transformation while giving users simple declarative control over what gets copied. Every major SSG (Hugo, 11ty, Jekyll) provides equivalent functionality.

### `--act-as-datetime` Flag (Time Travel for Dev Server)

Allow the dev server to behave as if it were running at a specified future (or past) date/time. This enables previewing future-`publishDate` content and testing `expiryDate` behavior without setting `draft: true` on every page.

```bash
alloy dev --act-as-datetime="2026-12-25T00:00:00Z"
```

All lifecycle filtering (`publishDate`, `expiryDate`) uses the provided datetime instead of `time.Now()`. Collections are built accordingly. The terminal and browser overlay show a banner indicating the simulated time. This is a dev-mode-only feature — `alloy build` and `alloy serve` always use real time.

### Reconsider Error Overlay Approach

The current spec (S8) defines an HTML error overlay injected into failed pages during `alloy dev`. While this is dev-mode only (never in `alloy build` or `alloy serve` output), injecting HTML the user didn't write raises concerns:

- Could interfere with component rendering and layout debugging
- Adds complexity to the dev server (HTML injection, error state tracking, overlay dismissal)
- Is opinionated — some developers prefer terminal-only errors and find overlays intrusive

Consider moving build failure reporting to terminal-only output, with structured error messages (file path, line number, code snippet, pipeline stage) displayed in the terminal where `alloy dev` is running. The WebSocket dev client could optionally `console.error()` the details in the browser's DevTools instead of rendering an overlay. This keeps the browser output clean and avoids injecting any HTML the user didn't author, while still surfacing errors where the developer can see them.

### Alternative Template Engine: pongo2

If `Notifuse/liquidgo` proves insufficient (missing features, upstream abandonment, incompatible architecture for TrackedDrop), evaluate [pongo2](https://pongo2.dev/docs/getting-started) as a replacement. pongo2 is a Go-native template engine implementing Django/Jinja2-style syntax. It shares the same delimiters as Liquid (`{{ }}` for output, `{% %}` for logic) but follows the Django template tradition instead of Ruby Liquid.

**Advantages over liquidgo:**
- 60+ built-in filters and 24 template tags out of the box
- Native template inheritance (`{% extends %}` / `{% block %}`) — Liquid lacks this
- Sandbox mode for safely rendering user-supplied templates
- Auto-escaping for XSS prevention
- MIT licensed, mature Go project

**Tradeoffs:**
- Django/Jinja2 filter syntax (`|filter:arg`) differs slightly from Liquid pipe syntax (`| filter: arg`)
- Not Shopify Liquid compatible — users coming from Jekyll/11ty would need to learn Django-style conventions
- Would require updating all layout examples in the spec and documentation

Since pongo2 and Liquid share delimiters, a migration would preserve the general look of templates while changing filter/tag semantics. This is a fallback option, not a planned switch.
