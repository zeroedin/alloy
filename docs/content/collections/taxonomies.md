---
layout: doc
title: Taxonomies
nav_weight: 20
description: "Cross-cutting taxonomies group content by front matter values like tags, with auto-generated index and term pages."
---

Taxonomies create cross-cutting groups from front matter values. A blog post and a docs page can both be tagged "javascript" and appear in the same taxonomy collection. Alloy auto-generates index and per-term pages from your taxonomy declarations.

```yaml
# alloy.config.yaml
taxonomies:
  tags:
  categories:
```

```yaml
# content/blog/web-components.md
---
title: "Building Web Components"
tags: ["javascript", "web-components", "lit"]
categories: ["tutorials"]
---
```

This populates `taxonomies.tags.javascript`, `taxonomies.tags.web-components`, `taxonomies.tags.lit`, and `taxonomies.categories.tutorials` -- each containing a list of pages with that term.

## Declaring Taxonomies

Taxonomies are declared in `alloy.config.yaml`. The declaration tells Alloy which front matter keys organize pages into named groups.

### Simple Declaration

```yaml
taxonomies:
  tags:
  categories:
  series:
```

Each key uses defaults: `tags` generates pages at `/tags/` and `/tags/:slug/`, using a layout named `tags`.

### Customized Declaration

```yaml
taxonomies:
  tags:
    permalink: "/topics/:slug/"
    layout: "topic"
  categories:
    permalink: "/sections/:slug/"
    layout: "term"
```

### Full Options

| Option | Default | Description |
|---|---|---|
| `permalink` | `"/<name>/:slug/"` | URL pattern for term pages |
| `layout` | taxonomy name | Layout template to use |
| `render` | `true` | Whether to generate output pages |

## Taxonomy Pages

When `render: true` (the default), Alloy generates two kinds of pages per taxonomy:

- **Index page** (`/tags/`) -- lists all terms
- **Term pages** (`/tags/javascript/`, `/tags/lit/`) -- lists pages with that term

Both use the same layout template. The `taxonomy` object in the template context tells you which type you are rendering:

{% raw %}
<wa-tab-group>
<wa-tab slot="nav" panel="taxpage-liquid" active>Liquid</wa-tab>
<wa-tab slot="nav" panel="taxpage-go">Go templates</wa-tab>

<wa-tab-panel name="taxpage-liquid" active>
<alloy-code lang="liquid">{% if taxonomy.term %}
  &lt;!-- Rendering /tags/javascript/ --&gt;
  &lt;h1&gt;{{ taxonomy.term }}&lt;/h1&gt;
  {% for post in taxonomy.pages %}
    &lt;a href="{{ post.url }}"&gt;{{ post.title }}&lt;/a&gt;
  {% endfor %}
{% else %}
  &lt;!-- Rendering /tags/ --&gt;
  {% for term in taxonomy.terms %}
    &lt;a href="{{ term.url }}"&gt;
      {{ term.name }} ({{ term.pages | size }})
    &lt;/a&gt;
  {% endfor %}
{% endif %}</alloy-code>
</wa-tab-panel>
<wa-tab-panel name="taxpage-go">
<alloy-code lang="html">{{ if .taxonomy.term }}
  &lt;!-- Rendering /tags/javascript/ --&gt;
  &lt;h1&gt;{{ .taxonomy.term }}&lt;/h1&gt;
  {{ range .taxonomy.pages }}
    &lt;a href="{{ .url }}"&gt;{{ .title }}&lt;/a&gt;
  {{ end }}
{{ else }}
  &lt;!-- Rendering /tags/ --&gt;
  {{ range .taxonomy.terms }}
    &lt;a href="{{ .url }}"&gt;
      {{ .name }} ({{ size .pages }})
    &lt;/a&gt;
  {{ end }}
{{ end }}</alloy-code>
</wa-tab-panel>
</wa-tab-group>
{% endraw %}

### Layout Lookup Order

For a taxonomy named `tags` with the default layout:

1. `layouts/taxonomies/tags.liquid`
2. `layouts/tags.liquid`

For a taxonomy with `layout: "term"`:

1. `layouts/taxonomies/term.liquid`
2. `layouts/term.liquid`

If no layout is found, the build fails:

```
[alloy] ERROR No layout found for taxonomy "tags"
        Expected: layouts/taxonomies/tags.liquid or layouts/tags.liquid
        Build aborted.
```

## Collection-Only Taxonomies (No Pages)

Set `render: false` to build taxonomy data without generating pages:

```yaml
taxonomies:
  tags:
    render: false
```

The data is still available as `taxonomies.tags.*` in templates. This is useful for organizing content into navigation sections without needing browsable taxonomy pages:

{% raw %}
<wa-tab-group>
<wa-tab slot="nav" panel="taxnav-liquid" active>Liquid</wa-tab>
<wa-tab slot="nav" panel="taxnav-go">Go templates</wa-tab>

<wa-tab-panel name="taxnav-liquid" active>
<alloy-code lang="liquid">&lt;!-- Use tags for navigation without dedicated tag pages --&gt;
{% assign nav_items = taxonomies.tags.foundations | sort: "order" %}
{% for page in nav_items %}
  &lt;a href="{{ page.url }}"&gt;{{ page.title }}&lt;/a&gt;
{% endfor %}</alloy-code>
</wa-tab-panel>
<wa-tab-panel name="taxnav-go">
<alloy-code lang="html">&lt;!-- Use tags for navigation without dedicated tag pages --&gt;
{{ $nav_items := sort .taxonomies.tags.foundations "order" }}
{{ range $nav_items }}
  &lt;a href="{{ .url }}"&gt;{{ .title }}&lt;/a&gt;
{{ end }}</alloy-code>
</wa-tab-panel>
</wa-tab-group>
{% endraw %}

When `render: false`, duplicate term slugs are not an error since no output pages are generated.

## Using Taxonomies in Templates

### Accessing All Terms

{% raw %}
<wa-tab-group>
<wa-tab slot="nav" panel="taxterms-liquid" active>Liquid</wa-tab>
<wa-tab slot="nav" panel="taxterms-go">Go templates</wa-tab>

<wa-tab-panel name="taxterms-liquid" active>
<alloy-code lang="liquid">&lt;!-- List all tags with page counts --&gt;
&lt;ul&gt;
  {% for term in taxonomy.terms %}
    &lt;li&gt;
      &lt;a href="{{ term.url }}"&gt;{{ term.name }}&lt;/a&gt;
      &lt;span&gt;({{ term.pages | size }})&lt;/span&gt;
    &lt;/li&gt;
  {% endfor %}
&lt;/ul&gt;</alloy-code>
</wa-tab-panel>
<wa-tab-panel name="taxterms-go">
<alloy-code lang="html">&lt;!-- List all tags with page counts --&gt;
&lt;ul&gt;
  {{ range .taxonomy.terms }}
    &lt;li&gt;
      &lt;a href="{{ .url }}"&gt;{{ .name }}&lt;/a&gt;
      &lt;span&gt;({{ size .pages }})&lt;/span&gt;
    &lt;/li&gt;
  {{ end }}
&lt;/ul&gt;</alloy-code>
</wa-tab-panel>
</wa-tab-group>
{% endraw %}

### Accessing a Specific Term

{% raw %}
<wa-tab-group>
<wa-tab slot="nav" panel="taxspecific-liquid" active>Liquid</wa-tab>
<wa-tab slot="nav" panel="taxspecific-go">Go templates</wa-tab>

<wa-tab-panel name="taxspecific-liquid" active>
<alloy-code lang="liquid">&lt;!-- Show all pages tagged "javascript" --&gt;
{% for post in taxonomies.tags.javascript %}
  &lt;article&gt;
    &lt;h3&gt;&lt;a href="{{ post.url }}"&gt;{{ post.title }}&lt;/a&gt;&lt;/h3&gt;
  &lt;/article&gt;
{% endfor %}</alloy-code>
</wa-tab-panel>
<wa-tab-panel name="taxspecific-go">
<alloy-code lang="html">&lt;!-- Show all pages tagged "javascript" --&gt;
{{ range .taxonomies.tags.javascript }}
  &lt;article&gt;
    &lt;h3&gt;&lt;a href="{{ .url }}"&gt;{{ .title }}&lt;/a&gt;&lt;/h3&gt;
  &lt;/article&gt;
{{ end }}</alloy-code>
</wa-tab-panel>
</wa-tab-group>
{% endraw %}

### Sorting Taxonomy Collections

{% raw %}
<wa-tab-group>
<wa-tab slot="nav" panel="taxsort-liquid" active>Liquid</wa-tab>
<wa-tab slot="nav" panel="taxsort-go">Go templates</wa-tab>

<wa-tab-panel name="taxsort-liquid" active>
<alloy-code lang="liquid">{% assign sorted = taxonomies.tags.foundations | sort: "order" %}
{% for page in sorted %}
  &lt;a href="{{ page.url }}"&gt;{{ page.title }}&lt;/a&gt;
{% endfor %}</alloy-code>
</wa-tab-panel>
<wa-tab-panel name="taxsort-go">
<alloy-code lang="html">{{ $sorted := sort .taxonomies.tags.foundations "order" }}
{{ range $sorted }}
  &lt;a href="{{ .url }}"&gt;{{ .title }}&lt;/a&gt;
{{ end }}</alloy-code>
</wa-tab-panel>
</wa-tab-group>
{% endraw %}

## Applying Tags via Data Cascade

Tags can be applied to all pages in a directory using `_data.yaml`, avoiding repetition in every file:

```yaml
# content/tutorials/_data.yaml
categories: ["tutorials"]
tags: ["learning"]
```

Every page under `content/tutorials/` inherits these taxonomy values. Individual pages can override with their own front matter (front matter always wins over cascade data).

## Custom Taxonomies

You can create any taxonomy beyond tags and categories:

```yaml
# alloy.config.yaml
taxonomies:
  tags:
  categories:
  series:
    permalink: "/series/:slug/"
    layout: "series"
  authors:
    permalink: "/team/:slug/"
    layout: "author"
```

```yaml
# content/blog/my-post.md
---
title: "Part 3: Advanced Patterns"
series: ["web-components-guide"]
authors: ["alice"]
---
```

This generates `/series/web-components-guide/` and `/team/alice/` pages listing all associated content.

## Undeclared Keys Are Ignored

If a post has `mood: ["happy"]` in front matter but `mood` is not in the `taxonomies` config, no collection is created. This prevents noisy collections from arbitrary front matter arrays (image lists, related links, etc.).

## Taxonomy Namespace

Taxonomies live under their own top-level template variable, `taxonomies`, separate from section collections:

{% raw %}
<wa-tab-group>
<wa-tab slot="nav" panel="taxns-liquid" active>Liquid</wa-tab>
<wa-tab slot="nav" panel="taxns-go">Go templates</wa-tab>

<wa-tab-panel name="taxns-liquid" active>
<alloy-code lang="liquid">{{ collections.blog }}            &lt;!-- section collection --&gt;
{{ taxonomies.tags.javascript }}  &lt;!-- taxonomy collection --&gt;</alloy-code>
</wa-tab-panel>
<wa-tab-panel name="taxns-go">
<alloy-code lang="html">{{ .collections.blog }}            &lt;!-- section collection --&gt;
{{ .taxonomies.tags.javascript }}  &lt;!-- taxonomy collection --&gt;</alloy-code>
</wa-tab-panel>
</wa-tab-group>
{% endraw %}

This prevents collisions between section names and taxonomy names.

## Related

- [Collections](/collections/) -- directory-based section collections
- [Data Cascade](/content/) -- how `_data.yaml` values cascade into taxonomy terms
- [Lifecycle Events](/hooks/) -- the `onPagesReady` hook fires before taxonomy collection
