---
layout: doc
title: Data Cascade
nav_weight: 30
description: "How Alloy merges global data, directory data, front matter, and plugin hooks into the data available on every page."
---

The data cascade is how Alloy assembles the data available to each page. Three levels of data merge together using deep-merge, with later levels overriding earlier ones:

```
1. Global data       (data/*.yaml, data/*.json)
2. Directory data    (_data.yaml -- cascades into subdirectories)
3. Front matter      (per-file YAML/TOML/JSON block)
```

**Front matter always wins** over directory data, which always wins over global data.

Plugins can also mutate page data at specific hook points during the build. See [Plugin hooks](#plugin-hooks) below.

## Quick example

Given this project structure:

```
data/
└── site.yaml              # { author: "Acme Corp", theme: "light" }

content/
├── _data.yaml             # { layout: "default", author: "Blog Team" }
├── index.md               # front matter: { title: "Home" }
└── blog/
    ├── _data.yaml          # { layout: "post", permalink: "/:year/:month/:slug/" }
    └── first-post.md       # front matter: { title: "Hello World", author: "Alice" }
```

The page `content/blog/first-post.md` sees:

| Key | Value | Source |
|---|---|---|
| `author` | `"Alice"` | Front matter (wins over directory and global) |
| `layout` | `"post"` | `blog/_data.yaml` (wins over parent `_data.yaml`) |
| `theme` | `"light"` | Global data (no override) |
| `title` | `"Hello World"` | Front matter |

## Directory data with `_data.yaml`

Place a `_data.yaml` file in any content directory to set default metadata for all pages in that directory and its subdirectories.

```yaml
# content/blog/_data.yaml
layout: post
permalink: "/:year/:month/:slug/"
tags: ["blog"]
```

Every page under `content/blog/` inherits these values. A page's own front matter can override any of them.

### Inheritance across directories

Directory data cascades into all descendant directories, not just immediate children. A `_data.yaml` at a parent level applies until a deeper `_data.yaml` overrides it:

```
content/_data.yaml                # layout: "default"
content/blog/_data.yaml           # layout: "post"   -- overrides parent
content/blog/2026/_data.yaml      # adds: featured: true
```

A page at `content/blog/2026/march/update.md` inherits from the nearest ancestor `_data.yaml`. The lookup walks upward through `blog/2026/march/`, `blog/2026/`, `blog/`, then `content/` until it finds a `_data.yaml`. Most directories will not have their own `_data.yaml` -- they rely entirely on ancestor inheritance.

### Merge example

```yaml
# content/_data.yaml
layout: default
meta:
  og_type: "website"
  twitter_card: "summary"
scripts: ["analytics.js"]
```

```yaml
# content/blog/_data.yaml
layout: post
meta:
  og_type: "article"
```

A page in `content/blog/` sees:

- `layout` = `"post"` (child overrides parent)
- `meta.og_type` = `"article"` (deep-merged, child wins)
- `meta.twitter_card` = `"summary"` (deep-merged, inherited from parent)
- `scripts` = `["analytics.js"]` (inherited, no override in child)

## Merge rules

**Objects are deep-merged.** Nested keys merge recursively. A child `_data.yaml` or front matter block only needs to specify the keys it wants to override -- everything else is inherited.

**Arrays are replaced, not concatenated.** This is predictable but requires awareness:

```yaml
# content/blog/_data.yaml
scripts: ["analytics.js"]

# content/blog/my-post.md front matter
scripts: ["custom.js"]

# Result: scripts = ["custom.js"]
# NOT ["analytics.js", "custom.js"]
```

To include values from both levels, list all values in the overriding array.

**Front matter always wins** over directory data, which always wins over global data. Within the directory chain, the deepest (nearest to the page) `_data.yaml` wins for any given key.

## Global data

Files in the `data/` directory are loaded into `site.data.*` and available to every template. See [Data Files](/content/data-files/) for details on formats and external data sources.

Global data sits at the bottom of the cascade -- it provides site-wide defaults that any directory or page can override.

## Performance

Global and directory data are loaded once and shared by pointer across all pages. Only front matter is per-page. Deep merging happens lazily -- only when a nested key exists at multiple cascade levels.

For a site with 3,000 pages and 50KB of shared data, memory usage is approximately 50KB (shared) + 1.5MB (front matter), not 150MB from deep copies.

## Plugin hooks

Plugins can mutate page data at three points during the build. Unlike the cascade levels above, hooks do not deep-merge -- they replace the page's front matter with whatever the plugin returns. These are advanced use cases; most sites only need the three cascade levels.

**`onPagesReady`**: Fires before taxonomy collection. Plugins can inject virtual pages or modify front matter. Data set here participates in taxonomy grouping.

**`onContentTransformed`**: Fires after Markdown rendering, per-page. Plugins can modify a page's front matter, rendered HTML, and TOC data.

**`onContentLoaded`**: Fires after all content rendering, batch-level. Plugins can modify `frontMatter` and `html` across all pages. Changes here win over everything else since they run last.

## Using cascade data in templates

All cascaded data is available in templates through the `page` and `site` objects:

{% raw %}
<wa-tab-group>
<wa-tab slot="nav" panel="cascade-liquid" active>Liquid</wa-tab>
<wa-tab slot="nav" panel="cascade-go">Go templates</wa-tab>

<wa-tab-panel name="cascade-liquid" active>
<alloy-code language="liquid">&lt;h1&gt;{{ page.title }}&lt;/h1&gt;
&lt;p&gt;By {{ page.author }}&lt;/p&gt;

{% if page.tags %}
  {% for tag in page.tags %}
    &lt;span class="tag"&gt;{{ tag }}&lt;/span&gt;
  {% endfor %}
{% endif %}

&lt;p&gt;Theme: {{ site.data.site.theme }}&lt;/p&gt;</alloy-code>
</wa-tab-panel>
<wa-tab-panel name="cascade-go">
<alloy-code language="html">&lt;h1&gt;{{ .page.title }}&lt;/h1&gt;
&lt;p&gt;By {{ .page.author }}&lt;/p&gt;

{{ if .page.tags }}
  {{ range .page.tags }}
    &lt;span class="tag"&gt;{{ . }}&lt;/span&gt;
  {{ end }}
{{ end }}

&lt;p&gt;Theme: {{ .site.data.site.theme }}&lt;/p&gt;</alloy-code>
</wa-tab-panel>
</wa-tab-group>
{% endraw %}

The data cascade is resolved before template rendering. By the time your template runs, `page.layout`, `page.tags`, and every other field reflect the fully merged result of global data, directory data, and front matter.
