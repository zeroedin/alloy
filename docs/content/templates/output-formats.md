---
layout: doc
title: Output Formats
---

A single content page can produce multiple output files. Add the `outputs` field to front matter to specify which formats a page generates:

```yaml
---
title: Recent Articles
layout: default
outputs: ["html", "json"]
---
```

This page produces both `/articles/index.html` (using the default layout) and `/articles/index.json` (using a JSON-specific layout). Each format requires a matching layout file.

## Layout naming convention

Output format layouts use a compound file extension: `<name>.<format>.<engine-ext>`.

| Layout file | Output format | Output file |
|---|---|---|
| `default.liquid` | HTML (default) | `index.html` |
| `default.json.liquid` | JSON | `index.json` |
| `feed.xml.liquid` | XML | `feed.xml` |
| `search.json.liquid` | JSON | `search.json` |

The template engine is format-agnostic -- Liquid syntax works the same whether producing HTML, JSON, XML, or plain text. The layout file extension tells Alloy what output format to generate.

## Format layout resolution

When a page requests a non-HTML output format, Alloy resolves the layout by checking candidates in order:

1. `layouts/single.<format>.liquid`
2. `layouts/<section>.<format>.liquid`
3. `layouts/<filename>.<format>.liquid`
4. `layouts/default.<format>.liquid`

For a page in the `blog` section requesting JSON output, Alloy tries `single.json.liquid`, then `blog.json.liquid`, then the page's filename, then `default.json.liquid`.

## RSS/Atom feed

Alloy does not include a built-in feed template. Create your own `layouts/feed.xml.liquid`:

```liquid
<?xml version="1.0" encoding="UTF-8"?>
<feed xmlns="http://www.w3.org/2005/Atom">
  <title>{{ site.title }}</title>
  <link href="{{ site.baseURL }}"/>
  <link href="{{ site.baseURL }}/feed.xml" rel="self"/>
  <updated>{{ site.buildDate | date: "%Y-%m-%dT%H:%M:%SZ" }}</updated>
  <id>{{ site.baseURL }}/</id>
  {% for post in collections.posts %}
  <entry>
    <title>{{ post.title | escape }}</title>
    <link href="{{ post.url | absolute_url: site.baseURL }}"/>
    <id>{{ post.url | absolute_url: site.baseURL }}</id>
    <updated>{{ post.date | date: "%Y-%m-%dT%H:%M:%SZ" }}</updated>
    <content type="html">{{ post.body | escape }}</content>
  </entry>
  {% endfor %}
</feed>
```

Then create a content file that uses this layout:

```yaml
---
title: Feed
layout: feed
permalink: /feed.xml
---
```

The `permalink` field controls the output path. The layout's compound extension (`feed.xml.liquid`) tells Alloy to produce XML output.

## JSON API endpoint

Expose content as a JSON endpoint with a JSON layout:

```liquid
{
  "articles": [
    {% for article in collections.articles %}
    {
      "title": {{ article.title | json }},
      "url": {{ article.url | json }},
      "date": {{ article.date | date: "%Y-%m-%d" | json }},
      "summary": {{ article.summary | json }}
    }{% unless forloop.last %},{% endunless %}
    {% endfor %}
  ]
}
```

Save as `layouts/api.json.liquid`, then create the content page:

```yaml
---
title: Articles API
layout: api
permalink: /api/articles.json
outputs: ["json"]
---
```

The `json` filter serializes values with proper escaping and quoting, ensuring valid JSON output.

## Search index

A common pattern is generating a search index for client-side search:

```liquid
[
  {% for page in collections.all %}
  {
    "title": {{ page.title | json }},
    "url": {{ page.url | json }},
    "body": {{ page.body | strip_html | truncate: 500 | json }}
  }{% unless forloop.last %},{% endunless %}
  {% endfor %}
]
```

Save as `layouts/search.json.liquid` and create `content/search.json` with `layout: search` and `outputs: ["json"]`.

## Sitemap

Alloy generates a `sitemap.xml` automatically during the build. Every content page is included unless excluded. Configure sitemap behavior in `alloy.config.yaml`:

```yaml
# alloy.config.yaml
sitemap:
  enabled: true          # default: true
```

The built-in sitemap includes all pages with their URLs and last-modified dates. Pages with `draft: true` are excluded from production builds, and pages with `permalink: false` are excluded entirely.

## Multiple outputs per page

When a page lists multiple formats in `outputs`, Alloy renders the page once per format, each with its own layout:

```yaml
---
title: About
layout: default
outputs: ["html", "json"]
---
```

This produces two files:

- `/about/index.html` using `default.liquid`
- `/about/index.json` using `default.json.liquid`

The HTML output is always generated unless explicitly excluded from the `outputs` list. Non-HTML formats only generate when listed.
