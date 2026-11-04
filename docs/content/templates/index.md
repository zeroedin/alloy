---
layout: doc
title: Templates Overview
nav_weight: 10
description: "An overview of Alloy's Liquid templates, layout lookup, output escaping, and the data available during rendering."
---

Alloy templates use the [Liquid](https://liquidmarkup.org/) template language. Templates live in the `layouts/` directory, are parsed once at startup, and rendered per page with full access to the site's data cascade.

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

## Template context

Every template receives these top-level variables:

| Variable | Description |
|---|---|
| `page` | Current page data: `page.title`, `page.url`, `page.date`, `page.summary`, `page.toc`, plus all front matter fields |
| `content` | The rendered body of the current page (Markdown already converted to HTML) |
| `site` | Site-wide data: `site.title`, `site.baseURL`, `site.language`, `site.data.*`, `site.pages` |
| `collections` | Collections from date-based sections and config-declared collections: `collections.blog`, `collections.releases`, etc. |
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

## Output escaping

Liquid prints values exactly as written. A data-file value of `<b>Sale today</b>` prints as bold text, not as the literal tags. That's what you want for your own rendered Markdown (`{{ content }}`) and HTML you wrote yourself.

To print a value as text, escape it with the `escape` filter:

```liquid
<p>{{ site.data.promo.banner }}</p>          <!-- <b>Sale today</b> prints as bold text -->
<p>{{ site.data.promo.banner | escape }}</p> <!-- prints the tags as text -->
```

Use `escape` on anything you didn't write yourself, such as a third-party feed or user-submitted data, and on values you place inside an HTML attribute.

## Template resolution

The `.liquid` extension marks a file as a Liquid template. The extension before `.liquid` determines the output format:

```
layouts/default.liquid          --> HTML output (default)
layouts/feed.xml.liquid         --> XML output
layouts/api.json.liquid         --> JSON output
```

Alloy looks for the `.liquid` file first and falls back to the bare extension (e.g., `default.html`) when no `.liquid` file is found. Both are read as Liquid.

```
layouts/
├── default.liquid       <-- used first
├── default.html         <-- used when there is no default.liquid, read as Liquid
├── feed.xml.liquid      <-- XML format layout
└── feed.xml             <-- used when there is no feed.xml.liquid, read as Liquid
```

## Rendering pipeline

Alloy renders content in a strict order:

1. **Markdown rendering** -- Goldmark parses `.md` files into HTML. Template tags (`{{ }}`, `{% %}`) are preserved through Markdown via a custom goldmark extension.
2. **Template rendering** -- Liquid evaluates the template tags in the rendered output and in layouts.
3. **Layout wrapping** -- The page body is injected into its resolved layout via `{{ content }}`. Layouts can chain to parent layouts.

Template tags inside `<code>` blocks in Markdown files are automatically escaped so they display as literal text rather than being evaluated. See the next section for the full rules.

## Literal template syntax

Whether `{{ }}` and `{% %}` are evaluated depends on where they appear in a Markdown file:

| Where the tags appear | Default | `templateTags: false` |
|---|---|---|
| Prose / body text | Evaluated by the template engine | Shown as typed |
| Inline code and fenced code blocks | Shown as typed | Shown as typed |

Code is always safe -- write `` `{{ site.title }}` `` in backticks or a fenced block and it displays exactly as typed, no escaping needed.

Prose is evaluated by default. This line in the page source:

```markdown
You are reading {{ page.title }}.
```

renders as:

> You are reading {{ page.title }}.

That quote is live -- the tag was evaluated against this page's context at build time, which is what you want when composing pages. But the same rule applies when a tag is meant as an *example*:

```markdown
The syntax {{ user.name }} inserts the author's name.
```

> The syntax {{ user.name }} inserts the author's name.

Also live -- and `user.name` is empty in this page's context, so the rendered sentence has a hole where the example should be. When you want prose tags shown as typed, escalate through three tools:

1. **Backticks.** If the tag can be code-formatted, put it in an inline code span. Always literal, most common case.
2. **`{% raw %}` for a one-off.** Wrap any run of prose in `{% raw %}...{% endraw %}` and the template engine outputs it verbatim:

   ```liquid
   {% raw %}Handlebars uses {{name}} for interpolation.{% endraw %}
   ```

3. **`templateTags: false` for the whole site.** If your content writes *about* template syntax everywhere and never uses tags in prose, turn prose evaluation off globally in [Markdown configuration](/content/#markdown-configuration):

   ```yaml
   content:
     markdown:
       goldmark:
         templateTags: false
   ```

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
- [Render Hooks](/templates/render-hooks/) -- customize Markdown element rendering
- [Filters](/templates/filters/) -- built-in and custom filter reference
- [Shortcodes](/templates/shortcodes/) -- reusable content snippets with parameters
- [Output Formats](/templates/output-formats/) -- multi-format rendering (HTML, JSON, XML)
