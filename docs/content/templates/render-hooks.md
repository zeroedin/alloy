---
layout: doc
title: Render Hooks
nav_weight: 25
description: "Override how Markdown elements render to HTML using Liquid templates in layouts/_markup/."
---

Render hooks let you replace Goldmark's default HTML output for specific Markdown elements with your own Liquid templates. Place a template in `layouts/_markup/` and Alloy uses it instead of the default rendering for that element type.

```
layouts/_markup/
├── render-blockquote.liquid
├── render-codeblock.liquid
├── render-codeblock-mermaid.liquid
├── render-heading.liquid
├── render-image.liquid
├── render-link.liquid
└── render-table.liquid
```

If no render hook template exists for an element type, Goldmark's default rendering applies. Render hooks run during Markdown rendering — before template tag processing and layout rendering.

## Supported hooks

| Template file | Markdown element |
|---|---|
| `render-blockquote.liquid` | Blockquotes (`>`) |
| `render-codeblock.liquid` | Fenced code blocks (` ``` `) |
| `render-codeblock-{lang}.liquid` | Language-specific code blocks |
| `render-heading.liquid` | Headings (`#`, `##`, `###`, etc.) |
| `render-image.liquid` | Images (`![alt](src)`) |
| `render-link.liquid` | Links (`[text](url)`) |
| `render-table.liquid` | Tables (`| ... |`) |

## Template context

Each render hook template receives a `markup` object with properties specific to the element type:

| Template | `markup.*` properties |
|---|---|
| `render-blockquote` | `inner` (rendered inner HTML), `attributes` |
| `render-codeblock` | `inner` (raw code text), `language`, `attributes` |
| `render-heading` | `inner` (rendered inner HTML), `level` (1–6), `id` (auto-generated slug), `text` (plain text, no HTML) |
| `render-image` | `src`, `alt`, `title`, `attributes` |
| `render-link` | `destination`, `text` (rendered inner HTML), `title`, `is_external` (boolean) |
| `render-table` | `inner` (rendered inner HTML), `attributes` |

The full `page.*` and `site.*` context is also available — render hooks can access front matter, site data, collections, and taxonomies.

## Engine selection

Render hook templates follow the configured template engine:

| Engine | Config value | Hook file extension | Syntax |
|---|---|---|---|
| Liquid | `"liquid"` (default) | `.liquid` | `{{ markup.language }}` |
| Go templates | `"go"` | `.html` | `{{ .markup.language }}` |

## Language-specific code blocks

`render-codeblock-{language}.liquid` overrides rendering for a specific fenced code block language. For example, `render-codeblock-mermaid.liquid` handles only mermaid blocks.

Lookup order:

1. `render-codeblock-{language}.liquid` — language-specific match
2. `render-codeblock.liquid` — generic fallback
3. Default Goldmark rendering — no hook exists

## Examples

### Custom code block rendering

Output a web component instead of the default `<pre><code>`:

```liquid
<!-- layouts/_markup/render-codeblock.liquid -->
<alloy-code lang="{{ markup.language }}">{{ markup.inner }}</alloy-code>
```

Pair this with a build plugin that applies syntax highlighting to `<alloy-code>` elements during `onPageRendered`, and the component handles copy-to-clipboard and filename display on the client.

### External link detection

Add `target="_blank"` and an icon to external links:

```liquid
<!-- layouts/_markup/render-link.liquid -->
{% if markup.is_external %}
  <a href="{{ markup.destination }}" target="_blank" rel="noopener">{{ markup.text }} ↗</a>
{% else %}
  <a href="{{ markup.destination }}">{{ markup.text }}</a>
{% endif %}
```

### Mermaid diagrams

Render mermaid code blocks as diagram containers instead of code:

```liquid
<!-- layouts/_markup/render-codeblock-mermaid.liquid -->
<div class="mermaid">{{ markup.inner }}</div>
```

Standard code blocks still use the generic `render-codeblock.liquid` (or default rendering). Only blocks tagged with ` ```mermaid ` are affected.

### Auto-linked headings

Add anchor links to headings:

```liquid
<!-- layouts/_markup/render-heading.liquid -->
<h{{ markup.level }} id="{{ markup.id }}">
  <a href="#{{ markup.id }}" class="anchor">#</a>
  {{ markup.inner }}
</h{{ markup.level }}>
```

The `markup.id` is auto-generated from the heading text using Alloy's `slugify` function (e.g., "My Section" becomes `my-section`).

## Template tag escaping

The `markup.inner` content in code block hooks is already escaped by Goldmark — `{{ }}` and `{% %}` inside code fences are protected from Liquid processing. No additional escaping is needed in your render hook template.

## Related

- [Templates Overview](/templates/) — template engines, context, rendering pipeline
- [Layouts](/templates/layouts/) — layout chaining and resolution
- [Shortcodes](/templates/shortcodes/) — reusable content snippets
- [Filters](/templates/filters/) — built-in template filters
