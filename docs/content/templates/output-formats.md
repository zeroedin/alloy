---
layout: doc
title: Output Formats
nav_weight: 50
description: "Render a single content page into multiple output formats: HTML, JSON, XML, or any text-based format."
---

Alloy can render a single content page into multiple output formats. Liquid is format-agnostic -- templates can produce HTML, JSON, XML, plain text, or any other text-based format.

```yaml
---
title: "My Blog Post"
outputs: ["html", "json"]
---
```

```liquid
<!-- layouts/post.json.liquid -->
{
  "title": "{{ page.title }}",
  "url": "{{ page.url | absolute_url }}",
  "date": "{{ page.date | date: '%Y-%m-%dT%H:%M:%S%z' }}",
  "content": {{ content | json }}
}
```

This page generates both `/my-blog-post/index.html` and `/my-blog-post/index.json`.

## How it works

Content rendering (Markdown to HTML) happens once per page. Layout rendering happens once per output format. Each format uses a separate layout file, so the same content can be presented differently in each format.

The `outputs` front matter field lists which formats to generate. When omitted, Alloy renders HTML only.

## Template file extensions

The file extension before `.liquid` (or the bare extension for Go templates) determines the output format:

**Liquid engine:**

```
layouts/default.liquid          --> HTML output
layouts/post.json.liquid        --> JSON output
layouts/feed.xml.liquid         --> XML output
layouts/data.csv.liquid         --> CSV output
layouts/robots.txt.liquid       --> plain text output
```

**Go template engine:**

```
layouts/default.html            --> HTML output
layouts/post.json                --> JSON output
layouts/feed.xml                 --> XML output
```

## Requesting multiple formats

Add the `outputs` array to a page's front matter to generate additional formats beyond HTML:

```yaml
---
title: "API Reference"
layout: "single"
outputs: ["html", "json"]
---
```

Alloy renders the page twice -- once with `layouts/single.liquid` (HTML) and once with `layouts/single.json.liquid` (JSON). Each format produces a separate output file.

### Layout resolution for formats

For a page requesting `json` output, the Liquid engine looks for layouts in this order:

1. `layouts/single.json.liquid` (format-specific with `.liquid` extension)
2. `layouts/single.json` (format-specific with bare extension, parsed as Liquid)

The layout must exist for each requested format. A missing format-specific layout is a build error.

## Practical examples

### JSON API endpoint

Generate a JSON representation of your blog index alongside the HTML page:

```yaml
# content/blog/index.md
---
title: "Blog"
layout: "blog-index"
outputs: ["html", "json"]
---
```

```liquid
<!-- layouts/blog-index.json.liquid -->
{
  "posts": [
    {% for post in collections.blog %}
    {
      "title": "{{ post.title | escape }}",
      "url": "{{ post.url | absolute_url }}",
      "date": "{{ post.date | date: '%Y-%m-%dT%H:%M:%S%z' }}",
      "summary": "{{ post.summary | escape }}"
    }{% unless forloop.last %},{% endunless %}
    {% endfor %}
  ]
}
```

Output: `/blog/index.json` with a machine-readable list of posts.

### RSS/Atom feed

Feeds are opt-in templates, not auto-generated. Place a `feed.xml` template in the `layouts/` directory:

```liquid
<!-- layouts/feed.xml.liquid -->
<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0" xmlns:atom="http://www.w3.org/2005/Atom">
  <channel>
    <title>{{ site.title }}</title>
    <link>{{ site.baseURL }}</link>
    <description>{{ site.description }}</description>
    <atom:link href="{{ '/feed.xml' | absolute_url }}" rel="self" type="application/rss+xml"/>
    {% for post in collections.blog limit: 20 %}
    <item>
      <title>{{ post.title | xml_escape }}</title>
      <link>{{ post.url | absolute_url }}</link>
      <pubDate>{{ post.date | rfc822_date }}</pubDate>
      <guid>{{ post.url | absolute_url }}</guid>
      <description>{{ post.summary | xml_escape }}</description>
    </item>
    {% endfor %}
  </channel>
</rss>
```

**Feed template placement determines scope:**

| Template location | Output path | Use case |
|---|---|---|
| `layouts/feed.xml.liquid` | `/feed.xml` | Site-wide feed |
| `layouts/blog/feed.xml.liquid` | `/blog/feed.xml` | Section feed |
| `layouts/taxonomies/tags/feed.xml.liquid` | `/tags/:slug/feed.xml` | Per-tag feed (rendered once per term) |

Each feed template has access to the same `collections`, `taxonomies`, and `site` context as any other template. The template controls what data it renders.

### Sitemap

Alloy auto-generates `sitemap.xml` from all published pages. Configure it in `alloy.config.yaml`:

```yaml
sitemap:
  changefreq: "weekly"
  priority: 0.5
```

Override per page in front matter:

```yaml
---
title: "Home"
sitemap:
  priority: 1.0
  changefreq: "daily"
---
```

Exclude a page from the sitemap:

```yaml
---
title: "Internal Page"
sitemap: false
---
```

Disable sitemap generation entirely:

```yaml
# alloy.config.yaml
sitemap: false
```

### Search index

Build a search index for client-side search (Pagefind, Lunr, etc.):

```yaml
# content/search.md
---
title: "Search"
layout: "search"
outputs: ["html", "json"]
permalink: "/search/"
---
```

```liquid
<!-- layouts/search.json.liquid -->
[
  {% for page in site.pages %}
  {
    "title": "{{ page.title | escape }}",
    "url": "{{ page.url }}",
    "content": "{{ page.summary | strip_html | escape }}"
  }{% unless forloop.last %},{% endunless %}
  {% endfor %}
]
```

## Custom output formats

Any text-based format works. The output format is determined by the layout file extension, not by a predefined list. Create a layout with the appropriate extension and reference it from your content:

```liquid
<!-- layouts/component.css.liquid -->
:host {
  {% for token in site.data.tokens %}
  --{{ token.name }}: {{ token.value }};
  {% endfor %}
}
```

```yaml
# content/tokens.md
---
title: "Design Tokens"
layout: "component"
outputs: ["css"]
permalink: "/tokens.css"
---
```

This generates a CSS file from your design token data.

## Engine-specific notes

The template engine is a global, project-wide setting. One engine is active per build. You cannot mix Liquid and Go template syntax within a single project's active templates.

When switching engines, create layout files with the appropriate extensions for the new engine. Alloy does not convert between template syntaxes.

## Related

- [Templates Overview](/templates/) -- engine configuration and template context
- [Layouts](/templates/layouts/) -- layout resolution and chaining
- [Filters](/templates/filters/) -- filters useful for feed and API output (`xml_escape`, `json`, `absolute_url`)
