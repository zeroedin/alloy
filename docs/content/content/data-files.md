---
layout: doc
title: Data Files
nav_weight: 70
description: "Load YAML, JSON, TOML, or CSV files from the data/ directory and access them globally in templates via site.data."
---

Data files provide structured data that is available to every template on your site. Place YAML, JSON, TOML, or CSV files in the `data/` directory and access them through `site.data.*`.

```yaml
# data/navigation.yaml
main:
  - label: "Home"
    url: "/"
  - label: "Blog"
    url: "/blog/"
  - label: "About"
    url: "/about/"
```

{% raw %}
```liquid
<nav>
  {% for item in site.data.navigation.main %}
    <a href="{{ item.url }}">{{ item.label }}</a>
  {% endfor %}
</nav>
```
{% endraw %}

## Supported formats

| Extension | Parser | Result type |
|---|---|---|
| `.yaml`, `.yml` | YAML | `map[string]any` |
| `.json` | JSON | Map (key order preserved; see [Key order](#key-order)) |
| `.toml` | TOML | `map[string]any` |
| `.csv` | CSV | Array of maps (header row = keys) |

Each file is keyed by its filename without the extension. `data/team.yaml` becomes `site.data.team`, `data/products.json` becomes `site.data.products`.

## Directory structure

Subdirectories create nested namespaces. `data/nav/main.yaml` becomes `site.data.nav.main`. Nesting depth is unlimited.

```text
data/
├── navigation.yaml        # site.data.navigation
├── team.yaml              # site.data.team
├── products.json          # site.data.products
├── authors.csv            # site.data.authors
├── nav/
│   └── main.yaml          # site.data.nav.main
└── api/
    └── v2/
        └── endpoints.yaml # site.data.api.v2.endpoints
```

Empty subdirectories are silently skipped — they produce no key in the namespace.

A file and a non-empty directory sharing the same stem (e.g., `nav.yaml` alongside a `nav/` directory containing data files) produces a build error:

```text
[alloy] ERROR data file stem conflict: "nav.yaml" and directory "nav/" both produce key "nav"
```

### Accessing nested data

{% raw %}
```liquid
{% for item in site.data.nav.main.items %}
  <a href="{{ item.url }}">{{ item.label }}</a>
{% endfor %}
```
{% endraw %}

## Key order

**If order matters, use a list.** A list iterates in file order in both engines, with no special handling:

```json
{
  "sections": [
    { "id": "intro", "title": "Introduction" },
    { "id": "setup", "title": "Setup" },
    { "id": "usage", "title": "Usage" }
  ]
}
```

```liquid
{% raw %}{% for section in site.data.nav.sections %}
  <h2>{{ section.title }}</h2>
{% endfor %}{% endraw %}
```

This is the recommendation for navigation menus and anything else where the order you wrote is the order you want. It works the same in Liquid and Go templates, and it is unaffected by everything below.

### What each engine does with map keys

Map key order depends on both the file format and the engine:

| Data | Liquid | Go templates |
| --- | --- | --- |
| JSON objects, plugin return values | file order | sorted by key |
| YAML, TOML | sorted by key | sorted by key |
| Lists (any format) | file order | file order |

JSON is loaded into an ordered map, which keeps the order you wrote. Liquid reads that map directly, so it can show file order. Go templates cannot: dot access like `{{ .site.data.sections.intro.title }}` requires a plain Go map, and a Go map has nowhere to store order — so keys come out sorted alphabetically, consistently on every build.

YAML and TOML discard key order as they load, so **neither engine can show file order for them**. Both sort instead.

If you need a specific order that is not alphabetical and not the file order, add a `weight` field and sort in the template — or, better, use a list.

### Iterating maps

The iteration syntax differs between template engines:

{% raw %}
<wa-tab-group>
<wa-tab slot="nav" panel="ordered-liquid" active>Liquid</wa-tab>
<wa-tab slot="nav" panel="ordered-go">Go templates</wa-tab>

<wa-tab-panel name="ordered-liquid" active>

In Liquid, `{% for %}` over a map yields `[key, value]` pairs. Access them by index:

<alloy-code language="liquid">{% for pair in site.data.sections %}
  &lt;h2&gt;{{ pair[0] }}&lt;/h2&gt;  &lt;!-- key: "intro", "setup", "usage" --&gt;
  &lt;p&gt;{{ pair[1].title }}&lt;/p&gt;
{% endfor %}</alloy-code>

For JSON data these come out in file order. Dot access works for individual keys: `{{ site.data.sections.intro.title }}`

</wa-tab-panel>
<wa-tab-panel name="ordered-go">

Dot access works directly, at any depth, including through arrays:

<alloy-code language="html">{{ .site.data.sections.intro.title }}
{{ .site.data.tokens.color.brand }}
{{ (index .site.data.sections.items 0).name }}</alloy-code>

`{{ range }}` works directly too, yielding keys in sorted order:

<alloy-code language="html">{{ range $key, $value := .site.data.sections }}
  &lt;h2&gt;{{ $key }}&lt;/h2&gt;
  &lt;p&gt;{{ $value.title }}&lt;/p&gt;
{{ end }}</alloy-code>

The `orange` helper still works and gives the same sorted order as `Key`/`Value` pairs:

<alloy-code language="html">{{ range orange .site.data.sections }}
  &lt;h2&gt;{{ .Key }}&lt;/h2&gt;
  &lt;p&gt;{{ .Value.title }}&lt;/p&gt;
{{ end }}</alloy-code>

`oget` also still works, but you rarely need it now: `{{ oget .site.data.sections "intro" }}` is the same as `{{ .site.data.sections.intro }}`. Reach for `index` when a key is not a valid template identifier — one containing a hyphen, say: `{{ index .site.data.tokens "color-brand" }}`

</wa-tab-panel>
</wa-tab-group>
{% endraw %}

## CSV files

CSV files are parsed with the first row as headers. Each subsequent row becomes a map keyed by the header values:

```csv
name,role,github
Alice,Engineering Lead,alice
Bob,Designer,bob-designs
Carol,PM,carol-pm
```

Access in templates:

{% raw %}
```liquid
{% for person in site.data.authors %}
  <p>{{ person.name }} -- {{ person.role }}</p>
{% endfor %}
```
{% endraw %}

## Name collision detection

Data files are keyed by stem name (filename without extension). If two files share a stem, the build fails:

```
[alloy] ERROR Data file conflict in data/:
        "team" is claimed by:
          1. team.csv
          2. team.yaml
        Resolve by renaming one file.
        Build aborted.
```

No silent overwrites, no priority system. Rename one file to resolve the collision.

## External data files

Files outside the `data/` directory can be mapped into the data namespace via config:

```yaml
# alloy.config.yaml
data:
  files:
    cem: "../custom-elements.json"
    tokens: "node_modules/@rhds/tokens/json/rhds.tokens.json"
```

Each key becomes a `site.data.*` entry. Paths are resolved relative to the project root:

{% raw %}
```liquid
<p>Schema version: {{ site.data.cem.schemaVersion }}</p>

{% for token in site.data.tokens.color %}
  <div style="background: {{ token.value }}">{{ token.name }}</div>
{% endfor %}
```
{% endraw %}

External data files support YAML, JSON, and TOML — the same formats as `data/` directory files except CSV. Pointing an external file at a `.csv` produces a build error. Use `data/` directory placement for CSV files.

External files share the same `site.data.*` namespace. Moving a YAML, JSON, or TOML file between `data/` and external config does not require template changes when the external mapping preserves the same key.

### Collision handling

If an external file key matches a `data/` directory file stem (e.g., `cem` key in config and `data/cem.json` on disk), the build fails with the same collision error. Choose external keys that do not conflict with filenames in `data/`.

External file not found is a build error -- not a warning, not silently skipped.

## External data sources

Alloy can fetch data from REST APIs and GraphQL endpoints at build time. Fetched data is injected into `site.data.*`, making it indistinguishable from local files in templates.

```yaml
# alloy.config.yaml
sources:
  posts:
    type: "rest"
    url: "https://api.example.com/posts.json"
    cache: 3600
    as: "posts"

  products:
    type: "graphql"
    endpoint: "https://api.example.com/graphql"
    query: |
      { products { id, name, price, slug } }
    cache: 1800
    as: "products"
```

Access fetched data the same way as local files:

{% raw %}
```liquid
{% for post in site.data.posts %}
  <h2><a href="/blog/{{ post.slug }}/">{{ post.title }}</a></h2>
{% endfor %}
```
{% endraw %}

### Built-in types vs plugin sources

Built-in `rest` and `graphql` types are single-request fetchers. They send one HTTP request and parse the response. For anything more complex, use `type: "plugin"` — see [Data Source Plugins](/plugins/node/#data-source-plugins).

| Capability | `rest` / `graphql` | `plugin` |
|---|---|---|
| Single unauthenticated GET | yes | yes |
| Authentication headers | no | yes |
| Pagination | no | yes |
| Custom HTTP methods | no | yes |
| Database access | no | yes |
| Multi-endpoint aggregation | no | yes |
| Retry / error handling | no | yes |
| Environment variables | no | yes |
| Requires Node.js | no | yes |
| Built-in caching | yes | yes |

### Caching

All fetched data is cached to `.alloy/fetch-cache/` on disk. The `cache` value sets the TTL in seconds. Cached data survives process restarts. If the TTL has not expired, the cached data is used without fetching.

### Combined with virtual pages

Fetched data feeds directly into [pagination](/content/pagination/) for page generation:

```yaml
# content/products.md
---
pagination:
  data: site.data.products
  as: product
permalink: "/products/{{ product.slug }}/"
---
<h1>{{ product.name }}</h1>
<p>{{ product.price }}</p>
```

One template plus an external data source generates pages at build time with no individual content files.

## Data in the cascade

Data files sit at the bottom of the [Data Cascade](/content/data-cascade/). Global data provides site-wide defaults that directory data (`_data.yaml`) and front matter can override.

```
1. Global data       ← data files (lowest priority)
2. Directory data    ← _data.yaml
3. Front matter      ← per-page (highest priority)
```

## Custom data directory

Override the default `data/` path in config:

```yaml
# alloy.config.yaml
structure:
  data: "./shared/data/"
```
