---
layout: doc
title: Data Cascade
---

Every page in Alloy inherits data from multiple sources, merged in a defined order. Higher levels override lower levels — front matter beats directory data, which beats global data.

```
content/
├── _data.yaml              # layout: default, author: "Site Team"
├── blog/
│   ├── _data.yaml          # permalink: "/blog/:slug/", author: "Blog Team"
│   └── post.md             # author: "Alice" (front matter wins)
└── docs/
    └── guide.md            # inherits layout: default, author: "Site Team"
```

In this example, `post.md` gets `author: "Alice"` from its own front matter, `permalink: "/blog/:slug/"` from `blog/_data.yaml`, and `layout: default` from the root `_data.yaml`. The docs `guide.md` inherits everything from the root `_data.yaml` with no overrides.

## The 6 cascade levels

Data is resolved in this order, last wins:

| Level | Source | Scope |
|---|---|---|
| 1 | Global data (`data/*.yaml`, `data/*.json`) | All pages |
| 2 | Directory data (`_data.yaml` cascade) | Directory and all descendants |
| 3 | Front matter | Single page |
| 4 | `onPagesReady` plugins | Pre-taxonomy, pre-Markdown |
| 5 | `onContentTransformed` plugins | Per-page, post-Markdown |
| 6 | `onContentLoaded` plugins | Batch-level, post-Markdown |

Levels 1-3 cover most use cases. Levels 4-6 are for [plugin hooks](/hooks/) that compute or mutate data programmatically.

## Directory data inheritance

A `_data.yaml` file cascades into **all** descendant directories, not just immediate children. Child directories can provide their own `_data.yaml` to override specific fields:

```yaml
# content/_data.yaml
layout: default
author: "Site Team"
scripts: ["analytics.js"]
```

```yaml
# content/blog/_data.yaml
permalink: "/blog/:slug/"
author: "Blog Team"
```

A page at `content/blog/2026/march/post.md` inherits from the nearest ancestor `_data.yaml` files. The lookup walks upward through the directory tree: `blog/2026/march/` then `blog/2026/` then `blog/` then `content/` — stopping at each level that has a `_data.yaml`.

Most directories will not have their own `_data.yaml`. They rely entirely on ancestor inheritance.

## Merge rules

**Objects are deep-merged.** Nested keys merge recursively across cascade levels:

```yaml
# content/_data.yaml
author:
  name: "Site Team"
  twitter: "@alloy"

# content/blog/post.md front matter
author:
  email: "alice@example.com"

# Result for post.md:
# author.name    → "Site Team"   (inherited)
# author.twitter → "@alloy"      (inherited)
# author.email   → "alice@example.com" (from front matter)
```

**Arrays are replaced, not concatenated.** This is predictable but requires awareness:

```yaml
# content/blog/_data.yaml
scripts: ["analytics.js"]

# content/blog/post.md front matter
scripts: ["custom.js"]

# Result: scripts = ["custom.js"]
# NOT ["analytics.js", "custom.js"]
# To include both, list all values in the front matter array.
```

## Override chain example

Given this directory structure:

```yaml
# data/site.yaml — global data (level 1)
company: "Acme Corp"
theme: "light"

# content/_data.yaml — all content (level 2)
layout: default
sidebar: true

# content/blog/_data.yaml — blog section (level 2, deeper)
layout: post
permalink: "/blog/:year/:slug/"
sidebar: false

# content/blog/hello.md — single page (level 3)
---
title: Hello World
date: 2026-04-10
sidebar: true
---
```

The resolved data for `hello.md`:

| Field | Value | Source |
|---|---|---|
| `site.data.site.company` | "Acme Corp" | Global data |
| `page.layout` | "post" | `blog/_data.yaml` (overrides root) |
| `page.permalink` | "/blog/:year/:slug/" | `blog/_data.yaml` |
| `page.sidebar` | true | Front matter (overrides `blog/_data.yaml`) |
| `page.title` | "Hello World" | Front matter |

## Performance

Global and directory data are loaded once and shared by pointer across all pages. Only front matter is per-page. Deep merging happens lazily — only when a nested key is accessed at multiple cascade levels.

For a site with 3000 pages and 50KB of shared data, memory usage is approximately 50KB (shared) + 1.5MB (front matter), not the 150MB that deep-copying shared data to every page would require.
