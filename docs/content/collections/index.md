---
layout: doc
title: Collections
---

Collections group related pages into ordered lists that you can loop over in templates. Not every subdirectory is a collection -- Alloy creates collections explicitly, not implicitly from your directory tree.

```liquid
{% for post in collections.blog %}
  <article>
    <h2><a href="{{ post.url }}">{{ post.title }}</a></h2>
    <time>{{ post.date | date: "%B %d, %Y" }}</time>
  </article>
{% endfor %}
```

## Creating collections

There are two ways to create a collection.

**Date-based permalink pattern.** Add a `_data.yaml` file with a permalink containing date tokens (`:year`, `:month`, `:day`). Any section with a date-based permalink pattern automatically becomes a collection:

```yaml
# content/blog/_data.yaml
permalink: "/blog/:year/:month/:day/:slug/"
```

All pages under `content/blog/` are now members of `collections.blog`, sorted by date descending.

**Taxonomy collections.** When you declare a taxonomy in config and pages use the corresponding front matter key, Alloy creates per-term collections automatically. See [Taxonomies](/collections/taxonomies/) for details.

A plain subdirectory without a date-based permalink or taxonomy association is just a directory -- it does not produce a collection.

## Default sort order

Collections sort by **date descending** (newest first). Pages without a date fall to the end in filename order.

```liquid
{%- comment -%} Newest post first, no configuration needed {%- endcomment -%}
{% for post in collections.blog %}
  {{ post.title }} — {{ post.date | date: "%Y-%m-%d" }}
{% endfor %}
```

## Custom sort via config

Override the default sort in `alloy.config.yaml`:

```yaml
# alloy.config.yaml
collections:
  blog:
    sortBy: title
    order: asc
```

`sortBy` accepts any front matter field. `order` is `asc` or `desc` (default `desc`).

## Inline sort in templates

Use the `sort` filter to re-sort a collection in a specific template without changing the global order:

```liquid
{% assign sorted = collections.blog | sort: "title" %}
{% for post in sorted %}
  <li>{{ post.title }}</li>
{% endfor %}
```

The `sort` filter is numeric-aware. Values that can be parsed as numbers are compared numerically: `"2"`, `"10"`, `"1"` sort as `1, 2, 10` -- not the lexicographic `1, 10, 2`. This applies to any field, including custom front matter like `order` or `weight`.

```liquid
{% assign by_weight = collections.docs | sort: "weight" %}
```

## Collection data shape

Each item in a collection is a page object with all its resolved front matter fields plus computed fields:

| Field | Description |
|---|---|
| `url` | The resolved permalink |
| `title` | Page title |
| `date` | Page date |
| `content` | Rendered HTML body |
| Any front matter key | Custom fields from front matter or `_data.yaml` cascade |

Collections are materialized eagerly during the build and frozen as read-only data. Modifying a collection in a template has no effect -- the data is a snapshot, not a live query.

## Accessing collections

Collections are available in every template as `collections.<name>`:

```liquid
{%- comment -%} List the 5 most recent blog posts {%- endcomment -%}
{% for post in collections.blog limit: 5 %}
  <a href="{{ post.url }}">{{ post.title }}</a>
{% endfor %}

{%- comment -%} Count all articles {%- endcomment -%}
<p>{{ collections.articles | size }} articles published</p>

{%- comment -%} Filter to a subset {%- endcomment -%}
{% assign featured = collections.blog | where: "featured", true %}
{% for post in featured %}
  <div class="featured">{{ post.title }}</div>
{% endfor %}
```

Taxonomy-generated collections are accessed via the `taxonomies` object instead. `taxonomies.tags.javascript` returns all pages tagged "javascript". See [Taxonomies](/collections/taxonomies/) for the full taxonomy API.
