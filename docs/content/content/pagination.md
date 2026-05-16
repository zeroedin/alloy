---
layout: doc
title: Pagination
---

Pagination in Alloy serves two purposes: generating individual pages from data items, and splitting long lists across multiple pages. The difference is `perPage`.

```yaml
---
pagination:
  data: site.data.team
  as: member
permalink: "/team/{{ member.slug }}/"
---
<h2>{{ member.name }}</h2>
<p>{{ member.role }}</p>
```

One template + 20 team members in data = 20 output pages.

## Two use cases

**Virtual pages** (`perPage` omitted or `1`, the default) — one output page per data item. Use this to generate individual pages from a data file or collection.

**Paginated lists** (`perPage > 1`) — data items are chunked into groups, one page per chunk. Use this for article listings, archive pages, and search results.

## Pagination front matter

Add a `pagination:` block to any content file's front matter:

| Key | Type | Default | Description |
|---|---|---|---|
| `data` | string | required | Data source — a `site.data.*` path or `collections.*` reference |
| `perPage` | int | `1` | Items per page. `1` = virtual pages, `> 1` = paginated list |
| `as` | string | required | Variable name for the current item (perPage=1) or item array (perPage>1) |

## Virtual pages (perPage = 1)

Generate one page per data item. The `as` variable holds a single item:

```yaml
---
layout: product
pagination:
  data: site.data.products
  as: product
permalink: "/products/{{ product.slug }}/"
---
<h2>{{ product.name }}</h2>
<p>{{ product.price | money }}</p>
<div>{{ product.description }}</div>
```

With 50 products in `data/products.yaml`, this produces 50 pages. No content files needed — one template drives all output.

## Front matter interpolation

When generating virtual pages, string front matter fields containing `{{ }}` or `{% %}` are interpolated using the `as` variable. This enables dynamic page metadata:

```yaml
---
title: "{{ member.name }}"
description: "Profile page for {{ member.name }}"
heading: "About {{ member.name | upcase }}"
layout: default
pagination:
  data: site.data.team
  perPage: 1
  as: member
permalink: "/team/{{ member.slug }}/"
---
```

For a team member `{name: "Alice", slug: "alice"}`:

- `page.title` becomes "Alice"
- `page.description` becomes "Profile page for Alice"
- `page.heading` becomes "About ALICE"
- `page.url` becomes "/team/alice/"

Interpolation rules:

- Only string-valued fields are interpolated. Numbers, booleans, arrays, and maps are left unchanged.
- Fields without `{{ }}` or `{% %}` markers skip the renderer entirely — no performance cost.
- Full Liquid (or Go template) syntax is supported, including filters.
- The template context contains only the `as` variable (e.g., `member`). `site.*`, `page.*`, and `collections.*` are not available during interpolation.
- `permalink`, `layout`, `pagination`, and keys starting with `_` are not interpolated.
- Interpolation only applies to virtual pages (`perPage: 1`). Paginated lists do not interpolate front matter.

## Paginated lists (perPage > 1)

Split a data source across multiple pages. The `as` variable holds an array of items for the current page:

```yaml
---
layout: article-list
pagination:
  data: collections.articles
  perPage: 10
  as: articles
permalink: "/articles/"
---
{% for article in articles %}
  <h2><a href="{{ article.url }}">{{ article.title }}</a></h2>
  <p>{{ article.summary }}</p>
{% endfor %}

{% if pagination.previousPage %}
  <a href="{{ pagination.previousPage }}">Previous</a>
{% endif %}
{% if pagination.nextPage %}
  <a href="{{ pagination.nextPage }}">Next</a>
{% endif %}
```

47 articles with `perPage: 10` produces 5 pages:

- `/articles/` — articles 1-10 (first page, no segment)
- `/articles/page/2/` — articles 11-20
- `/articles/page/3/` — articles 21-30
- `/articles/page/4/` — articles 31-40
- `/articles/page/5/` — articles 41-47

Alloy appends the page segments automatically. The `permalink` defines the base URL. The first page always outputs at the base permalink with no segment.

## Pagination context

Templates with pagination receive a `pagination` object:

| Field | Type | Description |
|---|---|---|
| `pagination.pageNumber` | int | Current page number (1-based) |
| `pagination.totalPages` | int | Total number of pages |
| `pagination.previousPage` | string / nil | URL of previous page (nil if first) |
| `pagination.nextPage` | string / nil | URL of next page (nil if last) |
| `pagination.first` | string | URL of the first page |
| `pagination.last` | string | URL of the last page |
| `pagination.items` | array | Items on the current page (also available via the `as` variable) |

## Page path config

The URL segment between the base permalink and the page number is configurable globally:

```yaml
# alloy.config.yaml
pagination:
  path: "page"    # default — /articles/page/2/
```

Change the segment word to suit your site's language or URL conventions:

```yaml
pagination:
  path: "p"       # /articles/p/2/
```

```yaml
pagination:
  path: "seite"   # /articles/seite/2/ (German)
```
