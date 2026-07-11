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
| `render-heading` | `inner` (rendered HTML), `text` (plain text), `level` (1–6), `id` (auto-generated slug or `{#custom-id}` override), `attributes` |
| `render-image` | `src`, `alt`, `title` |
| `render-link` | `destination`, `text` (plain link text), `title` (from `[text](url "title")`), `is_external` (boolean) |
| `render-table` | `inner` (rendered inner HTML), `attributes` |

`markup.attributes` is a map of key-value pairs from [block-level attribute syntax](#block-level-attributes). It is an empty map (not nil) when no attributes are present on the element. `render-image` does not receive attributes -- images are inline elements and do not support attribute syntax.

The `markup` object is the only context available inside a render hook -- `page.*` and `site.*` are not passed to hook templates. Hooks are pure element transformers.

## Engine selection

Render hook templates follow the configured template engine:

| Engine | Config value | Hook file extension | Syntax |
|---|---|---|---|
| Liquid | `"liquid"` (default) | `.liquid` | `{{ markup.language }}` |
| Go templates | `"gotemplate"` or `"go"` | `.html` | `{{ .markup.language }}` |

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

{% raw %}
<wa-tab-group>
<wa-tab slot="nav" panel="hook-link-liquid" active>Liquid</wa-tab>
<wa-tab slot="nav" panel="hook-link-go">Go templates</wa-tab>

<wa-tab-panel name="hook-link-liquid" active>
<alloy-code lang="liquid">&lt;!-- layouts/_markup/render-link.liquid --&gt;
{% if markup.is_external %}
  &lt;a href="{{ markup.destination }}" target="_blank" rel="noopener"&gt;{{ markup.text }} ↗&lt;/a&gt;
{% else %}
  &lt;a href="{{ markup.destination }}"&gt;{{ markup.text }}&lt;/a&gt;
{% endif %}</alloy-code>
</wa-tab-panel>
<wa-tab-panel name="hook-link-go">
<alloy-code lang="html">&lt;!-- layouts/_markup/render-link.html --&gt;
{{ if .markup.is_external }}
  &lt;a href="{{ .markup.destination }}" target="_blank" rel="noopener"&gt;{{ .markup.text }} ↗&lt;/a&gt;
{{ else }}
  &lt;a href="{{ .markup.destination }}"&gt;{{ .markup.text }}&lt;/a&gt;
{{ end }}</alloy-code>
</wa-tab-panel>
</wa-tab-group>
{% endraw %}

### Mermaid diagrams

Render mermaid code blocks as diagram containers instead of code:

```liquid
<!-- layouts/_markup/render-codeblock-mermaid.liquid -->
<div class="mermaid">{{ markup.inner }}</div>
```

Standard code blocks still use the generic `render-codeblock.liquid` (or default rendering). Only blocks tagged with ` ```mermaid ` are affected.

### Auto-linked headings

Add anchor links to headings. `markup.inner` contains the rendered HTML (so `## Hello **world**` gives `Hello <strong>world</strong>`), while `markup.text` contains the plain text (`Hello world`):

```liquid
<!-- layouts/_markup/render-heading.liquid -->
<h{{ markup.level }} id="{{ markup.id }}">
  <a href="#{{ markup.id }}" class="anchor">#</a>
  {{ markup.inner }}
</h{{ markup.level }}>
```

The `markup.id` is auto-generated from the heading text using Alloy's `slugify` function (e.g., "My Section" becomes `my-section`). A `{#custom-id}` attribute overrides the auto-generated slug.

### Styled code blocks with attributes

Use `markup.attributes` to pass classes and IDs from Markdown to your hook output:

```liquid
<!-- layouts/_markup/render-codeblock.liquid -->
<pre class="{{ markup.attributes.class }}"{% if markup.attributes.id %} id="{{ markup.attributes.id }}"{% endif %}>
  <code data-lang="{{ markup.language }}">{{ markup.inner }}</code>
</pre>
```

## Block-level attributes

Alloy supports `{.class #id key=value}` attribute syntax on block elements. Attributes are automatically enabled when `autoHeadingID` is true (the default).

Supported elements:

- **Headings** -- on the same line: `## My Heading {.subtitle #intro}`
- **Fenced code blocks** -- on the opening fence: `` ```go {.highlight #example} ``
- **Blockquotes** -- trailing line after the quote: `{.callout}`
- **Tables** -- trailing line after the table: `{.striped}`

````markdown
```go {.highlight #example}
fmt.Println("hello")
```

> This is important
{.callout}

| Name  | Role     |
| ----- | -------- |
| Alice | Engineer |
{.striped}
````

Attributes are exposed to render hooks via `markup.attributes`. When no attributes are present, `markup.attributes` is an empty map.

Images and links are inline elements and do not support attribute syntax.

## Template tag escaping

The `markup.inner` content in code block hooks is already escaped by Goldmark — `{{ }}` and `{% %}` inside code fences are protected from Liquid processing. No additional escaping is needed in your render hook template.

## Related

- [Templates Overview](/templates/) — template engines, context, rendering pipeline
- [Layouts](/templates/layouts/) — layout chaining and resolution
- [Shortcodes](/templates/shortcodes/) — reusable content snippets
- [Filters](/templates/filters/) — built-in template filters
