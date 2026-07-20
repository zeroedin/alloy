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

## Filter syntax by engine

All built-in filters are registered in both engines, but the calling syntax differs. Liquid uses pipe-and-colon syntax; Go templates call filters as functions with the input as the first argument.

{% raw %}
<wa-tab-group>
<wa-tab slot="nav" panel="syntax-liquid" active>Liquid</wa-tab>
<wa-tab slot="nav" panel="syntax-go">Go templates</wa-tab>

<wa-tab-panel name="syntax-liquid" active>
<alloy-code language="liquid">{{ page.title | upcase }}
{{ page.summary | truncate: 100 }}
{{ page.date | date: "%B %d, %Y" }}
{{ collections.blog | sort: "title" | first }}</alloy-code>
</wa-tab-panel>
<wa-tab-panel name="syntax-go">
<alloy-code language="html">{{ upcase .page.title }}
{{ truncate .page.summary 100 }}
{{ date .page.date "%B %d, %Y" }}
{{ first (sort .collections.blog "title") }}</alloy-code>
</wa-tab-panel>
</wa-tab-group>
{% endraw %}

Go's pipe syntax works for filters that take no extra arguments (`{% raw %}{{ .page.title | upcase }}{% endraw %}`), because Go pipelines pass the piped value as the *last* argument. For filters that take arguments, use function-call syntax so the input stays in the first position.

The examples in the reference tables below use Liquid syntax.

## String filters

{% raw %}
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
| `newline_to_br` | Convert newlines to `&lt;br&gt;` | `{{ page.bio | newline_to_br }}` |
| `contains` | Check if string contains substring | `{{ page.title | contains: "Guide" }}` --> `true` |
{% endraw %}

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

{% raw %}
| Filter | Description | Example |
|---|---|---|
| `sort` | Sort by a key | `{{ collections.blog | sort: "title" }}` |
| `reverse` | Reverse order | `{{ collections.blog | reverse }}` |
| `first` | First item | `{{ collections.blog | first }}` |
| `last` | Last item | `{{ collections.blog | last }}` |
| `limit` | First N items | `{{ collections.blog | limit: 5 }}` |
| `size` | Count items | `{{ collections.blog | size }}` |
| `map` | Extract a single field | `{{ collections.blog | map: "title" }}` |
| `uniq` | Remove duplicates | `{{ page.tags | uniq }}` |
| `compact` | Remove nil values | `{{ pages | compact }}` |
| `concat` | Concatenate two arrays | `{{ collections.blog | concat: collections.docs }}` |
| `flatten` | Flatten nested arrays one level | `{{ nested_lists | flatten }}` |
{% endraw %}

### where

The `where` filter selects items from an array where a field matches a value:

```liquid
{% assign featured = collections.blog | where: "featured", true %}
{% for post in featured %}
  <h2><a href="{{ post.url }}">{{ post.title }}</a></h2>
{% endfor %}

{% assign js_posts = collections.blog | where: "tags", "javascript" %}
```

### limit

The `limit` filter returns the first N items from an array. It provides Go template parity with Liquid's `{% for ... limit: N %}` loop clause:

{% raw %}
<wa-tab-group>
<wa-tab slot="nav" panel="limit-liquid" active>Liquid</wa-tab>
<wa-tab slot="nav" panel="limit-go">Go templates</wa-tab>

<wa-tab-panel name="limit-liquid" active>
<alloy-code language="liquid">&lt;h2&gt;Recent posts&lt;/h2&gt;
{% assign recent = collections.blog | limit: 5 %}
{% for post in recent %}
  &lt;a href="{{ post.url }}"&gt;{{ post.title }}&lt;/a&gt;
{% endfor %}</alloy-code>

Liquid's `{% for ... limit: 5 %}` clause still works -- the `limit` filter is an alternative that's useful in `assign` chains or when composing filters:

<alloy-code language="liquid">{% assign top3 = collections.blog | sort: "date" | reverse | limit: 3 %}</alloy-code>
</wa-tab-panel>
<wa-tab-panel name="limit-go">
<alloy-code language="html">&lt;h2&gt;Recent posts&lt;/h2&gt;
{{ range limit .collections.blog 5 }}
  &lt;a href="{{ .url }}"&gt;{{ .title }}&lt;/a&gt;
{{ end }}</alloy-code>

Go templates also have a built-in `slice` function (from Go's standard library, not an Alloy filter) that supports both start and end indices:

<alloy-code language="html">&lt;!-- slice: native Go built-in, equivalent to limit --&gt;
{{ range slice .collections.blog 0 5 }}
  &lt;a href="{{ .url }}"&gt;{{ .title }}&lt;/a&gt;
{{ end }}

&lt;!-- slice with offset: items 6-10 --&gt;
{{ range slice .collections.blog 5 10 }}
  &lt;a href="{{ .url }}"&gt;{{ .title }}&lt;/a&gt;
{{ end }}</alloy-code>

`slice` is a Go template built-in (not an Alloy filter) and is not available in Liquid. Use `limit` for cross-engine compatibility.
</wa-tab-panel>
</wa-tab-group>
{% endraw %}

`limit` clamps to array bounds -- `limit: 100` on a 3-item array returns all 3 items. A negative or zero argument returns an empty array. Calling `limit` with no argument returns the array unchanged.

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

{% raw %}
| Filter | Description | Example |
|---|---|---|
| `intersect` | Items in both arrays | `{{ page.tags | intersect: featured_tags }}` |
| `union` | Items in either array (deduplicated) | `{{ page.tags | union: default_tags }}` |
| `complement` | Items in first array but not second | `{{ all_tags | complement: hidden_tags }}` |
{% endraw %}

## URL filters

{% raw %}
| Filter | Description | Example |
|---|---|---|
| `url` | Resolve path relative to baseURL path prefix | `{{ "css/main.css" | url }}` --> `/blog/css/main.css` |
| `absolute_url` | Prepend full baseURL to a path | `{{ page.url | absolute_url }}` --> `https://example.com/page/` |
| `url_encode` | Percent-encode a string | `{{ page.title | url_encode }}` |
| `url_decode` | Decode a percent-encoded string | `{{ encoded | url_decode }}` |
{% endraw %}

```liquid
<link rel="stylesheet" href="{{ 'css/main.css' | url }}">
<link rel="canonical" href="{{ page.url | absolute_url }}">
```

Both filters use the site's configured `baseURL` automatically. `url` prepends the path portion (e.g., `/blog` from `https://example.com/blog`). `absolute_url` prepends the full base URL. An explicit argument to `absolute_url` overrides the configured base. Inputs that already start with `http://` or `https://` pass through untouched.

## Math filters

{% raw %}
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
{% endraw %}

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

{% raw %}
| Filter | Description | Example |
|---|---|---|
| `findRE` | Find regex matches | `{{ page.content | findRE: "&lt;h2&gt;(.*?)&lt;/h2&gt;" }}` |
| `replaceRE` | Regex replace | `{{ page.title | replaceRE: "[^a-z]", "" }}` |
{% endraw %}

## Data filters

{% raw %}
| Filter | Description | Example |
|---|---|---|
| `json` | Serialize value to JSON | `{{ site.data.config | json }}` |
| `default` | Fallback value if nil or empty | `{{ page.author | default: "Anonymous" }}` |
{% endraw %}

```liquid
<script type="application/ld+json">
  {{ page | json }}
</script>

<p>By {{ page.author | default: "Staff Writer" }}</p>
```

## Asset filters

### cachebust

Content-based cache busting. Reads the file, computes a SHA-256 hash of its contents, and appends the hash as a query parameter:

```liquid
<link rel="stylesheet" href="{{ 'css/main.css' | cachebust }}">
<!-- Output: /css/main.css?h=abc123def456 -->
```

The filter resolves paths against source directories in order: `static/` -> `assets/` -> `content/` (for co-located assets). File not found degrades gracefully -- returns the path without a hash.

### get_hash

Returns the hash digest of a file's contents. Resolves paths the same way as `cachebust`:

```liquid
{{ 'css/main.css' | get_hash }}
<!-- Output: base64-encoded SHA-256 digest -->

{{ 'css/main.css' | get_hash: 384 }}
<!-- SHA-384 instead of SHA-256 (also accepts 512) -->

{{ 'css/main.css' | get_hash: 256, false }}
<!-- hex-encoded instead of base64 -->
```

Useful for Subresource Integrity attributes: `integrity="sha256-{{ 'js/app.js' | get_hash }}"`.

## Building feeds with filters

RSS and Atom feed templates combine `escape` (XML entities are a subset of HTML escaping), the `date` filter with an RFC 822 format string, and `absolute_url`:

```liquid
<!-- layouts/feed.xml.liquid -->
{% for post in collections.blog %}
<item>
  <title>{{ post.title | escape }}</title>
  <pubDate>{{ post.date | date: "%a, %d %b %Y %H:%M:%S %z" }}</pubDate>
  <link>{{ post.url | absolute_url }}</link>
</item>
{% endfor %}
```

See [Output Formats](/templates/output-formats/) for the full feed setup.

## Liquid-only standard filters

The Liquid engine is built on a full Liquid implementation, so the standard Shopify Liquid filters are also available even though they are not part of Alloy's built-in set:

`slice`, `sort_natural`, `at_least`, `at_most`, `remove`, `remove_first`, `remove_last`, `replace_last`, `lstrip`, `rstrip`, `strip_newlines`, `escape_once`, `reject`, `has`, `find`, `find_index`, `sum`, `base64_encode`, `base64_decode`, `base64_url_safe_encode`, `base64_url_safe_decode`

These are not registered in the Go template engine -- templates that use them are not portable across engines.

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
