---
layout: doc
title: Layouts
nav_weight: 20
description: "How layouts wrap page content in shared HTML structure using the {{ content }} placeholder."
---

Layouts wrap your page content in shared HTML structure. Every content page is rendered into a layout, which injects the page body via `{{ content }}`.

```liquid
<!-- layouts/default.liquid -->
<!DOCTYPE html>
<html lang="{{ site.language }}">
<head>
  <meta charset="utf-8">
  <title>{{ page.title }} - {{ site.title }}</title>
</head>
<body>
  {% include "partials/header" %}
  <main>{{ content }}</main>
  {% include "partials/footer" %}
</body>
</html>
```

The `{{ content }}` variable holds the fully rendered body of the current page. For Markdown files, this is already converted to HTML before the layout is applied.

## Layout resolution order

Alloy resolves layouts through a predictable lookup chain. At each step it tries `.liquid` first, then falls back to the bare extension (e.g., `.html`). Both are read as Liquid.

### Blog-like sections

Sections with date-based permalink patterns (containing `:year`, `:month`, or `:day` tokens in `_data.yaml`) use special resolution.

**Index file** (`content/blog/index.md`):

1. `layout:` from front matter or `_data.yaml` cascade
2. `layouts/blog.liquid` → `layouts/blog.html` (section name)
3. `layouts/default.liquid` → `layouts/default.html` (fallback)
4. Build error

**Child file** (`content/blog/my-post.md`):

1. `layout:` from front matter or `_data.yaml` cascade
2. `layouts/post.liquid` → `layouts/post.html` (child of date-based section)
3. `layouts/default.liquid` → `layouts/default.html` (fallback)
4. Build error

### Regular sections and standalone pages

Pages in sections without date-based permalinks resolve through a shorter chain.

**Any file** (`content/docs/getting-started.md`):

1. `layout:` from front matter or `_data.yaml` cascade
2. `layouts/default.liquid` → `layouts/default.html` (fallback)
3. Build error

Layouts do not match by filename. If you relied on `layouts/getting-started.liquid` being picked up automatically, add `layout: "getting-started"` to the page's front matter or `_data.yaml`.

### Taxonomy pages

Auto-generated taxonomy pages check their own path first:

1. `layouts/taxonomies/<name>.liquid` → `layouts/taxonomies/<name>.html` (e.g., `layouts/taxonomies/tags.liquid`)
2. `layouts/<name>.liquid` → `layouts/<name>.html`

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

### Extension-bearing layout names

A layout name that includes a recognized extension (`.liquid`, `.html`, `.xml`, `.json`, `.txt`) is used as a literal filename -- no `.liquid` extension is appended:

```yaml
---
layout: "base.html"   # resolves to layouts/base.html exactly
---
```

Bare names (without an extension) get the usual lookup: `base` → `layouts/base.liquid` → `layouts/base.html`.

Extension-bearing layout names cannot be used with [output formats](/templates/output-formats/) -- `layout: "article.liquid"` with `outputs: ["html", "json"]` produces a build error with a fix-it message suggesting `layout: "article"` instead.

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

```liquid
<article class="post">
  <h1>{{ page.title }}</h1>
  <time datetime="{{ page.date | date: '%Y-%m-%d' }}">
    {{ page.date | date: "%B %d, %Y" }}
  </time>
  {% if page.summary %}
    <p class="summary">{{ page.summary }}</p>
  {% endif %}
  {{ content }}
  {% if page.tags %}
    <ul class="tags">
      {% for tag in page.tags %}
        <li><a href="/tags/{{ tag | slugify }}/">{{ tag }}</a></li>
      {% endfor %}
    </ul>
  {% endif %}
</article>
```

Custom front matter fields work the same way. If your content defines `author: "Alice"` in front matter, the layout accesses it as `{{ page.author }}`.

## Partials and includes

Partials are reusable template fragments stored in `layouts/partials/` (by convention -- any path under `layouts/` works). Partials resolve from the layouts directory.

{% raw %}
```liquid
{% include "partials/header" %}
{% include "partials/footer" %}
{% render "partials/social-links" %}
```

Both tags resolve paths relative to the `layouts/` directory, trying `name.liquid`, then `name.html`, then the bare name. The difference: `{% include %}` shares the parent template's variable scope, while `{% render %}` creates an isolated scope (variables from the parent are not accessible unless explicitly passed).
{% endraw %}

Plugin-registered filters work inside partials -- the same filter dispatch mechanism applies to all template files. Partials can include other partials (nesting is capped at 100 levels).

## Content-relative file inlining

The `{% inline %}` tag reads a file relative to the current content file and inserts its raw contents. No template processing occurs -- the file is inserted verbatim.

```markdown
<!-- content/about/index.md -->
# About

{% inline "./about-diagram.svg" %}
```

This is useful for SVGs that need to respond to CSS custom properties and cannot be loaded as `<img>` tags.

**Rules:**
- Paths must start with `./` or `../` -- bare paths like `{% inline "diagram.svg" %}` are rejected
- The resolved path must stay within the content root directory
- Only text file types are accepted: `.svg`, `.html`, `.htm`, `.txt`, `.css`, `.js`, `.json`, `.xml`, `.toml`, `.yaml`, `.yml`, `.md`. Binary types like `.png` produce a build error suggesting `<img>` instead

### Symlinks in layouts

Symlinks inside the `layouts/` directory are followed. A symlink at `layouts/partials/shared.liquid` pointing to a file outside the project root is valid -- `{% include %}` and `{% render %}` read the symlink target. The path traversal check applies to the logical path within the layouts tree, not the symlink's physical target.

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
