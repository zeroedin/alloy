---
layout: doc
title: Pagination
nav_weight: 50
description: "Generate paginated list pages and virtual pages from data using Alloy's pagination configuration."
---

Pagination in Alloy serves two purposes with one mechanism: generating paginated list pages and creating virtual pages from data. The difference is `perPage`.

```yaml
---
pagination:
  data: collections.articles
  perPage: 10
  as: articles
permalink: "/articles/"
---
```

- **`perPage` omitted or `1`** -- One page per item (virtual pages). This is the default.
- **`perPage > 1`** -- Items chunked into groups, one output page per chunk (paginated list).

## Paginated list pages

A paginated list splits a collection across multiple output pages:

```yaml
# content/articles.md
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

With 47 articles and `perPage: 10`, Alloy generates 5 pages:

| URL | Content |
|---|---|
| `/articles/` | Articles 1--10 (first page, no segment) |
| `/articles/page/2/` | Articles 11--20 |
| `/articles/page/3/` | Articles 21--30 |
| `/articles/page/4/` | Articles 31--40 |
| `/articles/page/5/` | Articles 41--47 |

The page path segment (`page`) is configured globally:

```yaml
# alloy.config.yaml
pagination:
  path: "page"       # default -- /articles/page/2/
```

Change the segment word to match your locale or preference: `path: "p"` produces `/articles/p/2/`, `path: "seite"` produces `/articles/seite/2/`. The first page always outputs at the base permalink with no segment.

## Virtual pages from data

When `perPage` is omitted or set to `1`, Alloy creates one output page per item. This turns a single template and a data source into many pages:

```yaml
# content/team.md
---
layout: default
pagination:
  data: site.data.team
  as: member
permalink: "/team/{{ member.slug }}/"
---
<h1>{{ member.name }}</h1>
<p>{{ member.role }}</p>
<img src="{{ member.photo }}" alt="{{ member.name }}">
```

Given `data/team.yaml` with 20 entries, this produces 20 pages at `/team/alice/`, `/team/bob/`, and so on. No individual content files needed.

### Data sources

The `data` field accepts a dot-path reference to any available data:

| Path | Source |
|---|---|
| `site.data.team` | `data/team.yaml` |
| `site.data.products` | `data/products.json` |
| `collections.blog` | Blog section collection |
| `collections.articles` | Articles section collection |

## The `as` variable

The `as` field names the variable that holds the current item (for virtual pages) or the current chunk (for list pages):

```yaml
pagination:
  data: site.data.team
  as: member       # {{ member.name }}, {{ member.slug }}
```

For list pages with `perPage > 1`, the `as` variable is an array:

```yaml
pagination:
  data: collections.articles
  perPage: 10
  as: articles     # {% for article in articles %}
```

## Pagination context

Every paginated page receives a `pagination` object in its template context:

```liquid
{{ pagination.pageNumber }}    -- Current page number (1-based)
{{ pagination.totalPages }}    -- Total page count
{{ pagination.previousPage }}  -- URL of previous page (nil if first)
{{ pagination.nextPage }}      -- URL of next page (nil if last)
{{ pagination.first }}         -- URL of first page
{{ pagination.last }}          -- URL of last page
{{ pagination.items }}         -- Items on the current page
```

The `as` variable is an alias for `pagination.items`. Both refer to the same data.

## Front matter interpolation

When generating virtual pages (`perPage: 1`), string-valued front matter fields containing `{{ }}` or `{% %}` are interpolated using the pagination item context. This gives each virtual page its own title, description, and other metadata:

```yaml
---
title: "{{ member.name }}"
heading: "About {{ member.name | upcase }}"
description: "Profile page for {{ member.name }}"
layout: default
pagination:
  data: site.data.team
  perPage: 1
  as: member
permalink: "/team/{{ member.slug }}/"
---
```

For a team member `{ name: "Alice", slug: "alice" }`, the virtual page gets:

- `page.title` = `"Alice"`
- `page.heading` = `"About ALICE"`
- `page.description` = `"Profile page for Alice"`
- `page.url` = `"/team/alice/"`

### Interpolation rules

- Only string-valued fields are interpolated. Numbers, booleans, arrays, and maps are left unchanged.
- Only fields containing `{{ }}` or `{% %}` are sent through the template engine. Fields without markers skip the renderer entirely.
- The template context contains only the `as:` variable (e.g., `{ member: item }`). `site.*`, `page.*`, and `collections.*` are not available during front matter interpolation.
- Skipped fields: `permalink` (already processed), `layout`, `pagination`, and any key starting with `_`.
- Interpolation only applies when `perPage` is `1`. List pages (`perPage > 1`) do not interpolate front matter because the `as:` variable is an array, not a single item.

## Combining with external data

Virtual pages pair well with [Data Files](/content/data-files/) and external data sources. Fetch product data from an API, store it in `data/`, and generate a page per product:

```yaml
# content/products.md
---
layout: product
pagination:
  data: site.data.products
  as: product
permalink: "/products/{{ product.slug }}/"
title: "{{ product.name }}"
---
<p>{{ product.description }}</p>
<p>Price: {{ product.price }}</p>
```

## Lifecycle interaction

Pagination operates on the post-filtered collection. [Lifecycle filtering](/content/lifecycle/) (drafts, future dates, expired dates) runs first, then pagination chunks the remaining items. A collection of 47 articles with 3 drafts produces 5 pages of 10 in build mode (44 items) but may produce different counts in dev mode (47 items, drafts included).
