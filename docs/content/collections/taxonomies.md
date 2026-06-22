---
layout: doc
title: Taxonomies
nav_weight: 20
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

```liquid
{% if taxonomy.term %}
  <!-- Rendering /tags/javascript/ -->
  <h1>{{ taxonomy.term }}</h1>
  {% for post in taxonomy.pages %}
    <a href="{{ post.url }}">{{ post.title }}</a>
  {% endfor %}
{% else %}
  <!-- Rendering /tags/ -->
  {% for term in taxonomy.terms %}
    <a href="{{ term.url }}">
      {{ term.name }} ({{ term.pages | size }})
    </a>
  {% endfor %}
{% endif %}
```

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

```liquid
<!-- Use tags for navigation without dedicated tag pages -->
{% assign nav_items = taxonomies.tags.foundations | sort: "order" %}
{% for page in nav_items %}
  <a href="{{ page.url }}">{{ page.title }}</a>
{% endfor %}
```

When `render: false`, duplicate term slugs are not an error since no output pages are generated.

## Using Taxonomies in Templates

### Accessing All Terms

```liquid
<!-- List all tags with page counts -->
<ul>
  {% for term in taxonomy.terms %}
    <li>
      <a href="{{ term.url }}">{{ term.name }}</a>
      <span>({{ term.pages | size }})</span>
    </li>
  {% endfor %}
</ul>
```

### Accessing a Specific Term

```liquid
<!-- Show all pages tagged "javascript" -->
{% for post in taxonomies.tags.javascript %}
  <article>
    <h3><a href="{{ post.url }}">{{ post.title }}</a></h3>
  </article>
{% endfor %}
```

### Sorting Taxonomy Collections

```liquid
{% assign sorted = taxonomies.tags.foundations | sort: "order" %}
{% for page in sorted %}
  <a href="{{ page.url }}">{{ page.title }}</a>
{% endfor %}
```

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

```liquid
{{ collections.blog }}            <!-- section collection -->
{{ taxonomies.tags.javascript }}  <!-- taxonomy collection -->
```

This prevents collisions between section names and taxonomy names.

## Related

- [Collections](/collections/) -- directory-based section collections
- [Data Cascade](/content/) -- how `_data.yaml` values cascade into taxonomy terms
- [Lifecycle Events](/hooks/) -- the `onPagesReady` hook fires before taxonomy collection
