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
  "url": "{{ page.url | absolute_url: site.baseURL }}",
  "date": "{{ page.date | date: '%Y-%m-%dT%H:%M:%S%z' }}",
  "content": {{ content | json }}
}
```

This page generates both `/my-blog-post/index.html` and `/my-blog-post/index.json`.

## How it works

Content rendering (Markdown to HTML) happens once per page. Layout rendering happens once per output format. Each format uses a separate layout file, so the same content can be presented differently in each format.

The `outputs` front matter field lists which formats to generate. When omitted, Alloy renders HTML only.

## Template file extensions

The format sits between the layout name and the engine extension:

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
layouts/post.json.html          --> JSON output
layouts/feed.xml.html           --> XML output
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

Note that the front matter `layout:` field only affects the HTML pass. Format layouts resolve by the candidate chain below -- `single.<format>`, section, filename, then default -- so name your format layout after one of those.

### Layout resolution for formats

For a page requesting `json` output, Alloy looks for layouts in this order (shown with the Liquid extension; the Go engine checks the same names with `.html`):

1. `layouts/single.json.liquid`
2. `layouts/<section>.json.liquid` (the page's section name)
3. `layouts/<filename>.json.liquid` (the page's filename without extension)
4. `layouts/default.json.liquid`

A layout must exist for each requested format. If none of the candidates exist, the build fails.

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
<!-- layouts/blog.json.liquid (matches the section name) -->
{
  "posts": [
    {% for post in collections.blog %}
    {
      "title": "{{ post.title | escape }}",
      "url": "{{ post.url | absolute_url: site.baseURL }}",
      "date": "{{ post.date | date: '%Y-%m-%dT%H:%M:%S%z' }}",
      "summary": "{{ post.summary | escape }}"
    }{% unless forloop.last %},{% endunless %}
    {% endfor %}
  ]
}
```

Output: `/blog/index.json` with a machine-readable list of posts.

### RSS/Atom feed

Feeds are opt-in, not auto-generated. A feed is a page requesting `xml` output through the same mechanism as any other format -- create a content stub that requests it, and a matching format layout:

```yaml
# content/blog/index.md
---
title: "Blog"
outputs: ["html", "xml"]
---
```

```liquid
<!-- layouts/blog.xml.liquid (matches the section name) -->
<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0" xmlns:atom="http://www.w3.org/2005/Atom">
  <channel>
    <title>{{ site.title }}</title>
    <link>{{ site.baseURL }}</link>
    {% for post in collections.blog limit: 20 %}
    <item>
      <title>{{ post.title | escape }}</title>
      <link>{{ post.url | absolute_url: site.baseURL }}</link>
      <pubDate>{{ post.date | date: "%a, %d %b %Y %H:%M:%S %z" }}</pubDate>
      <guid>{{ post.url | absolute_url: site.baseURL }}</guid>
      <description>{{ post.summary | escape }}</description>
    </item>
    {% endfor %}
  </channel>
</rss>
```

Output: `/blog/index.xml` alongside the HTML index. The template has access to the same `collections`, `taxonomies`, and `site` context as any other template -- XML entity escaping uses the standard `escape` filter, and RFC 822 dates come from the `date` filter with the format string shown above.

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

Disable sitemap generation entirely in your config:

```yaml
# alloy.config.yaml
sitemap: false
```

This prevents `sitemap.xml` from being written to the output. Useful for sites behind authentication or when sitemaps are generated by another tool.

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
- [Filters](/templates/filters/) -- filters useful for feed and API output (`escape`, `json`, `date`, `absolute_url`)
