---
layout: doc
title: Collections
nav_weight: 10
description: "How Alloy automatically groups content into collections from your directory structure, with no manual registration required."
---

Collections group related content pages so templates can iterate, sort, and filter them. Alloy creates collections automatically from your content structure -- no manual registration required.

```yaml
# content/blog/_data.yaml
permalink: "/:year/:month/:slug/"
```

Any section whose `_data.yaml` permalink pattern contains date tokens (`:year`, `:month`, `:day`) becomes a collection. The pages under `content/blog/` are now available in templates as `collections.blog`.

## Directory-Based Collections

Collections follow from your content directory structure. A directory with a date-based permalink pattern in its `_data.yaml` collects all child pages:

```
content/
  blog/
    _data.yaml          # permalink: "/:year/:month/:slug/"
    first-post.md       # -> collections.blog
    second-post.md      # -> collections.blog
    2024/
      archived.md       # -> collections.blog (subdirectories inherit)
  docs/
    getting-started.md  # not a collection (no date-based permalink)
```

Regular directories without date-based permalink patterns are just pages, not collections. To group non-blog pages across sections, use [taxonomies](/collections/taxonomies/).

## Iterating Collections

Loop through a collection in your templates to build index pages, feeds, or navigation:

```liquid
{% for post in collections.blog %}
  <article>
    <h2><a href="{{ post.url }}">{{ post.title }}</a></h2>
    <time>{{ post.date | date: "%B %d, %Y" }}</time>
    <p>{{ post.summary }}</p>
  </article>
{% endfor %}
```

Each item in the collection is a page object with all the usual properties: `url`, `title`, `date`, `summary`, and any custom front matter fields.

## Default Sort Order

Collections sort by **date descending** (newest first). When two pages share the same date, the full datetime is compared. If the datetime is identical or no time component is provided, filename alphabetical ascending is the tiebreaker. Pages without a date sort after all dated pages, ordered by filename.

Sort is deterministic across builds.

## Custom Sort Order

Override the default sort for a collection in your site config:

```yaml
# alloy.config.yaml
collections:
  blog:
    sortBy: "date"
    order: "desc"
```

You can sort by any front matter field. For a portfolio sorted by a custom `order` field:

```yaml
collections:
  projects:
    sortBy: "order"
    order: "asc"
```

## Sorting in Templates

Sort inline using built-in array filters:

```liquid
{% assign alphabetical = collections.blog | sort: "title" %}
{% assign by_author = collections.blog | sort: "author" %}
{% assign recent = collections.blog | sort: "date" | reverse %}
```

The `sort` filter is numeric-aware. Values like `order: 1, 2, 10, 20` sort correctly as numbers, not as strings (`1, 10, 2, 20`).

### Numeric Sort Rules

- `int` values (YAML `order: 10`) are compared as integers, including negatives
- `float64` with no fractional part (JSON `"order": 10.0`) are compared as integers
- String values containing only digits (`order: "10"`) are parsed and compared as integers
- Everything else falls back to string comparison
- Nil or missing values sort to the end

## Filtering Collections

Use the `where` filter to narrow a collection by any front matter field:

```liquid
{% assign featured = collections.blog | where: "featured", true %}
{% for post in featured %}
  <a href="{{ post.url }}">{{ post.title }}</a>
{% endfor %}
```

Combine with other filters for powerful queries:

```liquid
{% assign recent_tutorials = collections.blog
    | where: "category", "tutorial"
    | sort: "date"
    | reverse %}
```

## Collection Lifecycle

Collections interact with [content lifecycle](/content/) rules:

- **Drafts** (`draft: true`): excluded from collections in `alloy build` and `alloy serve`, included in `alloy dev`
- **Future publish dates**: excluded from collections everywhere until the date arrives
- **Expired content**: excluded from collections everywhere

Lifecycle filtering happens before pagination. A paginated list of 47 articles with 3 drafts produces pages based on 44 items in build mode, but may differ in dev mode where drafts are included.

## Accessing All Pages

All published pages are available as `site.pages` regardless of whether they belong to a collection:

```liquid
{% for page in site.pages %}
  <a href="{{ page.url }}">{{ page.title }}</a>
{% endfor %}
```

## Related

- [Taxonomies](/collections/taxonomies/) -- cross-cutting groups via tags, categories, and custom keys
- [Lifecycle Events](/hooks/) -- hooks that fire during collection building
- [Data Cascade](/content/) -- how `_data.yaml` drives collection creation
