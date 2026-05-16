---
layout: doc
title: Content Lifecycle
---

Control when content is published, hidden, or expired using three front matter fields: `draft`, `publishDate`, and `expiryDate`.

```yaml
---
title: Upcoming Feature
draft: true
publishDate: 2026-05-01
expiryDate: 2026-12-31
---
```

The default state is published. If `draft` is `false` or absent, and `publishDate` is not set or is in the past, the page is published immediately. You opt into hiding content, not into showing it.

## Draft content

```yaml
---
title: Work in Progress
draft: true
---
```

A draft page is excluded from `alloy build` and `alloy serve` output. It is visible only in `alloy dev` (dev mode), so authors can preview their work.

In dev mode, a draft page ignores `publishDate` and `expiryDate` — it behaves as if it were published now. Date fields are still used for sort ordering within collections.

## Future-dated content

```yaml
---
title: Launch Announcement
publishDate: 2026-06-15
---
```

A page with a future `publishDate` is excluded everywhere — `alloy build`, `alloy serve`, and `alloy dev`. The page appears automatically once the publish date arrives and the site is rebuilt.

To preview a future-dated page before its publish date, set `draft: true`. The draft flag overrides date filtering in dev mode.

## Expired content

```yaml
---
title: Summer Sale
expiryDate: 2026-09-01
---
```

A page with a past `expiryDate` is excluded everywhere — `alloy build`, `alloy serve`, and `alloy dev`. To preview an expired page, set `draft: true`.

## Behavior summary

| Field | `alloy build` | `alloy serve` | `alloy dev` |
|---|---|---|---|
| `draft: true` | Excluded | Excluded | Visible |
| `publishDate` in future | Excluded | Excluded | Excluded |
| `expiryDate` in past | Excluded | Excluded | Excluded |

## Interaction with collections and pagination

Lifecycle filtering happens before collection building and pagination:

- **Drafts** are excluded from `collections.*` in build and serve modes, but **included** in dev mode so authors can preview paginated lists with draft content.
- **Future `publishDate`** pages are excluded from `collections.*` in all modes.
- **Past `expiryDate`** pages are excluded from `collections.*` in all modes.

Pagination always operates on the post-filtered collection. A paginated list of 47 articles with 3 drafts produces pages based on 44 items in build/serve mode, but may produce different page counts in dev mode where all 47 items are included.

## Content summaries

The `summary` field in front matter provides a short description for use in list templates and meta tags:

```yaml
---
title: Building Web Components
summary: "A practical guide to building reusable web components with Lit."
---
```

Summaries support HTML via YAML multiline strings:

```yaml
summary: |
  <p>A practical guide to building <strong>reusable web components</strong> with Lit.</p>
```

Access it in templates as `page.summary`:

```liquid
{% for article in collections.articles %}
  <article>
    <h2><a href="{{ article.url }}">{{ article.title }}</a></h2>
    <p>{{ article.summary }}</p>
  </article>
{% endfor %}
```

If no `summary` is set in front matter, `page.summary` is nil. Alloy does not auto-generate summaries — the author provides one explicitly or the template handles the absence. Summaries are static data and are not processed through the template engine.

## Table of contents

Alloy extracts the heading structure from each page during Markdown rendering and exposes it as `page.toc`. Each entry in the array has:

| Field | Type | Description |
|---|---|---|
| `id` | string | The heading's `id` attribute (auto-generated or `{#custom-id}` override) |
| `text` | string | Plain text content of the heading |
| `level` | int | Heading level (2-6; h1 is excluded) |
| `children` | array | Nested headings one level deeper |

The top-level array contains the shallowest headings (typically h2). h3s nest under h2s, h4s under h3s. Pages with no headings (or only h1) have an empty `page.toc`.

Disable TOC generation for sites that don't use it:

```yaml
content:
  markdown:
    toc: false
```
