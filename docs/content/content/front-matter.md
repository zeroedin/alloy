---
layout: doc
title: Front Matter
nav_weight: 20
description: "Front matter reference: the YAML, TOML, or JSON metadata block that controls a page's title, URL, layout, and lifecycle."
---

Front matter is the metadata block at the top of every content file. Alloy uses it to determine the page title, URL, layout, taxonomy membership, and lifecycle state.

```yaml
---
title: "Building a Design System"
date: 2026-04-10
layout: post
tags: ["css", "design-systems"]
description: "A practical guide to building a component library from scratch."
---
```

## Supported formats

Alloy detects the front matter format by its delimiter:

| Delimiter | Format | Example |
|---|---|---|
| `---` | YAML | `---`<br>`title: "My Post"`<br>`---` |
| `+++` | TOML | `+++`<br>`title = "My Post"`<br>`+++` |
| `{` | JSON | `{ "title": "My Post" }` |

YAML is the most common. All three parse into the same internal data structure, so the rest of the pipeline is format-agnostic.

## Required vs optional

Front matter delimiters are required on all content files. Empty front matter is valid:

```markdown
---
---

Page content starts here.
```

A content file without any delimiters is a build error. The error message will suggest adding empty front matter if you have no metadata to set.

## Built-in fields

These fields have special meaning in Alloy. All are optional -- omit any you do not need.

### title

The page title. Available in templates as `{{ page.title }}`.

```yaml
title: "Getting Started with Alloy"
```

### date

The content date, used for sort order in collections and for permalink tokens (`:year`, `:month`, `:day`). Accepts any format that Go's `time.Parse` understands.

```yaml
date: 2026-04-10
date: 2026-04-10T14:30:00Z
```

When two pages in a collection share the same date, the full datetime is compared. If the datetime is identical or has no time component, filename alphabetical order is the tiebreaker.

### layout

The layout template to use for this page. Overrides the automatic layout lookup.

```yaml
layout: post
```

Set `layout: false` to skip layout wrapping entirely. The rendered content is written directly to the output file with no surrounding HTML.

### permalink

The output URL for this page. Overrides the automatic URL computed from the file path and cascade patterns.

```yaml
permalink: "/about/"
permalink: "/blog/:year/:month/:slug/"
permalink: "/team/{{ member.slug }}/"
```

Set `permalink: false` to process the page (include it in collections) without writing an output file. See [Permalinks](/content/permalinks/) for the full token reference.

### slug

Overrides the `:slug` permalink token. By default, `:slug` is derived from the filename.

```yaml
slug: "custom-url-segment"
```

### description

A short description of the page. Available as `{{ page.description }}` in templates. Alloy does not use this field internally, but it is conventional for SEO meta tags and feed entries.

```yaml
description: "A step-by-step walkthrough for configuring Alloy."
```

### summary

A page summary. Available as `{{ page.summary }}` in templates. Summaries are static data -- they are not rendered through the template engine.

```yaml
summary: "Learn how to set up your first Alloy project in under five minutes."
```

Supports HTML via YAML multiline strings:

```yaml
summary: |
  <p>Learn how to set up your first Alloy project.</p>
  <p>Covers installation, config, and your first build.</p>
```

If no `summary` is set, `{{ page.summary }}` is nil. Alloy does not auto-generate summaries.

Use summaries in list templates:

```liquid
{% for article in collections.articles %}
  <article>
    <h2><a href="{{ article.url }}">{{ article.title }}</a></h2>
    <p>By {{ article.author }} -- {{ article.date | date: "%B %Y" }}</p>
    {% if article.summary %}
      <p>{{ article.summary }}</p>
    {% endif %}
  </article>
{% endfor %}
```

### tags

An array of taxonomy terms. Alloy uses tags (and other declared taxonomies) to group pages into collections.

```yaml
tags: ["javascript", "web-components"]
```

Tags can also be set at the directory level via `_data.yaml`, so every page in a section inherits them without repeating the list. See [Data Cascade](/content/data-cascade/).

### draft

Marks the page as a draft.

```yaml
draft: true
```

Draft pages are excluded from `alloy build` and `alloy serve`. They are visible in `alloy dev` so authors can preview work in progress. See [Content Lifecycle](/content/lifecycle/) for the full rules.

### publishDate

The date when the page becomes published. Pages with a future `publishDate` are excluded from all build modes, including dev.

```yaml
publishDate: 2026-05-01
```

To preview a future-dated page in dev mode, set `draft: true` -- the draft flag overrides date filtering in dev mode.

### expiryDate

The date when the page expires. Pages with a past `expiryDate` are excluded from all build modes.

```yaml
expiryDate: 2026-12-31
```

### aliases

Additional output paths for this page. Each alias writes an identical copy of the rendered HTML -- not a redirect.

```yaml
aliases:
  - /about-us/
  - /team/
```

Aliases participate in the pre-build conflict detection. If an alias collides with another page's output path, the build fails. See [Permalinks](/content/permalinks/) for details.

### outputs

An array of output formats. Alloy looks for a matching layout for each format.

```yaml
outputs: ["html", "json"]
```

A page requesting `json` output needs a corresponding layout (e.g., `layouts/single.json.liquid`). The page is rendered once per format.

### sitemap

Per-page sitemap overrides.

```yaml
sitemap:
  priority: 0.8
  changefreq: "daily"
```

Set `sitemap: false` to exclude the page from the generated sitemap.

### pagination

Configures pagination or virtual page generation for this page. See [Pagination](/content/pagination/) for the full reference.

```yaml
pagination:
  data: collections.articles
  perPage: 10
  as: articles
```

## Custom fields

Any additional key-value pair in front matter is available in templates as `{{ page.fieldName }}`. Alloy does not restrict which keys you can use:

```yaml
---
title: "Team Member"
role: "Engineering Lead"
github: "https://github.com/example"
order: 3
---
```

```liquid
<h1>{{ page.title }}</h1>
<p>{{ page.role }}</p>
<a href="{{ page.github }}">GitHub</a>
```

Custom fields participate in the [Data Cascade](/content/data-cascade/) -- they can be set at the directory level in `_data.yaml` and overridden per-page in front matter.
