---
layout: doc
title: Hook Scoping
nav_weight: 20
---

Hook scoping controls what subset of site data and pages a hook receives. By declaring what your hook needs at registration time, you reduce the serialization cost of each hook dispatch -- especially important for large sites with thousands of pages.

```javascript
alloy.hook("onContentLoaded", {
  data: ["navigation", "team"],       // only these site.data keys
  pages: "/blog/**",                  // only blog pages
  pageFields: ["frontMatter", "url"]  // only these fields per page
}, (pages) => {
  // Receives only what was declared
  return pages;
});
```

## The `scope:` Options

### `data` -- Site Data Keys

Controls which `site.data` keys are serialized in the hook payload.

| Value | Behavior |
|---|---|
| `undefined` (omitted) | No site data serialized |
| `["navigation", "team"]` | Only these keys |
| `["*"]` | All site data keys |

```javascript
// Only receive navigation data
alloy.hook("onAfterValidation", { data: ["navigation"] }, (payload) => {
  // payload.cascade has navigation data
});

// Receive all site data
alloy.hook("onDataFetched", { data: ["*"] }, (data) => {
  // Full site.data available
});
```

### `pages` -- Page Filtering

Controls which pages are included in the hook payload.

| Value | Mode | Description |
|---|---|---|
| `false` (default) | Skip | No pages serialized |
| `true` or `"**"` | All | All pages included |
| `"/blog/**"` | Glob | Only pages matching the path glob |
| `{ tags: ["component"] }` | Taxonomy | Only pages matching taxonomy terms |

```javascript
// No pages (default -- best for data-only hooks)
alloy.hook("onDataFetched", { pages: false }, fn);

// All pages
alloy.hook("onContentLoaded", { pages: true }, fn);

// Only blog pages (glob filter)
alloy.hook("onContentLoaded", { pages: "/blog/**" }, fn);

// Only pages tagged "component" (taxonomy filter)
alloy.hook("onContentLoaded", {
  pages: { tags: ["component", "form"] }
}, fn);
```

#### Taxonomy Filter Rules

Multiple terms within the same taxonomy are OR'd (union):

```javascript
// Pages tagged "component" OR "form"
{ pages: { tags: ["component", "form"] } }
```

Multiple taxonomies are AND'd (intersection):

```javascript
// Pages tagged "component" AND categorized "ui"
{ pages: { tags: ["component"], categories: ["ui"] } }
```

### `pageFields` -- Per-Page Fields

Controls which fields are populated on each page in the payload.

| Value | Behavior |
|---|---|
| `undefined` (omitted) | All fields |
| `["frontMatter", "url"]` | Only these fields |
| `["*"]` | All fields (explicit) |

```javascript
// Only need front matter and URL (skip html, toc, content)
alloy.hook("onContentLoaded", {
  pages: true,
  pageFields: ["frontMatter", "url"]
}, fn);
```

Available fields: `path`, `url`, `frontMatter`, `content`, `html`, `toc`.

## Union Scope

When multiple hooks register for the same event with different scopes, Alloy computes a union and serializes it once for all hooks on that event.

If hook A requests `pageFields: ["html"]` and hook B requests `pageFields: ["toc"]`, both hooks receive pages with `html` AND `toc` populated.

```javascript
// Plugin A
alloy.hook("onContentLoaded", { pageFields: ["html"] }, fnA);

// Plugin B
alloy.hook("onContentLoaded", { pageFields: ["toc"] }, fnB);

// Both receive pages with html AND toc
```

This is a performance optimization -- one serialization pass per event instead of one per hook. Scoping reduces the total work, but it is not an isolation boundary. Plugin A may see fields it did not request if another plugin on the same event requested them.

## `addPages` Return Shape

When a hook declares `pages: false` and needs to inject virtual pages (via `onPagesReady`), use the `addPages` return shape to avoid round-tripping all existing pages:

```javascript
alloy.hook("onPagesReady", {
  data: ["elements"],
  pages: false          // don't serialize existing pages
}, (payload) => {
  const elements = payload.siteData.elements || [];
  const newPages = elements.map(el => ({
    path: `demos/${el.slug}.md`,
    url: `/demos/${el.slug}/`,
    frontMatter: { title: `${el.name} Demo`, layout: "demo" },
    content: `## ${el.name}\n\n${el.description}`
  }));
  return { addPages: newPages };
});
```

The pipeline detects the `addPages` key and appends the new pages without requiring the plugin to process existing pages.

## Hook Availability Matrix

Not all scope options work on all hooks. Two constraints apply:

1. **Pageless hooks** (`onConfig`, `onBeforeValidation`, `onAfterValidation`, `onDataFetched`) do not receive pages. Setting `pages` to anything other than `false` produces a validation error.

2. **Pre-taxonomy hooks** (`onPagesReady`) cannot use taxonomy filtering because taxonomy indices have not been built yet.

| Hook | Pages | Glob Filter | Taxonomy Filter |
|---|:---:|:---:|:---:|
| `onConfig` | no | error | error |
| `onBeforeValidation` | no | error | error |
| `onAfterValidation` | no | error | error |
| `onDataFetched` | no | error | error |
| `onPagesReady` | yes (batch) | yes | error |
| `onContentLoaded` | yes (batch) | yes | yes |
| `onDataCascadeReady` | yes (batch) | yes | yes |
| `onContentTransformed` | per-page | n/a | n/a |
| `onPageRendered` | per-page | n/a | n/a |
| `onAssetProcess` | per-asset | n/a | n/a |
| `onBuildComplete` | no | n/a | n/a |

Per-page hooks (`onContentTransformed`, `onPageRendered`) fire with a fixed payload shape. The pipeline already knows which page to serialize, so `pages` and `pageFields` scope options do not apply. Use `data` and `priority` on per-page hooks.

Registering an invalid scope mode on a hook produces a validation error at plugin load time -- before the build starts.

## Practical Example: Docs Site with Component Demos

A documentation site that generates demo pages from a data file, only processing pages it cares about:

```javascript
// plugins/demo-generator.js
export default function(alloy) {
  // Generate demo pages from data (only needs elements data, no existing pages)
  alloy.hook("onPagesReady", {
    data: ["elements"],
    pages: false
  }, (payload) => {
    const elements = payload.siteData.elements || [];
    return {
      addPages: elements.map(el => ({
        path: `demos/${el.slug}.md`,
        url: `/demos/${el.slug}/`,
        frontMatter: {
          title: `${el.name} Demo`,
          layout: "demo",
          tags: [el.tagName]
        },
        content: `## ${el.name}\n\n${el.description}`
      }))
    };
  });

  // Enrich only demo pages after rendering (glob filter)
  alloy.hook("onContentLoaded", {
    pages: "/demos/**",
    pageFields: ["frontMatter", "html"]
  }, (pages) => {
    pages.forEach(page => {
      page.frontMatter.generatedAt = new Date().toISOString();
    });
    return pages;
  });
}
```

## Related

- [Lifecycle Events](/hooks/) -- all hook events and payloads
- [Plugin System](/plugins/) -- plugin tiers and registration
