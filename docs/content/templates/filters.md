---
layout: doc
title: Filters
---

Filters transform values in template expressions. Chain them with the pipe (`|`) character:

```liquid
{{ page.title | upcase }}
{{ page.date | date: "%B %d, %Y" }}
{{ page.tags | sort | join: ", " }}
```

Filters process left to right. Each filter receives the output of the previous one, so `sort` runs first, then `join` formats the sorted result.

## String filters

| Filter | Description | Example |
|---|---|---|
| `upcase` | Uppercase | `{{ "hello" | upcase }}` &rarr; `HELLO` |
| `downcase` | Lowercase | `{{ "Hello" | downcase }}` &rarr; `hello` |
| `capitalize` | Capitalize first character | `{{ "hello world" | capitalize }}` &rarr; `Hello world` |
| `strip` | Remove leading/trailing whitespace | `{{ "  hi  " | strip }}` &rarr; `hi` |
| `truncate` | Truncate to length (default 50), appends `...` | `{{ page.summary | truncate: 100 }}` |
| `truncatewords` | Truncate to word count (default 15) | `{{ page.body | truncatewords: 25 }}` |
| `strip_html` | Remove all HTML tags | `{{ page.body | strip_html }}` |
| `escape` | HTML-escape special characters | `{{ page.title | escape }}` |
| `slugify` | Lowercase, replace non-alphanumeric with hyphens | `{{ "Hello World!" | slugify }}` &rarr; `hello-world` |
| `replace` | Replace all occurrences | `{{ "foo bar" | replace: "bar", "baz" }}` |
| `replace_first` | Replace first occurrence | `{{ "foo foo" | replace_first: "foo", "bar" }}` |
| `split` | Split string into array | `{{ "a,b,c" | split: "," }}` |
| `join` | Join array into string | `{{ tags | join: ", " }}` |
| `append` | Append string | `{{ "/about" | append: "/" }}` |
| `prepend` | Prepend string | `{{ page.url | prepend: site.baseURL }}` |
| `newline_to_br` | Convert newlines to `<br>` tags | `{{ page.bio | newline_to_br }}` |

## Date filter

The `date` filter formats dates using strftime patterns:

```liquid
{{ page.date | date: "%Y-%m-%d" }}
{{ page.date | date: "%B %d, %Y" }}
{{ page.date | date: "%a, %d %b %Y %H:%M:%S %z" }}
```

Common format tokens:

| Token | Output | Example |
|---|---|---|
| `%Y` | 4-digit year | `2026` |
| `%m` | Zero-padded month | `04` |
| `%d` | Zero-padded day | `10` |
| `%B` | Full month name | `April` |
| `%b` | Abbreviated month | `Apr` |
| `%A` | Full weekday | `Friday` |
| `%a` | Abbreviated weekday | `Fri` |
| `%H` | Hour (24h) | `14` |
| `%M` | Minute | `30` |
| `%S` | Second | `00` |
| `%z` | Timezone offset | `+0000` |

Input dates are parsed from ISO 8601, RFC 3339, `YYYY-MM-DD HH:MM:SS`, or `YYYY-MM-DD` formats.

## Array filters

| Filter | Description | Example |
|---|---|---|
| `sort` | Sort (numeric-aware) | `{{ site.data.items | sort: "order" }}` |
| `reverse` | Reverse order | `{{ collection | reverse }}` |
| `first` | First element | `{{ posts | first }}` |
| `last` | Last element | `{{ posts | last }}` |
| `size` | Element count | `{{ posts | size }}` |
| `where` | Filter by field value | `{{ posts | where: "draft", false }}` |
| `group_by` | Group into map by field | `{{ posts | group_by: "category" }}` |
| `map` | Extract field from each item | `{{ posts | map: "title" }}` |
| `flatten` | Flatten nested arrays one level | `{{ nested_tags | flatten }}` |
| `uniq` | Remove duplicates | `{{ all_tags | uniq }}` |
| `compact` | Remove nil values | `{{ items | compact }}` |
| `concat` | Concatenate two arrays | `{{ a | concat: b }}` |

The `sort` filter is numeric-aware: when values can be parsed as numbers, it compares them numerically. Strings `"2"`, `"10"`, `"1"` sort as `1, 2, 10` -- not `1, 10, 2`.

Sort by a nested key:

```liquid
{% assign sorted = collections.articles | sort: "order" %}
{% for article in sorted %}
  <a href="{{ article.url }}">{{ article.title }}</a>
{% endfor %}
```

## Set operation filters

| Filter | Description | Example |
|---|---|---|
| `intersect` | Elements in both arrays | `{{ a | intersect: b }}` |
| `union` | Combined unique elements | `{{ a | union: b }}` |
| `complement` | Elements in first but not second | `{{ a | complement: b }}` |

```liquid
{% assign common_tags = page.tags | intersect: featured_tags %}
{% for tag in common_tags %}
  <span class="featured">{{ tag }}</span>
{% endfor %}
```

## URL filters

| Filter | Description | Example |
|---|---|---|
| `url` | Ensure leading slash | `{{ "about" | url }}` &rarr; `/about` |
| `absolute_url` | Prepend base URL | `{{ page.url | absolute_url: site.baseURL }}` |
| `url_encode` | Percent-encode for URLs | `{{ "hello world" | url_encode }}` |
| `url_decode` | Decode percent-encoded string | `{{ encoded | url_decode }}` |

## Math filters

| Filter | Description | Example |
|---|---|---|
| `plus` | Add | `{{ 4 | plus: 2 }}` &rarr; `6` |
| `minus` | Subtract | `{{ 10 | minus: 3 }}` &rarr; `7` |
| `times` | Multiply | `{{ 3 | times: 4 }}` &rarr; `12` |
| `divided_by` | Integer divide | `{{ 10 | divided_by: 3 }}` &rarr; `3` |
| `modulo` | Remainder | `{{ 10 | modulo: 3 }}` &rarr; `1` |
| `ceil` | Round up | `{{ 4.2 | ceil }}` &rarr; `5` |
| `floor` | Round down | `{{ 4.8 | floor }}` &rarr; `4` |
| `round` | Round to nearest | `{{ 4.5 | round }}` &rarr; `5` |
| `abs` | Absolute value | `{{ -3 | abs }}` &rarr; `3` |

## Content filters

| Filter | Description |
|---|---|
| `markdownify` | Render Markdown string to HTML |
| `json` | Serialize value to JSON string |
| `default` | Return fallback if value is nil or empty string |
| `safeHTML` | Mark string as safe (no escaping) |

```liquid
{{ page.description | markdownify }}
{{ page.custom_field | default: "No value" }}
{{ site.data.config | json }}
```

## Regex filters

| Filter | Description | Example |
|---|---|---|
| `findRE` | Find all regex matches | `{{ page.body | findRE: "<h2>(.*?)</h2>" }}` |
| `replaceRE` | Replace regex matches | `{{ page.body | replaceRE: "</?em>", "" }}` |

## Asset filters

| Filter | Description |
|---|---|
| `fingerprint` | Insert content hash before file extension |
| `cachebust` | Append `?h=<hash>` query string from file contents |
| `get_hash` | Return SHA hash of file contents (256/384/512, hex or base64) |

```liquid
<link rel="stylesheet" href="{{ "/css/main.css" | cachebust }}">
<!-- outputs: /css/main.css?h=a1b2c3d4e5f6 -->

<script src="{{ "/js/app.js" | fingerprint }}"></script>
<!-- outputs: /js/app.a1b2c3d4.js -->
```

Asset filters resolve files from `static/`, `assets/`, and `content/` directories.

The `get_hash` filter accepts optional arguments for hash algorithm and encoding:

```liquid
{{ "/css/main.css" | get_hash }}              <!-- SHA-256, base64 -->
{{ "/css/main.css" | get_hash: 384 }}         <!-- SHA-384, base64 -->
{{ "/css/main.css" | get_hash: 256, false }}   <!-- SHA-256, hex -->
```

## Filter chaining

Combine filters to build complex transformations:

```liquid
{{ collections.articles | where: "draft", false | sort: "date" | reverse | first }}

{{ page.tags | sort | uniq | join: ", " | prepend: "Tags: " }}

{{ page.title | downcase | slugify | prepend: "/posts/" | append: "/" }}
```

Each pipe passes the result to the next filter. Read left to right to understand the transformation pipeline.
