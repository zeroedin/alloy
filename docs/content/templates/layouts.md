---
layout: doc
title: Layouts
nav_weight: 20
description: "How layouts wrap page content in shared HTML structure using the {{ content }} placeholder."
---

Layouts wrap your page content in shared HTML structure. Every content page is rendered into a layout, which injects the page body via `{{ content }}`.

{% raw %}
<wa-tab-group>
<wa-tab slot="nav" panel="layout-liquid" active>Liquid</wa-tab>
<wa-tab slot="nav" panel="layout-go">Go templates</wa-tab>

<wa-tab-panel name="layout-liquid" active>
<alloy-code lang="liquid">&lt;!-- layouts/default.liquid --&gt;
&lt;!DOCTYPE html&gt;
&lt;html lang="{{ site.language }}"&gt;
&lt;head&gt;
  &lt;meta charset="utf-8"&gt;
  &lt;title&gt;{{ page.title }} - {{ site.title }}&lt;/title&gt;
&lt;/head&gt;
&lt;body&gt;
  {% include "partials/header" %}
  &lt;main&gt;{{ content }}&lt;/main&gt;
  {% include "partials/footer" %}
&lt;/body&gt;
&lt;/html&gt;</alloy-code>
</wa-tab-panel>
<wa-tab-panel name="layout-go">
<alloy-code lang="html">&lt;!-- layouts/default.html --&gt;
&lt;!DOCTYPE html&gt;
&lt;html lang="{{ .site.language }}"&gt;
&lt;head&gt;
  &lt;meta charset="utf-8"&gt;
  &lt;title&gt;{{ .page.title }} - {{ .site.title }}&lt;/title&gt;
&lt;/head&gt;
&lt;body&gt;
  &lt;main&gt;{{ .content }}&lt;/main&gt;
&lt;/body&gt;
&lt;/html&gt;</alloy-code>
</wa-tab-panel>
</wa-tab-group>
{% endraw %}

The `{{ content }}` variable holds the fully rendered body of the current page. For Markdown files, this is already converted to HTML before the layout is applied.

## Layout resolution order

Alloy resolves layouts through a predictable lookup chain. The configured engine determines the extension checked at each step: `.liquid` for the Liquid engine, `.html` for the Go template engine.

### Blog-like sections

Sections with date-based permalink patterns (containing `:year`, `:month`, or `:day` tokens in `_data.yaml`) use special resolution.

**Index file** (`content/blog/index.md`):

1. `layout:` from front matter or `_data.yaml` cascade
2. `layouts/blog.liquid` (section name)
3. `layouts/index.liquid` (filename match)
4. `layouts/default.liquid` (fallback)
5. Build error

**Child file** (`content/blog/my-post.md`):

1. `layout:` from front matter or `_data.yaml` cascade
2. `layouts/post.liquid` (child of date-based section)
3. `layouts/my-post.liquid` (filename match)
4. `layouts/default.liquid` (fallback)
5. Build error

### Regular sections and standalone pages

Pages in sections without date-based permalinks resolve through a simpler chain.

**Any file** (`content/docs/getting-started.md`):

1. `layout:` from front matter or `_data.yaml` cascade
2. `layouts/getting-started.liquid` (filename match)
3. `layouts/default.liquid` (fallback)
4. Build error

### Taxonomy pages

Auto-generated taxonomy pages check their own path first:

1. `layouts/taxonomies/<name>.liquid` (e.g., `layouts/taxonomies/tags.liquid`)
2. `layouts/<name>.liquid`

## Specifying a layout

Set the layout explicitly in front matter:

```yaml
---
title: "About Us"
layout: "page"
---
```

Or apply a layout to an entire directory via `_data.yaml`:

```yaml
# content/docs/_data.yaml
layout: "doc"
```

All pages in `content/docs/` and subdirectories inherit this layout unless they override it in their own front matter. Front matter always takes priority over the cascade.

### Disabling layout wrapping

Set `layout: false` to output the page body without any layout wrapper:

```yaml
---
title: "Raw HTML Page"
layout: false
---
```

This is useful for pages that are complete HTML documents on their own, or for data-only pages consumed by other templates.

## Layout chaining

Layouts can reference a parent layout via a `layout:` directive in their front matter. The pipeline renders inside-out: page content flows into the innermost layout, which flows into the parent, and so on up the chain.

```liquid
<!-- layouts/has-toc.liquid -->
---
layout: "base"
---
<div class="with-toc">
  <aside>{% include "partials/toc" %}</aside>
  <main>{{ content }}</main>
</div>
```

```liquid
<!-- layouts/base.liquid -->
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

A page using `layout: "has-toc"` renders as: page body -> `has-toc` -> `base`. Each level injects `{{ content }}` from the level below.

Layout front matter is stripped before rendering -- only the `layout:` directive is used. Other front matter keys in layout files are ignored.

### Circular layout detection

Alloy scans all layout files at build start and fails the build if a cycle exists (e.g., `a -> b -> a`). Layout chains are capped at 10 levels to prevent infinite loops from malformed configurations.

## Accessing page data

All front matter fields are available inside layouts via the `page` object:

{% raw %}
<wa-tab-group>
<wa-tab slot="nav" panel="pagedata-liquid" active>Liquid</wa-tab>
<wa-tab slot="nav" panel="pagedata-go">Go templates</wa-tab>

<wa-tab-panel name="pagedata-liquid" active>
<alloy-code lang="liquid">&lt;article class="post"&gt;
  &lt;h1&gt;{{ page.title }}&lt;/h1&gt;
  &lt;time datetime="{{ page.date | date: '%Y-%m-%d' }}"&gt;
    {{ page.date | date: "%B %d, %Y" }}
  &lt;/time&gt;
  {% if page.summary %}
    &lt;p class="summary"&gt;{{ page.summary }}&lt;/p&gt;
  {% endif %}
  {{ content }}
  {% if page.tags %}
    &lt;ul class="tags"&gt;
      {% for tag in page.tags %}
        &lt;li&gt;&lt;a href="/tags/{{ tag | slugify }}/"&gt;{{ tag }}&lt;/a&gt;&lt;/li&gt;
      {% endfor %}
    &lt;/ul&gt;
  {% endif %}
&lt;/article&gt;</alloy-code>
</wa-tab-panel>
<wa-tab-panel name="pagedata-go">
<alloy-code lang="html">&lt;article class="post"&gt;
  &lt;h1&gt;{{ .page.title }}&lt;/h1&gt;
  &lt;time datetime="{{ date .page.date "%Y-%m-%d" }}"&gt;
    {{ date .page.date "%B %d, %Y" }}
  &lt;/time&gt;
  {{ if .page.summary }}
    &lt;p class="summary"&gt;{{ .page.summary }}&lt;/p&gt;
  {{ end }}
  {{ .content }}
  {{ if .page.tags }}
    &lt;ul class="tags"&gt;
      {{ range .page.tags }}
        &lt;li&gt;&lt;a href="/tags/{{ slugify . }}/"&gt;{{ . }}&lt;/a&gt;&lt;/li&gt;
      {{ end }}
    &lt;/ul&gt;
  {{ end }}
&lt;/article&gt;</alloy-code>
</wa-tab-panel>
</wa-tab-group>
{% endraw %}

Custom front matter fields work the same way. If your content defines `author: "Alice"` in front matter, the layout accesses it as `{{ page.author }}`.

## Partials and includes

Partials are reusable template fragments stored in `layouts/partials/` (by convention -- any path under `layouts/` works). Both engines resolve partials from the layouts directory.

{% raw %}
<wa-tab-group>
<wa-tab slot="nav" panel="partials-liquid" active>Liquid</wa-tab>
<wa-tab slot="nav" panel="partials-go">Go templates</wa-tab>

<wa-tab-panel name="partials-liquid" active>
<alloy-code lang="liquid">{% include "partials/header" %}
{% include "partials/footer" %}
{% render "partials/social-links" %}</alloy-code>

Both tags resolve paths relative to the `layouts/` directory, trying `name.liquid`, then `name.html`, then the bare name. The difference: `{% include %}` shares the parent template's variable scope, while `{% render %}` creates an isolated scope (variables from the parent are not accessible unless explicitly passed).
</wa-tab-panel>
<wa-tab-panel name="partials-go">
<alloy-code lang="html">{{ include "partials/header" }}
{{ include "partials/footer" }}
{{ include "partials/social-links" }}</alloy-code>

The `include` function resolves paths relative to the `layouts/` directory, trying `name.html`, then the bare name. Context is optional -- with no argument, the include inherits the current template context (like Liquid's `{% include %}`). Pass an explicit context to narrow scope:

<alloy-code lang="html">{{ include "partials/card" (dict "item" . "compact" true) }}</alloy-code>

Unlike Go's built-in `{{ template }}` action, `include` is a function -- its output can be captured in variables and used in pipelines:

<alloy-code lang="html">{{ $nav := include "partials/nav" }}
{{ if $nav }}&lt;div class="has-nav"&gt;{{ $nav }}&lt;/div&gt;{{ end }}</alloy-code>
</wa-tab-panel>
</wa-tab-group>
{% endraw %}

Plugin-registered filters work inside partials in both engines -- the same filter dispatch mechanism applies to all template files. Partials can include other partials (nesting is capped at 100 levels).

### Go template engine

With the Go template engine (`engine: "gotemplate"`), layouts are `.html` files using Go syntax. The same context is available with a leading dot: `{{ .page.title }}`, `{{ .site.title }}`, `{{ .content }}`:

```html
<!-- layouts/default.html -->
<!DOCTYPE html>
<html>
<head><title>{{ .page.title }} - {{ .site.title }}</title></head>
<body>
  {{ .content }}
</body>
</html>
```

Layout chaining works identically in both engines -- a `layout:` directive in the layout's front matter names the parent (see [Layout chaining](#layout-chaining)). Cross-file includes use the `{{ include }}` function (see [Partials and includes](#partials-and-includes)). `{% render %}` and `{% inline %}` are Liquid-only tags.

Two helper functions are registered for working with ordered map data (JSON data files preserve key order via an ordered map type that Go's index syntax cannot address):

```html
{{ oget .site.data.config "title" }}          <!-- ordered-map lookup -->
{{ range orange .site.data.nav }}              <!-- ordered-map iteration -->
  <a href="{{ oget .Value "url" }}">{{ .Key }}</a>
{{ end }}
```

## Content-relative file inlining

The `{% inline %}` tag reads a file relative to the current content file and inserts its raw contents. No template processing occurs -- the file is inserted verbatim.

```markdown
<!-- content/about/index.md -->
# About

{% inline "./about-diagram.svg" %}
```

This is useful for SVGs that need to respond to CSS custom properties and cannot be loaded as `<img>` tags.

**Rules:**
- Paths are resolved relative to the content file's directory; absolute paths are rejected
- The resolved path must stay within the content root directory
- Binary file types (images, fonts, audio/video, archives, PDF) are rejected with a build error -- use `<img>` for images instead
- `{% inline %}` is a Liquid tag; it is not available in the Go template engine

## Table of contents

Alloy extracts heading structure from Markdown pages and exposes it as `page.toc`. Build a TOC partial to render navigation:

```liquid
<!-- layouts/partials/toc.liquid -->
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

Each TOC entry has `id` (heading anchor), `text` (plain text), `level` (2-6), and `children` (nested headings). TOC extraction is controlled by `content.markdown.toc` (default: `true`). Heading anchor IDs are controlled separately by `content.markdown.goldmark.autoHeadingID` -- disabling TOC does not disable heading IDs (see [Content](/content/)).
