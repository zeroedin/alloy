---
layout: doc
title: Permalinks
nav_weight: 40
description: "How Alloy resolves output URLs through a three-level priority chain: front matter, directory data, then file path."
---

Permalinks control the output URL for each page. Alloy resolves URLs through a three-level priority chain: front matter first, then the `_data.yaml` cascade, then the file path.

```yaml
# content/blog/_data.yaml
permalink: "/:year/:month/:slug/"
```

With this directory data, a post at `content/blog/my-post.md` with `date: 2026-04-10` outputs to `/2026/04/my-post/`.

## Resolution order

1. **Front matter `permalink:`** -- Always wins. A page with an explicit permalink ignores all cascade patterns.
2. **`_data.yaml` cascade `permalink:`** -- Section-level patterns with token replacement. Applied to all pages in the directory and its subdirectories.
3. **`DefaultFromPath`** -- The file path maps directly to the URL. `content/about.md` becomes `/about/`.

There is no site-level `permalinks:` config key. URL patterns are section-level data set in `_data.yaml` -- sections own their own URL structure.

## Available tokens

Tokens in permalink patterns are replaced with values from the page's front matter and metadata:

| Token | Value | Example |
|---|---|---|
| `:year` | Content date, 4-digit | `2026` |
| `:month` | Content date, 2-digit | `04` |
| `:day` | Content date, 2-digit | `10` |
| `:slug` | Slugified title or filename | `my-first-post` |
| `:title` | Raw title from front matter | `My First Post` |
| `:section` | Top-level content directory | `blog` |
| `:filename` | Source filename without extension | `my-first-post` |

### Blog with date-based URLs

```yaml
# content/blog/_data.yaml
layout: post
permalink: "/blog/:year/:month/:slug/"
```

```yaml
# content/blog/my-first-post.md
---
title: "My First Post"
date: 2026-04-10
---
```

Output: `/blog/2026/04/my-first-post/`

### Docs site with flat URLs

```yaml
# content/docs/_data.yaml
layout: doc
permalink: "/docs/:slug/"
```

Every page under `content/docs/` gets a flat URL like `/docs/getting-started/`, regardless of subdirectory depth.

## Front matter permalinks

A page can set its own URL directly, overriding any cascade pattern:

```yaml
---
title: "About Us"
permalink: "/about/"
---
```

### Template permalinks

Front matter permalinks that contain `{{ }}` are rendered through the configured template engine. This lets you build dynamic URLs from any front matter field:

```yaml
---
title: "My Post"
permalink: "/{{ page.customField | slugify }}/{{ page.date | date: '%Y' }}/"
---
```

Template permalinks use the same engine configured in `templates.engine`. A Liquid permalink (`{{ page.slug }}`) in a Go template project will fail, and vice versa. Only pages with `{{ }}` in their permalink pay the template rendering cost.

### Static overrides

Permalinks without tokens or template tags are a zero-cost fast path:

```yaml
---
permalink: "/custom/path/here/"
---
```

### Overriding just the slug

To change only the URL segment derived from the filename, set `slug:` instead of a full permalink:

```yaml
---
title: "A Very Long Title That Would Make an Ugly URL"
slug: "short-url"
---
```

With a cascade pattern of `/:year/:month/:slug/`, this page outputs to `/:year/:month/short-url/`.

## Index files

Index files (`index.md`, `index.html`) resolve to their parent directory path by default, skipping cascade permalink patterns:

- `content/index.md` -- `/` (site root)
- `content/blog/index.md` -- `/blog/` (section landing)
- `content/blog/second-post/index.md` -- `/blog/second-post/` (page bundle)

This prevents a cascade pattern from turning `content/index.md` (title: "Home") into `/home/` instead of `/`.

Front matter `permalink:` still works on index files:

```yaml
---
title: "Home"
permalink: "/docs/"
---
```

## `permalink: false`

Process the page (include it in collections, make its data available) without writing an output file. Useful for data-only pages:

```yaml
---
title: "Shared Config"
permalink: false
layout: false
sharedData: "value available in collections but no output page"
---
```

## Aliases

A page can output at multiple paths. Aliases write identical copies of the rendered HTML -- not redirects.

```yaml
---
title: "About Us"
permalink: "/about/"
aliases:
  - /about-us/
  - /team/
---
```

This writes the same HTML to `_site/about/index.html`, `_site/about-us/index.html`, and `_site/team/index.html`.

Aliases are validated during the pre-build conflict detection pass. If an alias collides with another page's output path, the build fails with a clear error.

## Trailing slashes

All permalinks should end with a trailing slash. Alloy writes pages as `path/index.html`, so `/about/` becomes `_site/about/index.html`. This produces clean URLs that work with any static file server.

## Performance

Token replacement for 3,000 pages takes approximately 1ms. Only pages with `{{ }}` in their permalink pay the template rendering cost (~50 microseconds per page).
