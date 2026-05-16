---
layout: doc
title: Layouts
---

Layouts are template files in the `layouts/` directory that wrap page content. Every content page is rendered inside a layout using the `{{ content }}` tag.

```liquid
<!DOCTYPE html>
<html>
<head>
  <title>{{ page.title }}</title>
</head>
<body>
  <main>{{ content }}</main>
</body>
</html>
```

Save this as `layouts/default.liquid` and every page without an explicit layout uses it.

## Directory structure

```
layouts/
├── default.liquid        # Fallback layout
├── post.liquid           # Blog post layout
├── page.liquid           # Static page layout
├── feed.xml.liquid       # RSS feed (XML output)
└── partials/
    ├── header.liquid      # Reusable header
    ├── footer.liquid      # Reusable footer
    └── sidebar.liquid     # Sidebar component
```

Layouts live at the top level of `layouts/`. Partials go in the `partials/` subdirectory. Output-format layouts use a compound extension (`feed.xml.liquid`) to indicate the output format.

## Layout resolution order

Alloy resolves which layout to use for a page by checking candidates in order, using the first match found on disk:

| Priority | Source | Example |
|---|---|---|
| 1 | Front matter `layout` field | `layout: post` resolves to `layouts/post.liquid` |
| 2 | `_data.yaml` cascade | `layout: page` in a parent `_data.yaml` |
| 3 | `"post"` for date-based sections | Pages under a section with `:year` in its permalink pattern |
| 4 | Section name for index pages | `content/blog/index.md` tries `layouts/blog.liquid` |
| 5 | Filename match | `content/about.md` tries `layouts/about.liquid` |
| 6 | `default.liquid` fallback | Always the last candidate |

Set `layout: false` in front matter to skip layout wrapping entirely. The page renders its body with no surrounding template.

## The content tag

The `{{ content }}` tag in a layout is replaced with the rendered body of the page. It must appear exactly once in the layout:

```liquid
<body>
  <header>{% include "partials/header" %}</header>
  <article>
    <h1>{{ page.title }}</h1>
    {{ content }}
  </article>
  <footer>{% include "partials/footer" %}</footer>
</body>
```

## Layout chaining

A layout can specify its own parent layout in YAML front matter. This creates a chain where the inner layout renders first, then its output becomes the `{{ content }}` of the parent.

```liquid
---
layout: base
---
<article class="post">
  <header>
    <h1>{{ page.title }}</h1>
    <time>{{ page.date | date: "%B %d, %Y" }}</time>
  </header>
  {{ content }}
</article>
```

Save this as `layouts/post.liquid`. When a page uses `layout: post`, Alloy:

1. Renders the page body into `post.liquid` at `{{ content }}`
2. Takes that result and renders it into `base.liquid` at `{{ content }}`

The chain can go up to 10 levels deep. Alloy detects circular references at build time and reports an error.

A base layout at the top of the chain has no front matter (or empty front matter) -- it is the root:

```liquid
<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <title>{{ page.title }} - {{ site.title }}</title>
</head>
<body>
  {{ content }}
</body>
</html>
```

## Partials

Include reusable template fragments with the `{% include %}` tag. Partials are resolved from the `layouts/` directory:

```liquid
{% include "partials/header" %}
```

Alloy looks for the partial file using the same extension resolution as layouts: `partials/header.liquid` first, then `partials/header.html`, then `partials/header`.

Partials have access to the same template context as the layout that includes them -- `page`, `site`, `collections`, and all other top-level objects.

## Template context in layouts

Layouts receive the full template context:

| Variable | Description |
|---|---|
| `content` | Rendered page body |
| `page.title` | Page title from front matter |
| `page.date` | Page date |
| `page.url` | Output URL |
| `page.*` | Any front matter field |
| `site.*` | Config and global data |
| `collections.*` | Named collections |
| `taxonomies.*` | Taxonomy term maps |

Content variables set in the page body do not leak into the layout. The layout only sees front matter fields via `page.*` and the rendered body via `{{ content }}`.

## Example: layout with sidebar

```liquid
---
layout: base
---
<div class="layout-sidebar">
  <aside>
    {% include "partials/sidebar" %}
  </aside>
  <main>
    <h1>{{ page.title }}</h1>
    {{ content }}
  </main>
</div>
```

Pages that set `layout: sidebar` get the sidebar chrome, which itself is wrapped by the base layout through chaining.

## Example: complete chain

Given three layout files:

**`layouts/base.liquid`** (root -- no parent):

```liquid
<!DOCTYPE html>
<html>
<head><title>{{ page.title }}</title></head>
<body>
  {% include "partials/header" %}
  {{ content }}
  {% include "partials/footer" %}
</body>
</html>
```

**`layouts/doc.liquid`** (extends base):

```liquid
---
layout: base
---
<div class="docs">
  <nav>{% include "partials/sidebar" %}</nav>
  <article>{{ content }}</article>
</div>
```

**`layouts/api.liquid`** (extends doc):

```liquid
---
layout: doc
---
<div class="api-reference">
  <span class="badge">API</span>
  {{ content }}
</div>
```

A page with `layout: api` renders through: page body into `api.liquid` into `doc.liquid` into `base.liquid`.
