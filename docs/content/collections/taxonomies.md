---
layout: doc
title: Taxonomies
---

Taxonomies classify pages by terms like tags, categories, or any custom grouping. Declare a taxonomy in config, add terms to your front matter, and Alloy generates index and per-term pages automatically.

```yaml
# alloy.config.yaml
taxonomies:
  tags: {}
  categories: {}
```

```yaml
# content/blog/my-post.md
---
title: Building with WebAssembly
tags: [wasm, performance]
categories: [engineering]
---
```

This post appears in `taxonomies.tags.wasm`, `taxonomies.tags.performance`, and `taxonomies.categories.engineering`.

## Declaring taxonomies

Each key under `taxonomies:` in config maps to a front matter array field of the same name. The value is a configuration object (use `{}` for defaults):

```yaml
taxonomies:
  tags: {}
  categories:
    permalink: "/category/:slug/"
    layout: category
  topics:
    render: false
```

Front matter keys that are not declared in `taxonomies` config are ignored -- Alloy will not create taxonomy pages for undeclared keys, even if pages use them as arrays.

## Front matter usage

Assign terms to a page by listing them in the corresponding front matter field:

```yaml
---
title: My Post
tags: [javascript, css, html]
categories: [tutorials]
---
```

Terms are case-sensitive. `JavaScript` and `javascript` are distinct terms with separate pages.

## Applying terms via _data.yaml

Use the data cascade to apply taxonomy terms to an entire section:

```yaml
# content/blog/_data.yaml
tags: [blog]
categories: [articles]
```

Every page under `content/blog/` inherits these terms. Pages can add their own terms in front matter, but remember that arrays replace rather than merge -- a page with `tags: [css]` in its front matter gets only `css`, not `blog` and `css`. To include both, list all values:

```yaml
---
tags: [blog, css]
---
```

## Auto-generated pages

For each declared taxonomy with rendering enabled, Alloy generates two types of pages:

**Index page** -- lists all terms for the taxonomy. Output at `/<taxonomy>/` (e.g., `/tags/`).

**Per-term pages** -- lists all pages with that term. Output at `/<taxonomy>/<term>/` (e.g., `/tags/javascript/`).

No content files are needed for these pages. Alloy creates them from the taxonomy data.

## Template context

Taxonomy pages receive a `taxonomy` object in their template context:

**On the index page** (`/tags/`):

| Field | Type | Description |
|---|---|---|
| `taxonomy.term` | nil | Always nil on the index page |
| `taxonomy.terms` | array of objects | Each has `.name` (string) and `.pages` (array) |

```liquid
<ul>
  {% for t in taxonomy.terms %}
    <li>
      <a href="/tags/{{ t.name | slugify }}/">{{ t.name }}</a>
      ({{ t.pages | size }})
    </li>
  {% endfor %}
</ul>
```

**On a term page** (`/tags/javascript/`):

| Field | Type | Description |
|---|---|---|
| `taxonomy.term` | string | The current term (`"javascript"`) |
| `taxonomy.pages` | array | All pages with this term |

```liquid
<h2>Posts tagged "{{ taxonomy.term }}"</h2>
{% for page in taxonomy.pages %}
  <a href="{{ page.url }}">{{ page.title }}</a>
{% endfor %}
```

## Layout lookup order

Alloy resolves layouts for taxonomy pages in this order:

1. `layouts/taxonomies/<taxonomy>.liquid` (e.g., `layouts/taxonomies/tags.liquid`)
2. `layouts/<taxonomy>.liquid` (e.g., `layouts/tags.liquid`)
3. The default layout

The same layout is used for both the index page and per-term pages. Differentiate them by checking `taxonomy.term`:

```liquid
{% if taxonomy.term %}
  {%- comment -%} Per-term page {%- endcomment -%}
  <h2>{{ taxonomy.term }}</h2>
  {% for page in taxonomy.pages %}
    <a href="{{ page.url }}">{{ page.title }}</a>
  {% endfor %}
{% else %}
  {%- comment -%} Index page {%- endcomment -%}
  {% for t in taxonomy.terms %}
    <a href="/tags/{{ t.name | slugify }}/">{{ t.name }}</a>
  {% endfor %}
{% endif %}
```

## Custom permalink

Override the default URL pattern for taxonomy pages:

```yaml
taxonomies:
  categories:
    permalink: "/category/:slug/"
```

The `:slug` token is replaced with the slugified term name. The index page always renders at the base path (`/category/`).

## Custom layout

Specify a layout name directly in the taxonomy config:

```yaml
taxonomies:
  tags:
    layout: tag-list
```

This looks for `layouts/tag-list.liquid` (or `layouts/tag-list.html` with Go templates), bypassing the default lookup order.

## Disabling output with render: false

Set `render: false` to create taxonomy collections without generating output pages. The taxonomy data is still available in templates -- you just don't get the auto-generated index and term pages:

```yaml
taxonomies:
  series:
    render: false
```

Pages can still use `series: [beginner-go]` in front matter, and you can access `taxonomies.series.beginner-go` in templates. No `/series/` or `/series/beginner-go/` pages are written to disk.

This is useful for internal groupings that drive template logic but don't need their own landing pages.
