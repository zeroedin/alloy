---
layout: doc
title: Permalinks
---

Permalinks control the output URL for each page. Set them in front matter, inherit them from `_data.yaml`, or let Alloy derive them from the file path.

```yaml
# content/blog/_data.yaml — all blog posts get date-based URLs
permalink: "/blog/:year/:month/:slug/"
```

A post at `content/blog/my-post.md` with `date: 2026-04-10` becomes `/blog/2026/04/my-post/`.

## Resolution order

Alloy resolves permalinks in this order, first match wins:

1. **Front matter `permalink:`** — always wins
2. **`_data.yaml` cascade `permalink:`** — section-level pattern
3. **`DefaultFromPath`** — file path maps directly to URL (`content/about.md` becomes `/about/`)

There is no site-level `permalinks:` config key. URL patterns are section-level data that belongs in `_data.yaml` — sections own their own URL structure.

## Available tokens

Use tokens in permalink patterns for dynamic URL segments:

| Token | Value | Example |
|---|---|---|
| `:year` | Content date (4-digit) | `2026` |
| `:month` | Content date (2-digit) | `04` |
| `:day` | Content date (2-digit) | `10` |
| `:slug` | Slugified title or filename | `my-first-post` |
| `:title` | Raw title from front matter | `My First Post` |
| `:section` | Top-level content directory | `blog` |
| `:filename` | Source filename without extension | `my-first-post` |

Token replacement is fast — 3000 pages with token replacement takes approximately 1ms.

## Template permalinks

Permalinks containing `{{ }}` are rendered through the configured template engine (Liquid or Go templates). Use this for computed URLs based on custom front matter fields:

```yaml
---
title: My Post
customField: special-category
permalink: "/{{ page.customField | slugify }}/{{ page.date | date: '%Y' }}/"
---
```

The permalink template syntax must match the configured engine. A Liquid permalink in a Go template project will fail, and vice versa. Only pages with `{{ }}` in their permalink pay the template rendering cost.

## Static overrides

Front matter can set a fixed permalink or override just the slug token:

```yaml
---
title: About Our Company
permalink: "/about/"        # fixed URL, no tokens
---
```

```yaml
---
title: A Very Long Title That Would Make an Ugly URL
slug: "short-url"           # overrides :slug token only
---
```

## Section-level patterns via _data.yaml

Use `_data.yaml` to set URL patterns for an entire section:

```yaml
# content/blog/_data.yaml
permalink: "/:section/:year/:month/:slug/"
```

This cascades to all pages in `content/blog/` and its subdirectories. Individual pages can still override with their own front matter `permalink:`.

## Index file behavior

Index files (`index.md`, `index.html`) resolve to their parent directory path and **skip** cascade permalink patterns. This prevents a `_data.yaml` pattern from turning `content/index.md` (title: "Home") into `/home/` instead of `/`.

- `content/index.md` resolves to `/`
- `content/blog/index.md` resolves to `/blog/`
- `content/blog/second-post/index.md` resolves to `/blog/second-post/`

The lookup order for index files:

1. Front matter `permalink:` (if set) — always honored
2. `DefaultFromPath` — strips `/index` suffix, returns directory path

Non-index files follow the full resolution chain including `_data.yaml` cascade patterns.

## `permalink: false`

Process the page (include it in collections, make its data available) but write no output file. Useful for data-only pages that feed into other pages via collections:

```yaml
---
title: Shared Config
permalink: false
layout: false
sharedData: "value available in collections but no output page"
---
```

## Aliases

A page can output at multiple paths. Aliases are additional output locations for the same rendered content — not redirects. Three identical files, no server configuration needed:

```yaml
---
title: About Us
permalink: "/about/"
aliases:
  - /about-us/
  - /team/
---
```

This writes the same rendered HTML to `_site/about/index.html`, `_site/about-us/index.html`, and `_site/team/index.html`. If an alias conflicts with another page's output path, the build fails with a conflict error.
