---
layout: doc
title: Content Lifecycle
nav_weight: 60
description: "Control whether content appears in the build output using draft, future, and expiry fields in front matter."
---

Every content file has a lifecycle state that determines whether it appears in the build output. By default, all pages are published immediately. You opt into hiding content using front matter fields.

```yaml
---
title: "Upcoming Feature Announcement"
draft: true
publishDate: 2026-05-01
expiryDate: 2026-12-31
---
```

## Lifecycle fields

### draft

```yaml
draft: true
```

Draft pages are excluded from `alloy build` and `alloy serve`. They are visible in `alloy dev` so authors can preview work in progress.

In dev mode, a draft page ignores `publishDate` and `expiryDate` -- it behaves as if it were published now. Date fields are still used for sort ordering within collections.

### publishDate

```yaml
publishDate: 2026-05-01
```

Pages with a future `publishDate` are excluded from `alloy build`, `alloy serve`, and `alloy dev`. Future-dated pages are hidden everywhere until their publish date arrives.

To preview a future-dated page during development, set `draft: true`. The draft flag overrides date filtering in dev mode.

### expiryDate

```yaml
expiryDate: 2026-12-31
```

Pages with a past `expiryDate` are excluded from `alloy build`, `alloy serve`, and `alloy dev`. To preview an expired page, set `draft: true`.

## Behavior by build mode

| State | `alloy build` | `alloy serve` | `alloy dev` |
|---|---|---|---|
| Published (default) | Included | Included | Included |
| `draft: true` | Excluded | Excluded | Included |
| Future `publishDate` | Excluded | Excluded | Excluded |
| Past `expiryDate` | Excluded | Excluded | Excluded |
| `draft: true` + future date | Excluded | Excluded | Included |
| `draft: true` + past expiry | Excluded | Excluded | Included |

The default state is published. If `draft` is false or absent, and `publishDate` is not set or is in the past, the page appears in the output immediately.

## Collections and pagination interaction

Lifecycle filtering affects collections and pagination:

- **Drafts** are excluded from `collections.*` in build and serve mode. They are included in dev mode, so authors can preview paginated lists with draft content.
- **Future `publishDate`** pages are excluded from `collections.*` in all modes.
- **Past `expiryDate`** pages are excluded from `collections.*` in all modes.

Pagination always operates on the post-filtered collection. Lifecycle filtering runs first, then [pagination](/content/pagination/) chunks the remaining items:

```
47 articles total
 3 drafts
--
44 articles in build mode → 5 pages of 10
47 articles in dev mode   → 5 pages of 10
```

## Content processing pipeline

Every published page moves through these stages in order:

### Phase 0 -- Pre-build validation

1. **Config load** -- Parse `alloy.config.yaml`, merge CLI flags.
2. **Content discovery** -- Walk `content/`, identify content files by extension.
3. **Front matter extraction** -- Parse metadata from each file. Fast: reads only the metadata block, not the body.
4. **Data cascade assembly** -- Load global data (`data/`), directory data (`_data.yaml`), merge with front matter. See [Data Cascade](/content/data-cascade/).
5. **Output path computation** -- Compute every output URL from [permalinks](/content/permalinks/), aliases, pagination, and taxonomy rules.
6. **Conflict detection** -- Check for duplicate output paths across all sources. Fail fast before any rendering work.

### Phase 1 -- Content rendering

7. **Taxonomy collection** -- Build taxonomy maps from page front matter.
8. **Pagination expansion** -- Generate virtual pages from [pagination](/content/pagination/) templates.
9. **Content transformation** -- Markdown to HTML (via goldmark). Template tags (`{{ }}`, `{% %}`) pass through the Markdown parser and are evaluated.
10. **Template resolution** -- Match each page to its layout.
11. **Content template rendering** -- Evaluate template tags in the content body with page data and site data.
12. **Layout rendering** -- Inject rendered content into the resolved layout as `{{ content }}`.

### Phase 2 -- Config-driven SSR (opt-in, experimental)

If the [experimental `ssr:` config block](/experimental/ssr/) is configured, pages containing custom elements are piped through the SSR command for Declarative Shadow DOM expansion. Pages without custom elements skip this phase. This phase is skipped during `alloy dev`.

### Phase 3 -- Output

13. **Asset copy** -- Copy `assets/` files to `_site/`.
14. **Static and passthrough copy** -- Copy `static/` files and passthrough mappings.
15. **Output writing** -- Write all final HTML to `_site/`.

## Summaries

Alloy provides page summaries through the `summary` front matter field:

```yaml
---
title: "My Post"
summary: "A short description of this post"
---
```

Available in templates as `{{ page.summary }}`. If no `summary` is set, the value is nil. Alloy does not auto-generate summaries.

Summaries are static data -- they are not rendered through the template engine. For dynamic summary composition, build the summary in the list template:

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

## Table of contents

Alloy extracts the heading structure from each Markdown page during rendering and exposes it as `page.toc`:

```liquid
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

Each TOC entry has:

| Field | Type | Description |
|---|---|---|
| `id` | string | The heading's `id` attribute (auto-generated or `{#custom-id}` override) |
| `text` | string | Plain text content of the heading |
| `level` | int | Heading level (2--6; h1 is excluded) |
| `children` | array | Nested headings one level deeper |

Configure heading IDs and TOC generation:

```yaml
content:
  markdown:
    autoHeadingID: true    # default: true
    toc: true              # default: true -- set false to disable TOC extraction
```

## Incremental builds

In dev mode, after the initial full build, the file watcher triggers incremental rebuilds. Alloy uses content-hash change detection (SHA-256) to skip unchanged files entirely -- no re-parse, no re-render. Config changes trigger a full rebuild. Template changes invalidate only the pages using that template.
