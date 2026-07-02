---
layout: doc
title: Content Overview
nav_weight: 10
description: "An overview of how Alloy turns files in content/ into pages: front matter, the data cascade, rendering, and output."
---

Content in Alloy lives in the `content/` directory. Each file with a supported extension (`.md`, `.html` by default) becomes a page on your site. Alloy reads the front matter, applies the data cascade, renders the body, wraps it in a layout, and writes the result to `_site/`.

```
content/
├── index.md               # → _site/index.html  (site root)
├── about.html             # → _site/about/index.html
└── blog/
    ├── _data.yaml          # Directory-level data (cascades to all blog pages)
    ├── index.md            # → _site/blog/index.html
    ├── first-post.md       # → _site/blog/first-post/index.html
    └── second-post/        # Page bundle (co-located assets)
        ├── index.md        # → _site/blog/second-post/index.html
        └── hero.jpg        # Copied to _site/blog/second-post/hero.jpg
```

## Supported content formats

The `content.formats` config controls which file extensions are treated as content. The default is `["md", "html"]`:

```yaml
# alloy.config.yaml
content:
  formats: ["md", "html"]
```

Files matching these extensions go through the full pipeline: front matter extraction, data cascade, template rendering, and layout wrapping. Everything else in `content/` is treated as a passthrough file and copied to the output as-is.

## Front matter is required

Every content file must start with front matter delimiters. Alloy supports YAML (`---`), TOML (`+++`), and JSON (`{}`):

```markdown
---
title: "My First Post"
date: 2026-04-10
tags: ["tutorial", "getting-started"]
---

This is the body of the post. It supports **Markdown**, Liquid template
tags like {{ site.title }}, and raw HTML.
```

Empty front matter is valid -- use `---` followed by `---` on the next line when you have no metadata to set. Files without any front matter delimiters produce a build error.

## How content is processed

Every content file passes through a multi-stage pipeline:

1. **Discovery** -- Alloy walks the `content/` directory and identifies content files by extension.
2. **Front matter extraction** -- Metadata is parsed from the top of each file.
3. **Data cascade** -- Global data, directory data (`_data.yaml`), and front matter are merged. See [Data Cascade](/content/data-cascade/).
4. **Permalink resolution** -- The output URL is computed from front matter, cascade patterns, or the file path. See [Permalinks](/content/permalinks/).
5. **Content rendering** -- Markdown is converted to HTML. Template tags (`{{ }}`, `{% %}`) are evaluated.
6. **Layout wrapping** -- The rendered content is injected into a layout template as `{{ content }}`.
7. **Output** -- The final HTML is written to `_site/`.

For a deeper look at each stage, see [Content Lifecycle](/content/lifecycle/).

## Markdown configuration

Alloy uses goldmark for Markdown rendering. Configure it in `alloy.config.yaml`:

```yaml
content:
  markdown:
    goldmark:
      unsafe: true           # Allow raw HTML in Markdown (default: true; set false to escape it)
      typographer: true      # Smart quotes and dashes (default: false)
      templateTags: true     # Treat {{ }} and {% %} as template syntax in Markdown (default: true; false = literal text)
      autoHeadingID: true    # Add id attributes to headings (default: true)
      customElements: true   # Keep multi-line custom element blocks intact (default: true)
```

With `customElements: true` (the default), a block-level custom element (any tag containing a dash, like `<my-gallery>`) is kept intact through Markdown rendering -- including blank lines and nested elements inside it -- instead of being split at the first blank line as standard HTML blocks are.

With `templateTags: true` (the default), Liquid and Go template syntax passes through the Markdown parser untouched. You can mix Markdown and template logic in the same file without escaping.

Set `templateTags: false` to disable template syntax recognition in Markdown files. `{{ }}` and `{% %}` in body text render as literal characters instead of being evaluated as Liquid. Use this for sites whose content discusses other template languages (Mustache, Handlebars, Go templates) and does not need Liquid processing in content files. Template syntax inside fenced code blocks and inline code is always escaped for display regardless of this setting.

## Table of contents

Alloy extracts the heading structure from each Markdown page during rendering and exposes it as `page.toc`:

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

Each TOC entry has:

| Field | Type | Description |
|---|---|---|
| `id` | string | The heading's `id` attribute (auto-generated or `{#custom-id}` override) |
| `text` | string | Plain text content of the heading |
| `level` | int | Heading level (2--6; h1 is excluded) |
| `children` | array | Nested headings one level deeper |

TOC data is always extracted during Markdown rendering. The `goldmark.autoHeadingID` setting controls whether headings get `id` attributes in the HTML output -- without them, TOC anchor links have nothing to target.

## HTML content files

`.html` files in `content/` are classified based on their content:

1. **Has front matter** -- Processed as a content page, same as Markdown.
2. **No front matter + starts with `<!DOCTYPE` or `<html>`** -- Treated as a full HTML document and copied to output as-is (passthrough).
3. **No front matter + no DOCTYPE** -- Treated as an HTML fragment. The file body becomes the page content with empty front matter, inheriting its layout from the `_data.yaml` cascade.

This classification lets you co-locate standalone HTML documents alongside templated content without adding front matter to every file.

## Co-located assets

Non-content files inside `content/` are automatically copied to the output, preserving their relative path. This enables page bundles where a post and its images live together:

```
content/blog/my-post/
├── index.md          # Content file → _site/blog/my-post/index.html
├── diagram.svg       # Passthrough → _site/blog/my-post/diagram.svg
└── hero.png          # Passthrough → _site/blog/my-post/hero.png
```

Reference co-located assets with relative paths in your Markdown: `![Diagram](diagram.svg)`.

## Custom directory structure

Override the default `content/` path in config:

```yaml
# alloy.config.yaml
structure:
  content: "./docs/pages/"
```

All pipeline stages, the file watcher, and the dev server respect the configured path.

## What to read next

- [Front Matter](/content/front-matter/) -- All available front matter fields
- [Data Cascade](/content/data-cascade/) -- How global, directory, and page data merge
- [Permalinks](/content/permalinks/) -- URL patterns and tokens
- [Pagination](/content/pagination/) -- List pages and virtual page generation
- [Content Lifecycle](/content/lifecycle/) -- Draft, future, and expired content
- [Data Files](/content/data-files/) -- Global data from `data/` and external sources
