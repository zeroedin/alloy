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
2. **`_data.yaml` cascade `permalink:`** -- Section-level patterns with token replacement. The **nearest** `_data.yaml` in the directory tree wins -- a subdirectory's pattern overrides its parent's.
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

Every page under `content/docs/` gets a flat URL like `/docs/getting-started/`, unless a subdirectory's `_data.yaml` sets its own pattern.

### Nested patterns

A subdirectory can override its parent's permalink pattern. Alloy uses the **nearest** `_data.yaml` permalink in the directory tree:

```yaml
# content/blog/_data.yaml — simple slugs for static pages
permalink: "/blog/:slug/"

# content/blog/posts/_data.yaml — date-based URLs for posts
permalink: "/blog/:year/:month/:slug/"
```

Pages in `content/blog/` use `/blog/:slug/`. Pages in `content/blog/posts/` use `/blog/:year/:month/:slug/`.

## Front matter permalinks

A page can set its own URL directly, overriding any cascade pattern:

```yaml
---
title: "About Us"
permalink: "/about/"
---
```

### Template permalinks

Permalinks containing `{{ }}` are rendered as Liquid template expressions. Template permalinks work on both regular pages and [pagination](/content/pagination/) virtual pages.

**Token permalinks and template permalinks are two separate modes** -- they are not composable. If a permalink contains `{{`, the entire string is rendered as a template expression. Token syntax (`:year`, `:slug`, etc.) is not processed and would appear as literal text.

```yaml
---
title: "My Post"
slug: "my-post"
permalink: "/articles/{{ page.slug }}/"
---
```

Template permalinks render through Liquid regardless of the configured engine. The `page` object contains front matter fields, date, slug, summary, and collection:

| Token | Template equivalent |
|---|---|
| `:year` | `{{ page.date \| date: '%Y' }}` |
| `:month` | `{{ page.date \| date: '%m' }}` |
| `:day` | `{{ page.date \| date: '%d' }}` |
| `:slug` | `{{ page.slug }}` |
| `:title` | `{{ page.title }}` |
| `:section` | `{{ page.collection }}` |

Tokens are simpler and faster -- use them when the built-in set covers your needs. Template permalinks are for cases that need custom front matter fields or Liquid filters:

```yaml
---
title: "My Custom Post"
lang: "en"
permalink: "/{{ page.lang }}/{{ page.title | slugify }}/"
---
```

`page.url` is not available in the template permalink context (it is what's being computed). A template permalink that renders to an empty string is a build error.

Pagination pages can use the `as:` variable in template permalinks:

```yaml
---
pagination:
  data: site.data.team
  perPage: 1
  as: member
permalink: "/team/{{ member.slug }}/"
---
```

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
