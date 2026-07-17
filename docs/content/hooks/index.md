---
layout: doc
title: Lifecycle Events
nav_weight: 10
description: "Reference for Alloy's lifecycle hooks, the events plugins use to modify content, inject pages, and observe builds."
---

Lifecycle hooks let plugins run code at specific points in the build pipeline. Register a hook with `alloy.hook()` or `alloy.on()` to modify content, inject pages, transform data, or observe build events.

```javascript
// plugins/lazy-images.js
export default function(alloy) {
  alloy.hook("onContentTransformed", {}, (page) => {
    page.html = page.html.replace(/<img /g, '<img loading="lazy" ');
    return page;
  });
}
```

Hooks work identically across all plugin tiers (QuickJS, WASM, Node). Payloads are JSON-serializable.

## Hook Registration

```javascript
alloy.hook(eventName, options, handlerFn);
// or equivalently:
alloy.on(eventName, options, handlerFn);
```

The `options` object is required. It controls execution order and payload scoping:

```javascript
alloy.hook("onPageRendered", {
  priority: 10,                       // lower runs first (default 50)
  data: ["navigation"],               // site.data keys to include
  pages: "/blog/**",                  // page filter (glob)
  pageFields: ["frontMatter", "url"]  // fields per page
}, fn);
```

See [Hook Scoping](/hooks/scoping/) for the full scoping API.

## All Lifecycle Events

### Per-Build Hooks

These fire once per build. Payloads are JSON objects representing build-level state.

#### onConfig

Fires after config is loaded. Plugin can mutate configuration values.

```javascript
alloy.hook("onConfig", {}, (config) => {
  config.build.output = "dist";
  return config;
});
```

| Field | Description |
|---|---|
| `title` | Site title |
| `baseURL` | Site base URL |
| `build` | Build settings (`output`, `clean`) |
| ... | All config fields |

#### onDataFetched

Fires after external data sources are fetched and merged into site data. Plugin can modify or enrich the data.

```javascript
alloy.hook("onDataFetched", { data: ["team"] }, (data) => {
  if (data.team) {
    data.teamCount = data.team.length;
    data.teamByDepartment = {};
    for (const member of data.team) {
      const dept = member.department || "unassigned";
      if (!data.teamByDepartment[dept]) data.teamByDepartment[dept] = [];
      data.teamByDepartment[dept].push(member);
    }
  }
  return data;
});
```

This is the primary mechanism for adding computed data that templates can access via `site.data.*`.

#### onBeforeValidation

Fires before output path conflict detection. The payload is a map of output paths to source identifiers. Plugins can add entries by mutating the map directly — the return value is discarded.

```javascript
alloy.hook("onBeforeValidation", {}, (paths) => {
  paths["_redirects"] = "plugin:netlify-redirects";
  paths["_headers"] = "plugin:netlify-headers";
});
```

> **Note:** Mutations work because in-process plugin runtimes (QuickJS, WASM) share the underlying Go map. The return value is not processed — add entries by writing keys on the payload object, not by returning a new object.

#### onAfterValidation

Fires after validation passes. The payload is the same output path map from `onBeforeValidation`, now including any entries plugins added. The return value is discarded.

```javascript
alloy.hook("onAfterValidation", {}, (paths) => {
  console.log(`Validated ${Object.keys(paths).length} output paths`);
});
```

> **Note:** The `cascade` payload described in PLAN.md is not yet implemented. Site data injection after validation is tracked in a separate issue.

### Pre-Taxonomy Hook

#### onPagesReady

Fires once per language batch, after the data cascade is applied but before taxonomy collection. This is the injection point for virtual pages that need to participate in taxonomies.

```javascript
alloy.hook("onPagesReady", { data: ["elements"], pages: false }, (payload) => {
  var elements = payload.siteData.elements || [];
  var newPages = [];
  for (var i = 0; i < elements.length; i++) {
    var el = elements[i];
    newPages.push({
      path: "demos/" + el.slug + ".md",
      url: "/demos/" + el.slug + "/",
      frontMatter: {
        title: el.name + " Demo",
        layout: "demo",
        tags: [el.tagName]
      },
      content: "## " + el.name + "\n\n" + el.description
    });
  }
  return { addPages: newPages };
});
```

**Virtual page fields:**

| Field | Required | Description |
|---|---|---|
| `path` | yes | Source-relative identifier (e.g., `demos/button.md`) |
| `url` | yes | Permalink (e.g., `/demos/button/`) |
| `frontMatter` | no | Page metadata, including taxonomy terms like `tags` |
| `content` | no | Raw markdown content (rendered through the pipeline) |

Virtual pages injected here flow through the full remaining pipeline: taxonomy collection, content rendering, layout resolution, and output writing.

When using `pages: false` in the options, return `{ addPages: [...] }` to inject pages without round-tripping all existing pages through the plugin bridge.

### Content Hooks

#### onContentLoaded

Fires once with the full pages array after content rendering. Modify `frontMatter` and `html` on existing pages. Other fields (`content`, `path`, `url`) are present for inspection but mutations are not applied back.

```javascript
alloy.hook("onContentLoaded", {
  pages: true,
  pageFields: ["frontMatter", "html", "url"]
}, (pages) => {
  pages.forEach(page => {
    if (page.frontMatter.draft) {
      page.frontMatter.noindex = true;
    }
  });
  return pages;
});
```

The return array must be the same length and order as the input. Virtual page injection is not supported here -- use `onPagesReady` instead.

#### onDataCascadeReady

Fires once with the full pages array after the data cascade is resolved. Each entry has the per-page cascade data. Plugin can enrich cascade data.

```javascript
alloy.hook("onDataCascadeReady", { pages: true }, (pages) => {
  pages.forEach(page => {
    page.data.generatedAt = new Date().toISOString();
  });
  return pages;
});
```

### Per-Page Hooks

These fire once per page. They receive page-scoped payloads.

#### onContentTransformed

Fires after Markdown-to-HTML conversion but before layout rendering. Receives a page-scoped object with `html`, `toc`, `path`, `url`, and `frontMatter`.

```javascript
alloy.hook("onContentTransformed", {}, (page) => {
  // Add lazy loading to images
  page.html = page.html.replace(/<img /g, '<img loading="lazy" ');

  // Build TOC for non-markdown pages
  if (!page.toc || page.toc.length === 0) {
    page.toc = extractHeadingsFromHTML(page.html);
  }

  return page;
});
```

#### onPageRendered

Fires after template rendering produces the final page HTML. Receives an HTML string and returns an HTML string.

```javascript
alloy.hook("onPageRendered", {}, (html) => {
  return html.replace(/\s+/g, ' ').trim();
});
```

### Per-Asset Hook

#### onAssetProcess (Tier 3 Only)

Fires once per asset file during asset copy. Receives `{ path, content }` and returns `{ content }`.

```javascript
alloy.hook("onAssetProcess", {}, async (asset) => {
  if (asset.path.endsWith('.css')) {
    return { content: await minifyCSS(asset.content) };
  }
  return asset;
});
```

### Read-Only Hooks

Return values are ignored. Plugins observe but cannot modify.

#### onBuildComplete

```javascript
alloy.hook("onBuildComplete", {}, (result) => {
  console.log(`Built ${result.pageCount} pages in ${result.duration}`);
});
```

#### onDevServerStart

```javascript
alloy.hook("onDevServerStart", {}, (info) => {
  console.log(`Server ready at ${info.url}`);
});
```

#### onFileChanged

```javascript
alloy.hook("onFileChanged", {}, (filePath) => {
  console.log(`Changed: ${filePath}`);
});
```

## Hook Execution Order

Hooks execute by priority (lower runs first), then by alphabetical plugin filename within the same priority. Each hook receives the output of the previous one -- they chain, not race.

```javascript
// Plugin A: runs first
alloy.hook("onPageRendered", { priority: 10 }, transformFn);

// Plugin B: runs second (default priority 50)
alloy.hook("onPageRendered", {}, analyticsFn);

// Plugin C: runs last
alloy.hook("onPageRendered", { priority: 100 }, ssrFn);
```

## Hook Timeout

Each hook call is subject to the configured timeout (default 5 seconds). A timed-out hook produces a warning, its modifications are discarded, and the build continues with the pre-hook payload.

```yaml
plugins:
  timeout: 5000
```

## Related

- [Hook Scoping](/hooks/scoping/) -- control what data hooks receive
- [Plugin System](/plugins/) -- plugin tiers and registration
- [QuickJS Plugins](/plugins/quickjs/) -- embedded JS plugins
- [Node Plugins](/plugins/node/) -- subprocess plugins with npm access
