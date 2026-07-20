---
layout: doc
title: Templates Overview
nav_weight: 10
description: "An overview of Alloy's Liquid and Go html/template engines, layout lookup, and the data available during rendering."
---

Alloy uses the [Liquid](https://liquidmarkup.org/) template language by default. Templates live in the `layouts/` directory, are parsed once at startup, and rendered per page with full access to the site's data cascade.

```yaml
# alloy.config.yaml
templates:
  engine: "liquid"   # default; also supports "gotemplate" or "go"
```

{% raw %}
<wa-tab-group>
<wa-tab slot="nav" panel="overview-liquid" active>Liquid</wa-tab>
<wa-tab slot="nav" panel="overview-go">Go templates</wa-tab>

<wa-tab-panel name="overview-liquid" active>
<alloy-code language="liquid">&lt;!-- layouts/default.liquid --&gt;
&lt;!DOCTYPE html&gt;
&lt;html&gt;
&lt;head&gt;&lt;title&gt;{{ page.title }}&lt;/title&gt;&lt;/head&gt;
&lt;body&gt;
  {% include "partials/header" %}
  {{ content }}
  {% include "partials/footer" %}
&lt;/body&gt;
&lt;/html&gt;</alloy-code>
</wa-tab-panel>
<wa-tab-panel name="overview-go">
<alloy-code language="html">&lt;!-- layouts/default.html --&gt;
&lt;!DOCTYPE html&gt;
&lt;html&gt;
&lt;head&gt;&lt;title&gt;{{ .page.title }}&lt;/title&gt;&lt;/head&gt;
&lt;body&gt;
  {{ include "partials/header" }}
  {{ .content }}
  {{ include "partials/footer" }}
&lt;/body&gt;
&lt;/html&gt;</alloy-code>
</wa-tab-panel>
</wa-tab-group>
{% endraw %}

Every content page is rendered through its resolved layout. The layout receives the page's rendered body as `{{ content }}` and can access all front matter fields via the `page` object.

## Template engines

Alloy supports two built-in (Tier 1) template engines. The engine is a global, project-wide setting -- one engine is active per build.

| Engine | Config value | File extension | Syntax |
|---|---|---|---|
| Liquid | `"liquid"` (default) | `.liquid` | `{{ var }}`, `{% tag %}` |
| Go templates | `"gotemplate"` or `"go"` | `.html` | `{{ .var }}`, `{{ range }}` |

Both engines receive the same `map[string]any` context from the data cascade. All built-in [filters](/templates/filters/) are registered in both engines at startup -- in Go templates, filters are called as functions (`{{ upcase .page.title }}`) rather than with pipe-and-colon syntax. See [Filter syntax by engine](/templates/filters/#filter-syntax-by-engine).

Set the engine to `"gotemplate"` (or the shorthand `"go"`) for Go templates. Any unrecognized value fails config validation with an error.

Plugin-provided template engines (Nunjucks, EJS, Pug via the Node bridge) are an experimental design concept and are **not implemented** -- today the `engine` setting accepts only `"liquid"` and `"gotemplate"` (or `"go"`), and unrecognized values are rejected at startup.

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

{% raw %}
<wa-tab-group>
<wa-tab slot="nav" panel="context-liquid" active>Liquid</wa-tab>
<wa-tab slot="nav" panel="context-go">Go templates</wa-tab>

<wa-tab-panel name="context-liquid" active>
<alloy-code language="liquid">&lt;h1&gt;{{ page.title }}&lt;/h1&gt;
&lt;time&gt;{{ page.date | date: "%B %d, %Y" }}&lt;/time&gt;
{{ content }}

&lt;h2&gt;Recent posts&lt;/h2&gt;
{% for post in collections.blog limit: 5 %}
  &lt;a href="{{ post.url }}"&gt;{{ post.title }}&lt;/a&gt;
{% endfor %}</alloy-code>
</wa-tab-panel>
<wa-tab-panel name="context-go">
<alloy-code language="html">&lt;h1&gt;{{ .page.title }}&lt;/h1&gt;
&lt;time&gt;{{ date .page.date "%B %d, %Y" }}&lt;/time&gt;
{{ .content }}

&lt;h2&gt;Recent posts&lt;/h2&gt;
{{ range limit .collections.blog 5 }}
  &lt;a href="{{ .url }}"&gt;{{ .title }}&lt;/a&gt;
{{ end }}</alloy-code>
</wa-tab-panel>
</wa-tab-group>
{% endraw %}

## Template resolution

The `.liquid` extension marks a file as a Liquid template. The extension before `.liquid` determines the output format:

```
layouts/default.liquid          --> HTML output (default)
layouts/feed.xml.liquid         --> XML output
layouts/api.json.liquid         --> JSON output
```

The configured engine determines which extensions are checked first. The Liquid engine looks for `.liquid` files and falls back to bare extensions (e.g., `default.html`) when no `.liquid` file is found, parsing the bare file as Liquid. The Go template engine resolves `.html` files only.

```
layouts/
├── default.liquid       <-- used when engine: "liquid"
├── default.html         <-- used when engine: "gotemplate" (or Liquid fallback)
├── feed.xml.liquid      <-- XML format layout (Liquid)
└── feed.xml             <-- XML format layout (Go templates, or Liquid fallback)
```

If a file contains syntax for the wrong engine, the build fails with a parse error. Alloy does not inspect file contents to determine the engine.

## Rendering pipeline

Alloy renders content in a strict order:

1. **Markdown rendering** -- Goldmark parses `.md` files into HTML. Template tags (`{{ }}`, `{% %}`) are preserved through Markdown via a custom goldmark extension.
2. **Template rendering** -- The configured engine evaluates Liquid or Go template syntax in the rendered output and in layouts.
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

   `{% raw %}` is a Liquid tag. Under the Go template engine, emit literal braces with a quoted expression instead: `{{ "{{" }}name{{ "}}" }}`.
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
