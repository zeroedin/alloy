---
layout: doc
title: QuickJS Plugins
nav_weight: 20
description: "Write plain JavaScript plugins that run on an embedded QuickJS engine, no Node.js dependency and no build step."
---

QuickJS plugins are plain JavaScript files that run on an embedded QuickJS engine inside the Alloy process. No Node.js dependency, no build step -- drop a `.js` file in `plugins/` and it works immediately.

```javascript
// plugins/reading-time.js
export default function(alloy) {
  alloy.filter("readingTime", (content) => {
    const words = content.split(/\s+/).filter(w => w.length > 0).length;
    const minutes = Math.ceil(words / 200);
    return `${minutes} min read`;
  });
}
```

```liquid
<span>{{ page.content | readingTime }}</span>
```

## How It Works

Alloy embeds a single QuickJS instance compiled to WASM, running via wazero (pure Go, zero CGo). Your `.js` files are evaluated in this shared context at startup.

- **Startup cost**: ~10-50ms one-time
- **Per-call cost**: ~10-50 microseconds
- **Memory**: ~2-4MB

QuickJS plugins are sandboxed -- they cannot access the filesystem, network, or system resources.

## Registering Filters

Filters transform values in template expressions:

```javascript
// plugins/filters.js
export default function(alloy) {
  // Simple string filter
  alloy.filter("initials", (name) => {
    return name.split(' ').map(w => w[0]).join('');
  });

  // Filter with arguments
  alloy.filter("truncateAt", (text, maxLength) => {
    if (text.length <= maxLength) return text;
    return text.slice(0, maxLength) + '...';
  });

  // Numeric filter
  alloy.filter("percentage", (value, total) => {
    return Math.round((value / total) * 100) + '%';
  });
}
```

Use in templates:

```liquid
{{ page.author | initials }}
{{ page.description | truncateAt: 120 }}
{{ page.score | percentage: page.maxScore }}
```

Filter arguments are passed as additional parameters after the input value.

## Registering Shortcodes

Shortcodes embed rich HTML snippets in content files:

```javascript
// plugins/shortcodes.js
export default function(alloy) {
  // Inline shortcode (self-closing)
  alloy.shortcode("youtube", (args) => {
    const id = args[0];
    return `<iframe src="https://www.youtube.com/embed/${id}"
            frameborder="0" allowfullscreen></iframe>`;
  });

  // Block shortcode (wraps content)
  alloy.shortcode("callout", (args, content) => {
    const type = args[0] || "info";
    return `<div class="callout callout--${type}">${content}</div>`;
  });

  // Shortcode using site data
  alloy.shortcode("componentDemo", (args) => {
    const tagName = args[0];
    const elements = alloy.data.elements || [];
    const el = elements.find(e => e.tagName === tagName);
    if (!el) return `<!-- unknown component: ${tagName} -->`;
    return `<div class="demo">
      <h3>${el.name}</h3>
      <${el.tagName}></${el.tagName}>
    </div>`;
  });
}
```

Use in content:

```liquid
{% youtube "dQw4w9WgXcQ" %}

{% callout "warning" %}
Do not use this in production without testing first.
{% endcallout %}

{% componentDemo "rh-button" %}
```

## Registering Hooks

Hooks let plugins run code at specific points in the build pipeline:

```javascript
// plugins/transforms.js
export default function(alloy) {
  // Add lazy loading to all images
  alloy.hook("onContentTransformed", {}, (page) => {
    page.html = page.html.replace(/<img /g, '<img loading="lazy" ');
    return page;
  });

  // Minify final HTML output
  alloy.hook("onPageRendered", {}, (html) => {
    return html.replace(/\s+/g, ' ').trim();
  });
}
```

### Hook Options

The second argument to `alloy.hook()` is a required options object:

```javascript
// Control execution order with priority (lower runs first, default 50)
alloy.hook("onPageRendered", { priority: 10 }, earlyTransformFn);
alloy.hook("onPageRendered", { priority: 100 }, lateTransformFn);

// Declare what data the hook needs (reduces serialization cost)
alloy.hook("onContentLoaded", {
  data: ["navigation"],     // only serialize these site.data keys
  pages: "/blog/**",        // only receive blog pages
  pageFields: ["frontMatter", "url"]  // only these fields per page
}, fn);
```

See [Lifecycle Events](/hooks/) for all available hooks and [Hook Scoping](/hooks/scoping/) for the full scoping API.

## Accessing Site Data

`alloy.data` provides read-only access to the same data available as `site.data` in templates:

```javascript
export default function(alloy) {
  alloy.filter("teamMember", (slug) => {
    const team = alloy.data.team || [];
    const member = team.find(m => m.slug === slug);
    return member ? member.name : slug;
  });
}
```

Access `alloy.data` inside filter, shortcode, and hook functions -- not at the top level of your plugin. During plugin evaluation, `alloy.data` is `undefined`.

To modify data, use hooks like `onDataFetched`:

```javascript
alloy.hook("onDataFetched", { data: ["team"] }, (data) => {
  data.teamCount = data.team.length;
  return data;
});
```

## QuickJS vs Node

A `.js` file in `plugins/` runs on QuickJS by default. If your plugin needs Node APIs or npm packages, add `runtime: "node"`:

```javascript
// This runs on QuickJS (default)
export default function(alloy) { /* ... */ }

// This runs on Node (Tier 3)
export const runtime = "node";
export default function(alloy) { /* ... */ }
```

See [Node Plugins](/plugins/node/) for details on Tier 3 plugins.

## Limitations

- No filesystem access (`fs`, `path`)
- No network access (`fetch`, `http`)
- No Node.js APIs or npm packages
- No `require()` or `import` of external modules
- The `alloy.data` accessor is a read-only snapshot -- assigning to it inside a filter or shortcode doesn't change what templates see. To add or modify data templates can read, mutate and return the `data` payload in a data hook like `onDataFetched` (shown above).

For any of these capabilities, use a [Node plugin](/plugins/node/) instead.

## Related

- [Plugin System](/plugins/) -- overview and tier comparison
- [WASM Plugins](/plugins/wasm/) -- compiled plugins for maximum performance
- [Node Plugins](/plugins/node/) -- full Node.js access
- [Lifecycle Events](/hooks/) -- all hook events and payloads
