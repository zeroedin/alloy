---
layout: doc
title: Data Files
---

Data files provide structured data available to every template. Place YAML, JSON, TOML, or CSV files in the `data/` directory and access them as `site.data.*`.

```yaml
# data/navigation.yaml
items:
  - label: Home
    url: /
  - label: Blog
    url: /blog/
  - label: About
    url: /about/
```

```liquid
<nav>
  {% for item in site.data.navigation.items %}
    <a href="{{ item.url }}">{{ item.label }}</a>
  {% endfor %}
</nav>
```

## Supported formats

| Extension | Format | Result |
|---|---|---|
| `.yaml`, `.yml` | YAML | Map |
| `.json` | JSON | Map (insertion order preserved) |
| `.toml` | TOML | Map |
| `.csv` | CSV | Array of maps (header row = keys) |

All formats parse into the same internal structure. The rest of the pipeline — data cascade, template context, plugin hooks — is format-agnostic.

## File naming and access

Data files are keyed by their stem name (filename without extension). A file at `data/team.yaml` is accessible as `site.data.team`:

```
data/
├── navigation.yaml     → site.data.navigation
├── team.json           → site.data.team
├── settings.toml       → site.data.settings
└── employees.csv       → site.data.employees
```

## Name collision errors

Data files are keyed by stem name, so `team.csv` and `team.yaml` both claim the key `"team"`. If two or more data files in the same directory share a stem name, the build fails:

```
[alloy] ERROR Data file conflict in data/:
        "team" is claimed by:
          1. team.csv
          2. team.yaml
        Resolve by renaming one file.
        Build aborted.
```

No silent overwrites, no priority system. Resolve collisions by renaming one file.

## JSON insertion order

JSON data preserves insertion order. Keys in a JSON object are accessible in the order they appear in the file, which matters when iterating over entries in templates.

## CSV data

CSV files are parsed as an array of maps. The first row provides the keys:

```csv
name,role,email
Alice,Engineering,alice@example.com
Bob,Design,bob@example.com
```

```liquid
{% for person in site.data.employees %}
  <p>{{ person.name }} — {{ person.role }}</p>
{% endfor %}
```

## External data files

Files outside the `data/` directory can be mapped into the data namespace via the `data.files` config:

```yaml
# alloy.config.yaml
data:
  files:
    cem: "../custom-elements.json"
    tokens: "node_modules/@rhds/tokens/json/rhds.tokens.json"
```

Each key becomes a `site.data.*` entry. Paths are resolved relative to the project root. The file is parsed by extension using the same parsers as `data/` directory files:

```liquid
<p>Schema version: {{ site.data.cem.schemaVersion }}</p>
```

External data files share the same namespace as `data/` directory files. If an external file key matches a `data/` directory file stem (e.g., `cem` key in config and `data/cem.json` on disk), the build fails with the same collision error. Choose keys that don't conflict with filenames in `data/`.

External file not found is a build error — not a warning, not silently skipped.

## External data sources

For data fetched over the network at build time, use the `sources:` config. Alloy supports three source types:

```yaml
# alloy.config.yaml
sources:
  # Built-in REST — single HTTP GET
  posts:
    type: "rest"
    url: "https://api.example.com/posts.json"
    cache: 3600
    as: "posts"

  # Built-in GraphQL — single query
  products:
    type: "graphql"
    endpoint: "https://api.example.com/graphql"
    query: |
      { products { id, name, price, slug } }
    cache: 1800
    as: "products"

  # Plugin — full control over fetching
  blog:
    type: "plugin"
    plugin: "cms-posts"
    cache: 3600
    as: "blog"
```

The `as` key determines the `site.data.*` path. Templates access external sources identically to local data files:

```liquid
{% for post in site.data.posts %}
  <h2>{{ post.title }}</h2>
{% endfor %}
```

**REST sources** parse the response based on Content-Type: JSON, XML, and CSV are supported. **GraphQL sources** automatically unwrap the `data` envelope so you get the clean payload. **Plugin sources** give a Node.js plugin full ownership of data acquisition — authentication, pagination, retries, and error handling.

## Caching

All source data (built-in and plugin) is cached to `.alloy/fetch-cache/` on disk. The `cache` value sets the TTL in seconds. Cached data survives process restarts and is used without fetching until the TTL expires.

In dev mode, caching is cache-first — file changes do not trigger refetches. Use `alloy dev --refetch` to bypass the cache TTL and fetch fresh data on startup.
