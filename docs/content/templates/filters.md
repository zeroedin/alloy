---
layout: doc
title: Filters
nav_weight: 30
description: "Reference for Alloy's 50+ built-in template filters covering strings, dates, arrays, URLs, math, and content processing."
---

Filters transform values in template expressions. Alloy ships with 50+ built-in filters covering strings, dates, arrays, URLs, math, and content processing. Filters are registered in both the Liquid and Go template engines at startup.

```liquid
{{ page.title | upcase }}
{{ page.date | date: "%B %d, %Y" }}
{{ collections.blog | sort: "title" | first }}
```

Filters chain left to right. Each filter receives the output of the previous expression as its input.

## String filters

| Filter | Description | Example |
|---|---|---|
| `upcase` | Convert to uppercase | `{{ "hello" | upcase }}` --> `HELLO` |
| `downcase` | Convert to lowercase | `{{ "HELLO" | downcase }}` --> `hello` |
| `capitalize` | Capitalize first character | `{{ "hello world" | capitalize }}` --> `Hello world` |
| `slugify` | URL-safe slug | `{{ "My Blog Post!" | slugify }}` --> `my-blog-post` |
| `truncate` | Truncate to character count | `{{ page.summary | truncate: 100 }}` |
| `truncatewords` | Truncate to word count | `{{ page.summary | truncatewords: 20 }}` |
| `strip_html` | Remove all HTML tags | `{{ page.summary | strip_html }}` |
| `escape` | HTML-escape special characters | `{{ page.title | escape }}` |
| `replace` | Replace all occurrences | `{{ page.title | replace: " ", "-" }}` |
| `replace_first` | Replace first occurrence | `{{ "aabbcc" | replace_first: "a", "x" }}` --> `xabbcc` |
| `split` | Split string into array | `{{ "a,b,c" | split: "," }}` |
| `join` | Join array into string | `{{ page.tags | join: ", " }}` |
| `strip` | Remove leading/trailing whitespace | `{{ " hello " | strip }}` --> `hello` |
| `append` | Append a string | `{{ page.slug | append: ".html" }}` |
| `prepend` | Prepend a string | `{{ page.slug | prepend: "/blog/" }}` |
| `newline_to_br` | Convert newlines to `<br>` | `{{ page.bio | newline_to_br }}` |
| `contains` | Check if string contains substring | `{{ page.title \| contains: "Guide" }}` → `true` |

### slugify

The `slugify` filter converts any string to a URL-safe slug by lowercasing, replacing spaces with hyphens, and stripping non-alphanumeric characters:

```liquid
{{ "My First Blog Post!" | slugify }}
<!-- Output: my-first-blog-post -->

<a href="/tags/{{ tag | slugify }}/">{{ tag }}</a>
```

## Date filter

The `date` filter formats dates using strftime directives, powered by `lestrrat-go/strftime` for full POSIX compliance.

```liquid
{{ page.date | date: "%B %d, %Y" }}
<!-- Output: April 10, 2026 -->

{{ page.date | date: "%Y-%m-%d" }}
<!-- Output: 2026-04-10 -->

<time datetime="{{ page.date | date: '%Y-%m-%dT%H:%M:%S%z' }}">
  {{ page.date | date: "%A, %B %e, %Y" }}
</time>
```

Accepts `time.Time` objects or string input (ISO 8601, RFC 3339, `YYYY-MM-DD HH:MM:SS`, `YYYY-MM-DD`). Returns input unchanged when no format argument is provided.

**Common directives:**

| Directive | Output | Example |
|---|---|---|
| `%Y` | 4-digit year | `2026` |
| `%m` | 2-digit month | `04` |
| `%d` | 2-digit day | `10` |
| `%B` | Full month name | `April` |
| `%b` | Abbreviated month | `Apr` |
| `%A` | Full weekday name | `Friday` |
| `%H` | Hour (24h) | `14` |
| `%M` | Minute | `30` |
| `%S` | Second | `00` |
| `%z` | Timezone offset | `+0000` |

## Array filters

Array filters operate on collections, taxonomy groups, and any list data.

| Filter | Description | Example |
|---|---|---|
| `sort` | Sort by a key | `{{ collections.blog | sort: "title" }}` |
| `reverse` | Reverse order | `{{ collections.blog | reverse }}` |
| `first` | First item | `{{ collections.blog | first }}` |
| `last` | Last item | `{{ collections.blog | last }}` |
| `size` | Count items | `{{ collections.blog | size }}` |
| `map` | Extract a single field | `{{ collections.blog | map: "title" }}` |
| `uniq` | Remove duplicates | `{{ page.tags | uniq }}` |
| `compact` | Remove nil values | `{{ pages | compact }}` |
| `concat` | Concatenate two arrays | `{{ collections.blog | concat: collections.docs }}` |

### where

The `where` filter selects items from an array where a field matches a value:

```liquid
{% assign featured = collections.blog | where: "featured", true %}
{% for post in featured %}
  <h2><a href="{{ post.url }}">{{ post.title }}</a></h2>
{% endfor %}

{% assign js_posts = collections.blog | where: "tags", "javascript" %}
```

### sort

The `sort` filter is numeric-aware. When values are whole numbers, they are compared numerically rather than as strings:

```liquid
{% assign sorted_nav = taxonomies.tags.foundations | sort: "order" %}
{% for page in sorted_nav %}
  <a href="{{ page.url }}">{{ page.title }}</a>
{% endfor %}
```

With front matter `order: 1`, `order: 2`, `order: 10`, `order: 20`, the sort produces `1, 2, 10, 20` -- not `1, 10, 2, 20` as a naive string sort would.

**Numeric detection rules:**
- Integer YAML values (`order: 10`, `order: -1`) are compared as integers
- Float values with no fractional part (`10.0`) are compared as integers
- String values containing only digits (`"10"`) are parsed and compared as integers
- Everything else falls back to string comparison
- Nil or missing values sort to the end

### group_by

The `group_by` filter groups items by a shared field value:

```liquid
{% assign by_year = collections.blog | group_by: "year" %}
{% for group in by_year %}
  <h2>{{ group.name }}</h2>
  {% for post in group.items %}
    <a href="{{ post.url }}">{{ post.title }}</a>
  {% endfor %}
{% endfor %}
```

### map

The `map` filter extracts a single field from every item in an array:

```liquid
{{ collections.blog | map: "title" | join: ", " }}
<!-- Output: First Post, Second Post, Third Post -->
```

## Set operation filters

| Filter | Description | Example |
|---|---|---|
| `intersect` | Items in both arrays | `{{ page.tags | intersect: featured_tags }}` |
| `union` | Items in either array (deduplicated) | `{{ page.tags | union: default_tags }}` |
| `complement` | Items in first array but not second | `{{ all_tags | complement: hidden_tags }}` |

## URL filters

| Filter | Description | Example |
|---|---|---|
| `url` | Resolve path relative to `baseURL` | `{{ "css/main.css" | url }}` |
| `absolute_url` | Full absolute URL with domain | `{{ page.url | absolute_url }}` |
| `url_encode` | Percent-encode a string | `{{ page.title | url_encode }}` |
| `url_decode` | Decode a percent-encoded string | `{{ encoded | url_decode }}` |

```liquid
<link rel="stylesheet" href="{{ 'css/main.css' | url }}">
<link rel="canonical" href="{{ page.url | absolute_url }}">
```

## Math filters

| Filter | Description | Example |
|---|---|---|
| `plus` | Add | `{{ 5 | plus: 3 }}` --> `8` |
| `minus` | Subtract | `{{ 10 | minus: 3 }}` --> `7` |
| `times` | Multiply | `{{ 4 | times: 3 }}` --> `12` |
| `divided_by` | Divide | `{{ 10 | divided_by: 3 }}` --> `3` |
| `modulo` | Remainder | `{{ 10 | modulo: 3 }}` --> `1` |
| `ceil` | Round up | `{{ 4.2 | ceil }}` --> `5` |
| `floor` | Round down | `{{ 4.8 | floor }}` --> `4` |
| `round` | Round to nearest | `{{ 4.5 | round }}` --> `5` |
| `abs` | Absolute value | `{{ -5 | abs }}` --> `5` |

## Content filters

### markdownify

Renders a Markdown string to HTML using the same goldmark configuration as the main content renderer:

```liquid
{{ page.description | markdownify }}
```

This is useful for rendering Markdown in front matter fields or data file values. The filter uses the site's `content.markdown` settings (unsafe mode, typographer, heading IDs) but does not run template tag protection -- it processes already-rendered values.

```yaml
# data/features.yaml
- title: Core Engine
  description: "Built on **Go** for speed and `goldmark` for Markdown."
```

```liquid
{% for feature in site.data.features %}
  <div class="feature">
    <h3>{{ feature.title }}</h3>
    {{ feature.description | markdownify }}
  </div>
{% endfor %}
```

### safeHTML

Bypasses auto-escaping for trusted HTML content. Relevant primarily for the Go template engine:

```liquid
{{ page.embed_code | safeHTML }}
```

## Regex filters

| Filter | Description | Example |
|---|---|---|
| `findRE` | Find regex matches | `{{ page.content | findRE: "<h2>(.*?)</h2>" }}` |
| `replaceRE` | Regex replace | `{{ page.title | replaceRE: "[^a-z]", "" }}` |

## Data filters

| Filter | Description | Example |
|---|---|---|
| `json` | Serialize value to JSON | `{{ site.data.config | json }}` |
| `default` | Fallback value if nil or empty | `{{ page.author | default: "Anonymous" }}` |

```liquid
<script type="application/ld+json">
  {{ page | json }}
</script>

<p>By {{ page.author | default: "Staff Writer" }}</p>
```

## Asset filters

### fingerprint

Content-hash fingerprinting for cache busting. Computes a SHA-256 hash of the file contents and appends it to the filename:

```liquid
<link rel="stylesheet" href="{{ 'css/main.css' | fingerprint }}">
<!-- Output: /css/main.abc123def456.css -->
```

The filter resolves paths against source directories in order: `static/` -> `assets/` -> `content/` (for co-located assets).

### cachebust

Appends a content hash as a query parameter instead of rewriting the filename:

```liquid
<link rel="stylesheet" href="{{ 'css/main.css' | cachebust }}">
<!-- Output: /css/main.css?h=abc123def456 -->
```

File not found degrades gracefully -- returns the path without a hash.

### get_hash

Returns the raw hash digest of a file:

```liquid
{{ 'css/main.css' | get_hash }}
<!-- Output: base64-encoded SHA-256 digest -->
```

## Feed-related filters

These filters are helpful when building RSS or Atom feed templates:

```liquid
<!-- layouts/feed.xml.liquid -->
{% for post in collections.blog %}
<item>
  <title>{{ post.title | xml_escape }}</title>
  <pubDate>{{ post.date | rfc822_date }}</pubDate>
  <link>{{ post.url | absolute_url }}</link>
</item>
{% endfor %}
```

## Custom filters via plugins

Register custom filters from a [plugin](/plugins/). Plugin filters work in layouts, content templates, and partials.

**JS plugin (Tier 2 -- in-process):**

```javascript
// plugins/word-count.js
export default function(alloy) {
  alloy.filter("wordCount", (content) => {
    return content.split(/\s+/).filter(w => w.length > 0).length;
  });
}
```

```liquid
<p>{{ page.content | wordCount }} words</p>
```

**WASM plugin (Tier 2 -- compiled, fastest):**

Compile a filter to `.wasm` from Rust, TinyGo, or AssemblyScript for maximum performance on filters called thousands of times per build.

**Node plugin (Tier 3 -- full Node.js access):**

```javascript
// plugins/reading-time.js
export const runtime = "node";
export default function(alloy) {
  alloy.filter("readingTime", (content) => {
    const words = content.split(/\s+/).length;
    return Math.ceil(words / 200);
  });
}
```

If two plugins register the same filter name, the last one loaded wins. Alloy logs a warning:

```
[alloy] WARN Filter "slugify" registered by plugins/custom-slugify.wasm
        overwrites built-in filter "slugify"
```

Load order: built-in Go filters first, then Tier 2 plugins (`.js` and `.wasm` alphabetically), then Tier 3 Node plugins.
