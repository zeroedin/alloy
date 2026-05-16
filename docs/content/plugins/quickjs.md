---
layout: doc
title: QuickJS Plugins
---

QuickJS plugins are plain `.js` files in the `plugins/` directory. No build step, no bundler, no npm install. Drop a file in, and it runs sandboxed via wazero at microsecond-level latency.

```javascript
// plugins/word-count.js
alloy.filter("wordcount", (content) => {
  return content.split(/\s+/).filter(w => w.length > 0).length.toString();
});
```

```liquid
{{ page.content | wordcount }} words
```

## Registering filters

Use `alloy.filter(name, fn)` to register a Liquid filter. The function receives the input value and any arguments passed in the template:

```javascript
// plugins/reading-time.js
alloy.filter("reading_time", (content) => {
  const words = content.split(/\s+/).length;
  const minutes = Math.ceil(words / 250);
  return `${minutes} min read`;
});
```

```liquid
{{ page.content | reading_time }}
```

Filters with arguments:

```javascript
// plugins/truncate-words.js
alloy.filter("truncate_smart", (text, maxWords) => {
  const words = text.split(/\s+/);
  if (words.length <= maxWords) return text;
  return words.slice(0, maxWords).join(" ") + "...";
});
```

```liquid
{{ page.summary | truncate_smart: 30 }}
```

## Registering shortcodes

Use `alloy.shortcode(name, fn)` to register a custom Liquid tag. The function receives an array of string arguments and, for block shortcodes, the inner body content:

```javascript
// plugins/youtube.js
alloy.shortcode("youtube", (args) => {
  const id = args[0];
  return `<div class="video-embed">
    <iframe src="https://www.youtube.com/embed/${id}"
            frameborder="0" allowfullscreen></iframe>
  </div>`;
});
```

```liquid
{% youtube "dQw4w9WgXcQ" %}
```

Block shortcodes receive the body as the second argument:

```javascript
// plugins/callout.js
alloy.shortcode("callout", (args, body) => {
  const type = args[0] || "info";
  return `<aside class="callout callout-${type}">${body}</aside>`;
});
```

```liquid
{% callout "warning" %}
  Back up your data before upgrading.
{% endcallout %}
```

## Accessing site data

`alloy.data` provides a read-only snapshot of site data. It is available inside filter and shortcode functions, not at the top level of the plugin file:

```javascript
// plugins/site-title.js
alloy.filter("with_site_title", (text) => {
  const title = alloy.data.site.title;
  return `${text} | ${title}`;
});
```

`alloy.data` includes:

| Key | Contents |
|---|---|
| `alloy.data.site` | Config values (`title`, `baseURL`, etc.) |
| `alloy.data.collections` | All named collections |
| `alloy.data.taxonomies` | All taxonomy term maps |

The data is a snapshot taken at plugin execution time. It is read-only -- mutations have no effect on the build.

## Registering hooks

Use `alloy.hook(event, options, fn)` to subscribe to build lifecycle events. The options object is required:

```javascript
// plugins/image-lazy.js
alloy.hook("onContentTransformed", { priority: 50, pages: true }, (page) => {
  page.body = page.body.replace(
    /<img /g,
    '<img loading="lazy" '
  );
  return page;
});
```

See [Lifecycle Events](/hooks/) for the full list of events and their payloads.

## Performance

QuickJS plugins run inside wazero (pure Go WebAssembly runtime) with these characteristics:

| Metric | Typical value |
|---|---|
| Startup | ~10-50ms (once, at build start) |
| Per-call latency | ~10-50 microseconds |
| Memory overhead | ~2-4MB per plugin instance |

For a site with 1000 pages and a filter called once per page, total plugin overhead is roughly 10-50ms -- negligible compared to I/O and template rendering.

## Sandboxing

QuickJS plugins run in a strict sandbox enforced by wazero:

- No filesystem access
- No network access
- No system calls
- No access to the host process

The only data a QuickJS plugin can access is what Alloy passes to it through function arguments and `alloy.data`. This makes QuickJS plugins safe to run from untrusted sources.

## Limitations

- No `require()` or `import` -- each plugin is a single self-contained file
- No async/await -- all functions are synchronous
- No Node.js APIs (`fs`, `path`, `http`, etc.)
- No npm packages -- if you need npm, use a [Node plugin](/plugins/node/)
- `alloy.data` is not available at the top level of the file, only inside registered functions
