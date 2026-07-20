---
layout: doc
title: Collections
nav_weight: 10
description: "How Alloy automatically groups content into collections from your directory structure, with no manual registration required."
---

Collections group related content pages so templates can iterate, sort, and filter them. Alloy creates collections automatically from your content structure -- no manual registration required.

```yaml
# content/blog/_data.yaml
permalink: "/:year/:month/:slug/"
```

Any section whose `_data.yaml` permalink pattern contains date tokens (`:year`, `:month`, `:day`) becomes a collection. The pages under `content/blog/` are now available in templates as `collections.blog`.

## Directory-Based Collections

Collections follow from your content directory structure. A directory with a date-based permalink pattern in its `_data.yaml` collects all child pages:

```
content/
  blog/
    _data.yaml          # permalink: "/:year/:month/:slug/"
    first-post.md       # -> collections.blog
    second-post.md      # -> collections.blog
    2024/
      archived.md       # -> collections.blog (subdirectories inherit)
  docs/
    getting-started.md  # not a collection (no date-based permalink)
```

Regular directories without date-based permalink patterns are just pages, not collections. To group non-blog pages across sections, use [taxonomies](/collections/taxonomies/).

## Config-Declared Collections

A section without a date-based permalink can still become a collection by declaring it in the site config. Any section named under `collections:` collects its child pages, exactly like a date-based section:

```yaml
# alloy.config.yaml
collections:
  releases:
    sortBy: "date"
    order: "desc"
```

```
content/
  releases/
    index.md        # section index (not a collection member)
    v1.0.0.md       # -> collections.releases
    v1.1.0.md       # -> collections.releases
```

This is how a release notes section, a changelog, or a project portfolio becomes iterable as `collections.releases` without date tokens in its URLs.

In both kinds of collections, the section's own `index.md` is a container, not a member -- it does not appear in the collection. Page bundles (`releases/v1.0.0/index.md`) are members.

## Iterating Collections

Loop through a collection in your templates to build index pages, feeds, or navigation:

{% raw %}
<wa-tab-group>
<wa-tab slot="nav" panel="iterate-liquid" active>Liquid</wa-tab>
<wa-tab slot="nav" panel="iterate-go">Go templates</wa-tab>

<wa-tab-panel name="iterate-liquid" active>
<alloy-code language="liquid">{% for post in collections.blog %}
  &lt;article&gt;
    &lt;h2&gt;&lt;a href="{{ post.url }}"&gt;{{ post.title }}&lt;/a&gt;&lt;/h2&gt;
    &lt;time&gt;{{ post.date | date: "%B %d, %Y" }}&lt;/time&gt;
    &lt;p&gt;{{ post.summary }}&lt;/p&gt;
  &lt;/article&gt;
{% endfor %}</alloy-code>
</wa-tab-panel>
<wa-tab-panel name="iterate-go">
<alloy-code language="html">{{ range .collections.blog }}
  &lt;article&gt;
    &lt;h2&gt;&lt;a href="{{ .url }}"&gt;{{ .title }}&lt;/a&gt;&lt;/h2&gt;
    &lt;time&gt;{{ date .date "%B %d, %Y" }}&lt;/time&gt;
    &lt;p&gt;{{ .summary }}&lt;/p&gt;
  &lt;/article&gt;
{{ end }}</alloy-code>
</wa-tab-panel>
</wa-tab-group>
{% endraw %}

Each item in the collection is a page object with all the usual properties: `url`, `title`, `date`, `summary`, and any custom front matter fields.

## Default Sort Order

Collections sort by **date descending** (newest first). When two pages share the same date, the full datetime is compared. If the datetime is identical or no time component is provided, filename alphabetical ascending is the tiebreaker. Pages without a date sort after all dated pages, ordered by filename.

Sort is deterministic across builds.

## Custom Sort Order

Override the default sort for a collection in your site config:

```yaml
# alloy.config.yaml
collections:
  blog:
    sortBy: "date"
    order: "desc"
```

You can sort by any front matter field. For a portfolio sorted by a custom `order` field:

```yaml
collections:
  projects:
    sortBy: "order"
    order: "asc"
```

## Sorting in Templates

Sort inline using built-in array filters:

{% raw %}
<wa-tab-group>
<wa-tab slot="nav" panel="sort-liquid" active>Liquid</wa-tab>
<wa-tab slot="nav" panel="sort-go">Go templates</wa-tab>

<wa-tab-panel name="sort-liquid" active>
<alloy-code language="liquid">{% assign alphabetical = collections.blog | sort: "title" %}
{% assign by_author = collections.blog | sort: "author" %}
{% assign recent = collections.blog | sort: "date" | reverse %}</alloy-code>
</wa-tab-panel>
<wa-tab-panel name="sort-go">
<alloy-code language="html">{{ $alphabetical := sort .collections.blog "title" }}
{{ $by_author := sort .collections.blog "author" }}
{{ $recent := reverse (sort .collections.blog "date") }}</alloy-code>
</wa-tab-panel>
</wa-tab-group>
{% endraw %}

The `sort` filter is numeric-aware. Values like `order: 1, 2, 10, 20` sort correctly as numbers, not as strings (`1, 10, 2, 20`).

### Numeric Sort Rules

- `int` values (YAML `order: 10`) are compared as integers, including negatives
- `float64` with no fractional part (JSON `"order": 10.0`) are compared as integers
- String values containing only digits (`order: "10"`) are parsed and compared as integers
- Everything else falls back to string comparison
- Nil or missing values sort to the end

## Filtering Collections

Use the `where` filter to narrow a collection by any front matter field:

{% raw %}
<wa-tab-group>
<wa-tab slot="nav" panel="filter-liquid" active>Liquid</wa-tab>
<wa-tab slot="nav" panel="filter-go">Go templates</wa-tab>

<wa-tab-panel name="filter-liquid" active>
<alloy-code language="liquid">{% assign featured = collections.blog | where: "featured", true %}
{% for post in featured %}
  &lt;a href="{{ post.url }}"&gt;{{ post.title }}&lt;/a&gt;
{% endfor %}</alloy-code>
</wa-tab-panel>
<wa-tab-panel name="filter-go">
<alloy-code language="html">{{ $featured := where .collections.blog "featured" true }}
{{ range $featured }}
  &lt;a href="{{ .url }}"&gt;{{ .title }}&lt;/a&gt;
{{ end }}</alloy-code>
</wa-tab-panel>
</wa-tab-group>
{% endraw %}

Combine with other filters:

{% raw %}
<wa-tab-group>
<wa-tab slot="nav" panel="chain-liquid" active>Liquid</wa-tab>
<wa-tab slot="nav" panel="chain-go">Go templates</wa-tab>

<wa-tab-panel name="chain-liquid" active>
<alloy-code language="liquid">{% assign recent_tutorials = collections.blog
    | where: "category", "tutorial"
    | sort: "date"
    | reverse %}</alloy-code>
</wa-tab-panel>
<wa-tab-panel name="chain-go">
<alloy-code language="html">{{ $recent_tutorials := reverse (sort (where .collections.blog "category" "tutorial") "date") }}</alloy-code>
</wa-tab-panel>
</wa-tab-group>
{% endraw %}

## Collection Lifecycle

Collections interact with [content lifecycle](/content/) rules:

- **Drafts** (`draft: true`): excluded from collections in `alloy build` and `alloy serve`, included in `alloy dev`
- **Future publish dates**: excluded from collections everywhere until the date arrives
- **Expired content**: excluded from collections everywhere

Lifecycle filtering happens before pagination. A paginated list of 47 articles with 3 drafts produces pages based on 44 items in build mode, but may differ in dev mode where drafts are included.

## Accessing All Pages

All published pages are available as `site.pages` regardless of whether they belong to a collection:

{% raw %}
<wa-tab-group>
<wa-tab slot="nav" panel="allpages-liquid" active>Liquid</wa-tab>
<wa-tab slot="nav" panel="allpages-go">Go templates</wa-tab>

<wa-tab-panel name="allpages-liquid" active>
<alloy-code language="liquid">{% for page in site.pages %}
  &lt;a href="{{ page.url }}"&gt;{{ page.title }}&lt;/a&gt;
{% endfor %}</alloy-code>
</wa-tab-panel>
<wa-tab-panel name="allpages-go">
<alloy-code language="html">{{ range .site.pages }}
  &lt;a href="{{ .url }}"&gt;{{ .title }}&lt;/a&gt;
{{ end }}</alloy-code>
</wa-tab-panel>
</wa-tab-group>
{% endraw %}

## Related

- [Taxonomies](/collections/taxonomies/) -- cross-cutting groups via tags, categories, and custom keys
- [Lifecycle Events](/hooks/) -- hooks that fire during collection building
- [Data Cascade](/content/) -- how `_data.yaml` drives collection creation
