---
layout: doc
title: Hook Scoping
---

The options object passed to `alloy.hook()` controls which pages a hook sees and what data it receives. Scoping reduces payload size and lets Alloy optimize hook execution.

```javascript
alloy.hook("onContentTransformed", {
  priority: 50,
  pages: "content/blog/**",
  pageFields: ["title", "body", "url"],
  data: ["site"],
}, (page) => {
  // Only fires for blog pages
  // page only has title, body, and url fields
  return page;
});
```

## The options object

| Option | Type | Default | Description |
|---|---|---|---|
| `priority` | number | 50 | Execution order. Lower runs first. |
| `pages` | `false` / `true` / `string` | `false` | Which pages trigger this hook |
| `pageFields` | array of strings | all fields | Subset of page fields included in the payload |
| `data` | array of strings | none | Site data keys to include in the payload |

The options object is required. At minimum, pass `{}` or `{ priority: 50 }`.

## Pages filtering

The `pages` option determines which pages a per-page hook (`onContentTransformed`, `onPageRendered`) fires for, and whether batch hooks receive page data.

### pages: false

The hook does not receive any page data. Use this for hooks that operate on non-page data (config, assets, build stats):

```javascript
alloy.hook("onConfig", { pages: false }, (data) => {
  // No page data in payload
  data.config.customSetting = true;
  return data;
});
```

### pages: true

The hook fires for every page:

```javascript
alloy.hook("onContentTransformed", { pages: true }, (page) => {
  // Fires for every page in the site
  return page;
});
```

### Glob patterns

A glob string filters pages by their source path relative to `content/`:

```javascript
// Only blog posts
alloy.hook("onContentTransformed", {
  pages: "blog/**",
}, (page) => {
  return page;
});

// Only top-level pages (not in subdirectories)
alloy.hook("onContentTransformed", {
  pages: "*.md",
}, (page) => {
  return page;
});

// Multiple sections
alloy.hook("onContentTransformed", {
  pages: "{blog,news}/**",
}, (page) => {
  return page;
});
```

The glob uses `**` for recursive matching and `*` for single-segment wildcards.

### Taxonomy filter

Filter pages by taxonomy term:

```javascript
alloy.hook("onContentTransformed", {
  pages: "taxonomy:tags/javascript",
}, (page) => {
  // Only pages tagged "javascript"
  return page;
});
```

The format is `taxonomy:<taxonomy-name>/<term>`.

## pageFields

By default, hooks receive all page fields. Use `pageFields` to request only the fields you need, reducing serialization overhead for Node plugins:

```javascript
alloy.hook("onContentTransformed", {
  pages: true,
  pageFields: ["body", "url"],
}, (page) => {
  // page.body and page.url are present
  // page.title, page.date, etc. are not included
  page.body = transform(page.body);
  return page;
});
```

Fields you set on the returned page are merged back into the full page object, even if they were not in `pageFields`. This lets you modify `body` without requesting `title`.

## data

Request specific site data keys to be included in the hook payload:

```javascript
alloy.hook("onContentTransformed", {
  pages: true,
  data: ["site", "collections"],
}, (page) => {
  const blogPosts = alloy.data.collections.blog;
  // Use site-wide data while processing each page
  return page;
});
```

Without `data`, site data is not included in the per-call payload (though `alloy.data` remains available in QuickJS plugins). For Node plugins, specifying `data` keys avoids sending the entire site data snapshot over the JSON-RPC channel on every call.

## Union scope

When multiple hooks subscribe to the same event, Alloy computes the **union** of their requested scopes. If hook A requests `pageFields: ["title", "body"]` and hook B requests `pageFields: ["body", "url"]`, both hooks receive pages with `title`, `body`, and `url`.

```javascript
// Hook A
alloy.hook("onContentTransformed", {
  pages: true,
  pageFields: ["title", "body"],
}, (page) => { /* ... */ });

// Hook B
alloy.hook("onContentTransformed", {
  pages: true,
  pageFields: ["body", "url"],
}, (page) => {
  // Receives title, body, AND url — union of both hooks' requests
  return page;
});
```

The same union logic applies to `data` keys and `pages` globs. If one hook requests `pages: "blog/**"` and another requests `pages: true`, the effective scope is all pages.

Scoping is an optimization hint, not an isolation boundary. A hook may receive more data than it requested due to union computation with other hooks on the same event.

## Hook availability matrix

Not all scoping options apply to every event. Per-page options (`pages`, `pageFields`) are only meaningful for events that carry page data:

| Event | `pages` | `pageFields` | `data` | Notes |
|---|---|---|---|---|
| `onConfig` | -- | -- | -- | Receives config object only |
| `onDataFetched` | -- | -- | Yes | Operates on raw data files |
| `onContentLoaded` | Yes | Yes | Yes | Batch, receives all pages |
| `onDataCascadeReady` | Yes | Yes | Yes | Batch, post-cascade |
| `onPagesReady` | Yes | Yes | Yes | Batch, return `addPages` |
| `onBeforeValidation` | Yes | Yes | Yes | Batch |
| `onAfterValidation` | Yes | Yes | Yes | Batch |
| `onContentTransformed` | Yes | Yes | Yes | Per-page |
| `onPageRendered` | Yes | Yes | Yes | Per-page |
| `onAssetProcess` | -- | -- | -- | Receives asset object |
| `onBuildComplete` | -- | -- | -- | Receives build stats |
| `onDevServerStart` | -- | -- | -- | Receives server object |
| `onFileChanged` | -- | -- | -- | Receives file path and event |

A `--` means the option is ignored for that event.

## addPages return shape for onPagesReady

The `onPagesReady` hook has a unique return shape. Instead of returning a modified page, it returns an object with an `addPages` array of virtual page definitions:

```javascript
alloy.hook("onPagesReady", {
  priority: 50,
  pages: true,
  pageFields: ["title", "date", "tags", "url"],
}, (data) => {
  // Read existing pages to generate new ones
  const tagCounts = {};
  for (const page of data.pages) {
    for (const tag of (page.tags || [])) {
      tagCounts[tag] = (tagCounts[tag] || 0) + 1;
    }
  }

  return {
    addPages: [{
      title: "Tag Statistics",
      permalink: "/stats/tags/",
      layout: "stats",
      tagCounts: tagCounts,
      body: "",
    }],
  };
});
```

Each virtual page in `addPages` must be a map with at minimum:

| Field | Required | Description |
|---|---|---|
| `permalink` | Yes | Output URL for the page |
| `title` | Yes | Page title |
| `layout` | No | Layout template name |
| `body` | No | Page body content (pre-rendered HTML or Markdown) |
| Any key | No | Custom front matter fields |

Virtual pages are injected before taxonomy processing. They participate in collections and can carry taxonomy terms (`tags`, `categories`, etc.) just like regular content pages.
