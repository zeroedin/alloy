---
layout: doc
title: Templates Overview
---

Alloy templates control how your content is rendered into output pages. The default template engine is Liquid, powered by the Notifuse/liquidgo library.

```liquid
<article>
  <h1>{{ page.title }}</h1>
  <time>{{ page.date | date: "%B %d, %Y" }}</time>
  {{ content }}
</article>
```

## Template engines

Alloy supports two template engines. The engine is a **global, project-wide setting** -- every layout, partial, and content template uses the same engine.

| Engine | Config value | File extension | Syntax |
|---|---|---|---|
| Liquid (default) | `"liquid"` | `.liquid` | `{{ var }}`, `{% tag %}` |
| Go html/template | `"gotemplate"` | `.html` | `{{ .var }}`, `{{ range }}` |

Switch engines in `alloy.config.yaml`:

```yaml
# alloy.config.yaml
templates:
  engine: "gotemplate"   # default: "liquid"
```

## File extension conventions

With the Liquid engine, Alloy resolves layout files by looking for `.liquid` first, then falling back to the bare extension. A layout named `post` resolves as:

1. `layouts/post.liquid`
2. `layouts/post`

With the Go template engine, Alloy looks for `.html`:

1. `layouts/post.html`
2. `layouts/post`

## Template context

Every template has access to these top-level objects:

| Object | Contents |
|---|---|
| `page` | Front matter fields + computed fields (`url`, `date`, `section`, `toc`) |
| `site` | Config values and global data (`site.title`, `site.baseURL`, `site.data.*`) |
| `collections` | Named content collections (`collections.articles`, `collections.docs`) |
| `taxonomies` | Taxonomy term maps (`taxonomies.tags`, `taxonomies.categories`) |
| `pagination` | Pagination state when the page uses [pagination](/content/pagination/) |
| `content` | Rendered page body (available in layouts) |

Access context in Liquid with dot notation:

```liquid
<title>{{ page.title }} - {{ site.title }}</title>

<nav>
  {% for item in site.data.navigation %}
    <a href="{{ item.url }}">{{ item.label }}</a>
  {% endfor %}
</nav>
```

In Go template mode, prefix variables with a dot:

```html
<title>{{ .page.title }} - {{ .site.title }}</title>

<nav>
  {{ range .site.data.navigation }}
    <a href="{{ .url }}">{{ .label }}</a>
  {{ end }}
</nav>
```

## Template resolution

Alloy evaluates templates in content files before rendering Markdown. Template tags (`{{ }}` and `{% %}`) in your Markdown body are processed by the template engine, then the result is passed through the Markdown renderer.

Template tags inside fenced code blocks are automatically detected and preserved -- they render as literal text, not evaluated expressions:

````markdown
```liquid
{{ page.title }}
```
````

This outputs the literal string `{{ page.title }}` in a code block, not the evaluated value.

## Global data in templates

Files in the `data/` directory are available as `site.data.<filename>`. A file at `data/authors.yaml` containing a list of authors is accessible as `site.data.authors`:

```liquid
{% for author in site.data.authors %}
  <span>{{ author.name }}</span>
{% endfor %}
```

JSON, YAML, and CSV data files are all supported. See [Data Files](/content/data-files/) for details.
