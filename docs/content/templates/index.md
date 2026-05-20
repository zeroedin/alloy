---
layout: doc
title: Templates Overview
nav_weight: 10
---

Alloy uses the [Liquid](https://liquidmarkup.org/) template language by default. Templates live in the `layouts/` directory, are parsed once at startup, and rendered per page with full access to the site's data cascade.

```yaml
# alloy.config.yaml
templates:
  engine: "liquid"   # default; also supports "go" (Go html/template)
```

```liquid
<!-- layouts/default.liquid -->
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

Every content page is rendered through its resolved layout. The layout receives the page's rendered body as `{{ content }}` and can access all front matter fields via the `page` object.

## Template engines

Alloy supports two built-in (Tier 1) template engines. The engine is a global, project-wide setting -- one engine is active per build.

| Engine | Config value | File extension | Syntax |
|---|---|---|---|
| Liquid | `"liquid"` (default) | `.liquid` | `{{ var }}`, `{% tag %}` |
| Go templates | `"go"` | `.html` | `{{ .var }}`, `{{ range }}` |

Both engines receive the same `map[string]any` context from the data cascade. All built-in [filters](/templates/filters/) are registered in both engines at startup.

A third-party engine (Nunjucks, EJS, Pug) can be registered via the Node bridge as a Tier 3 plugin engine, though every page render becomes an IPC round-trip with significant performance cost.

## Template context

Every template receives these top-level variables:

| Variable | Description |
|---|---|
| `page` | Current page data: `page.title`, `page.url`, `page.date`, `page.summary`, `page.toc`, plus all front matter fields |
| `content` | The rendered body of the current page (Markdown already converted to HTML) |
| `site` | Site-wide data: `site.title`, `site.baseURL`, `site.language`, `site.data.*`, `site.pages` |
| `collections` | Section-based collections: `collections.blog`, `collections.docs`, etc. |
| `taxonomies` | Taxonomy groups: `taxonomies.tags.javascript`, `taxonomies.categories.tutorials`, etc. |
| `pagination` | Pagination context (only on paginated pages): `pagination.pageNumber`, `pagination.totalPages`, `pagination.nextPage`, `pagination.previousPage` |

```liquid
<h1>{{ page.title }}</h1>
<time>{{ page.date | date: "%B %d, %Y" }}</time>
{{ content }}

<h2>Recent posts</h2>
{% for post in collections.blog limit: 5 %}
  <a href="{{ post.url }}">{{ post.title }}</a>
{% endfor %}
```

## Template resolution

The `.liquid` extension marks a file as a Liquid template. The extension before `.liquid` determines the output format:

```
layouts/default.liquid          --> HTML output (default)
layouts/feed.xml.liquid         --> XML output
layouts/api.json.liquid         --> JSON output
```

When the Liquid engine cannot find a `.liquid` file, it falls back to the bare extension and parses it as Liquid. The Go engine uses bare extensions directly (`.html`, `.xml`, `.json`) and never reads `.liquid` files.

```
layouts/
├── default.liquid     <-- used when engine: "liquid"
├── default.html       <-- used when engine: "go" (or as Liquid fallback)
├── feed.xml.liquid    <-- used when engine: "liquid"
├── feed.xml           <-- used when engine: "go" (or as Liquid fallback)
└── robots.txt         <-- static content, used by either engine
```

If a file contains syntax for the wrong engine, the build fails with a parse error. Alloy does not inspect file contents to determine the engine.

## Rendering pipeline

Alloy renders content in a strict order:

1. **Markdown rendering** -- Goldmark parses `.md` files into HTML. Template tags (`{{ }}`, `{% %}`) are preserved through Markdown via a custom goldmark extension.
2. **Template rendering** -- The configured engine evaluates Liquid or Go template syntax in the rendered output and in layouts.
3. **Layout wrapping** -- The page body is injected into its resolved layout via `{{ content }}`. Layouts can chain to parent layouts.

Template tags inside `<code>` blocks in Markdown files are automatically escaped so they display as literal text rather than being evaluated.

## Directory structure

```
layouts/
├── default.liquid         # Fallback layout for all pages
├── blog.liquid            # Blog index layout (matches section name)
├── post.liquid            # Blog post layout (child of date-based section)
└── partials/              # Reusable template fragments
    ├── header.liquid
    └── footer.liquid
```

The layouts directory is configurable:

```yaml
# alloy.config.yaml
structure:
  layouts: "./docs/layouts/"   # default: "layouts"
```

## Next steps

- [Layouts](/templates/layouts/) -- layout chaining, resolution order, partials
- [Filters](/templates/filters/) -- built-in and custom filter reference
- [Shortcodes](/templates/shortcodes/) -- reusable content snippets with parameters
- [Output Formats](/templates/output-formats/) -- multi-format rendering (HTML, JSON, XML)
