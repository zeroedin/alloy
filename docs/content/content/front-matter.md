---
layout: doc
title: Front Matter
---

Front matter is a metadata block at the top of every content file. It defines the page's title, layout, URL, and any custom fields you need in templates.

```markdown
---
title: My First Post
layout: post
date: 2026-04-10
tags: ["go", "ssg"]
---

Post content starts here.
```

All front matter fields are available in templates as `page.<field>` (Liquid) or `.page.<field>` (Go templates).

## Format delimiters

Alloy detects the front matter format from its opening delimiter:

| Delimiter | Format | Example |
|---|---|---|
| `---` | YAML | `---`<br>`title: "My Post"`<br>`---` |
| `+++` | TOML | `+++`<br>`title = "My Post"`<br>`+++` |
| `{` | JSON | `{ "title": "My Post" }` |

YAML is the most common choice. All three formats parse into the same internal structure — the rest of the pipeline is format-agnostic.

## Empty front matter

A file with empty delimiters is a valid content page. All fields default to nil/zero. This is useful for pages that get all their metadata from the [data cascade](/content/data-cascade/):

```markdown
---
---

This page inherits layout, tags, and permalink from _data.yaml.
```

## Built-in fields

These fields have special meaning to Alloy:

| Field | Type | Description |
|---|---|---|
| `title` | string | Page title. Used in `page.title` and as the default `:title` permalink token. |
| `layout` | string | Layout template name (without extension). `layout: false` skips layout wrapping. |
| `permalink` | string / false | Output URL. Supports tokens (`:slug`, `:year`) and template expressions (`{{ }}`). `false` processes the page but writes no output file. |
| `date` | date | Content date. Used for sorting and permalink tokens `:year`, `:month`, `:day`. |
| `tags` | array | Taxonomy tags. Populates `taxonomies.tags.*` collections. |
| `draft` | bool | `true` excludes the page from production builds. Visible in dev mode. |
| `publishDate` | date | Page is excluded everywhere until this date arrives. |
| `expiryDate` | date | Page is excluded everywhere after this date passes. |
| `summary` | string | Short description. Available as `page.summary`. Not auto-generated — author-provided only. |
| `slug` | string | Overrides the `:slug` permalink token. Defaults to the slugified filename. |
| `aliases` | array | Additional output paths for this page (not redirects — identical copies). |
| `outputs` | array | Output formats to generate (e.g., `["html", "json"]`). Each format needs a matching layout. |
| `description` | string | Page description, often used in meta tags. Available as `page.description`. |

## Custom fields

Any key in front matter becomes a `page.*` variable in templates:

```yaml
---
title: Building Web Components
author: Alice
difficulty: intermediate
order: 3
---
```

```liquid
<p>Written by {{ page.author }}</p>
<span class="badge">{{ page.difficulty }}</span>
```

## Table of contents

Alloy automatically extracts headings from Markdown content and exposes them as `page.toc` — a nested array of heading data. Each entry has `id`, `text`, `level`, and `children` fields.

```liquid
<nav class="toc">
  {% for item in page.toc %}
    <a href="#{{ item.id }}">{{ item.text }}</a>
    {% if item.children.size > 0 %}
      <ul>
        {% for child in item.children %}
          <li><a href="#{{ child.id }}">{{ child.text }}</a></li>
        {% endfor %}
      </ul>
    {% endif %}
  {% endfor %}
</nav>
```

Headings get auto-generated `id` attributes by default (e.g., "Getting Started" becomes `id="getting-started"`). Override with the heading attributes syntax:

```markdown
## My Custom Section {#custom-id}
```

Configure TOC and heading IDs in `alloy.config.yaml`:

```yaml
content:
  markdown:
    toc: true            # default: true — generate page.toc
    autoHeadingID: true   # default: true — add id attributes to headings
```
