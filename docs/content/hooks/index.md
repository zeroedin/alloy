---
layout: doc
title: Lifecycle Events
nav_weight: 10
description: "Reference for Alloy's lifecycle hooks, the events plugins use to modify content, inject pages, and observe builds."
---

Lifecycle hooks let plugins run code at specific points in the build. Register one with `alloy.hook()` to modify content, inject pages, transform data, or observe the build.

```javascript
// plugins/lazy-images.js
export default function(alloy) {
  alloy.hook("onContentTransformed", {}, (page) => {
    page.html = page.html.replace(/<img /g, '<img loading="lazy" ');
    return page;
  });
}
```

That is the whole shape of a hook: pick an event, receive a payload, return it modified.

## Registering a Hook

```javascript
alloy.hook(eventName, options, handlerFn);
// alloy.on() is an alias for the same thing
alloy.on(eventName, options, handlerFn);
```

The `options` object is **required**, even when empty — pass `{}`. It controls execution order and how much data your hook receives:

```javascript
alloy.hook("onPageRendered", {
  priority: 10,                       // lower runs first (default 50)
  data: ["navigation"],               // site.data keys to include
  pages: "/blog/**",                  // page filter (glob)
  pageFields: ["frontMatter", "url"]  // fields per page
}, fn);
```

Everything you ask for is serialized and handed to your plugin, so scoping is the main lever on plugin performance. See [Hook Scoping](/hooks/scoping/) for the full API.

## When Hooks Fire

Hooks fire in a fixed order during a build. Use this to pick the right one — the earlier a hook runs, the less is decided, and the more you can still influence.

| Order | Hook | Fires |
|---|---|---|
| 1 | `onConfig` | Config loaded, before anything is read from disk |
| 2 | `onDataFetched` | External data sources fetched and merged |
| 3 | `onPagesReady` | Pages discovered, before taxonomy collection — the virtual page injection point |
| 4 | `onBeforeValidation` | Output paths computed, before conflict detection |
| 5 | `onAfterValidation` | Conflict detection passed |
| 6 | `onContentLoaded` | All pages loaded, with rendered content available |
| 7 | `onDataCascadeReady` | Per-page data cascade resolved |
| 8 | `onContentTransformed` | Per page — Markdown converted to HTML, before layout |
| 9 | `onPageRendered` | Per page — layout applied, final HTML ready |
| 10 | `onFormatRendered` | Per non-HTML output format (`json`, `xml`, …) |
| 11 | `onAssetProcess` | Per file in the assets directory |
| 12 | `onBuildComplete` | Build finished |

Two hooks fire only under `alloy dev`: `onDevServerStart` when the server boots, and `onFileChanged` on every file-watch batch.

## Choosing a Hook

| You want to… | Use |
|---|---|
| Change config before the build reads it | `onConfig` |
| Add computed values to `site.data` | `onDataFetched` |
| Generate pages from data | `onPagesReady` |
| Register extra output files | `onBeforeValidation` |
| Add build-wide values for templates | `onAfterValidation` |
| Modify front matter across all pages at once | `onContentLoaded` |
| Rewrite a page's HTML before its layout is applied | `onContentTransformed` |
| Rewrite a page's final HTML | `onPageRendered` |
| Post-process JSON/XML output | `onFormatRendered` |
| Minify or transform CSS/JS assets | `onAssetProcess` |
| Report stats or write a manifest | `onBuildComplete` |

When two hooks could work, prefer the later one for HTML changes — `onPageRendered` sees the finished page — and the earlier one for anything that must influence downstream stages.

## Rules That Apply to Every Hook

### Execution order

Hooks run by **priority** (lower first, default 50). Within the same priority, order is decided by **plugin tier** first, then plugin name:

1. Priority — lower runs first
2. Tier — in-process plugins (QuickJS, WASM) before Node subprocess plugins
3. Plugin name — alphabetical within a tier

The tier step matters when you mix plugin types: a QuickJS plugin named `zebra.js` runs before a Node plugin named `alpha.js` at the same priority. If you need a specific order across tiers, set `priority` explicitly rather than relying on filenames.

```javascript
alloy.hook("onPageRendered", { priority: 10 }, runsFirst);
alloy.hook("onPageRendered", {}, runsSecond);          // default 50
alloy.hook("onPageRendered", { priority: 100 }, runsLast);
```

### Chained vs. independent hooks

Most hooks **chain**: each plugin receives the previous plugin's return value, so changes accumulate.

Some hooks run each plugin **independently** against the original payload — plugins do not see each other's returns, and results are collected separately:

| Dispatch | Hooks |
|---|---|
| Chained | `onConfig`, `onDataFetched`, `onContentLoaded`, `onDataCascadeReady`, `onContentTransformed`, `onAssetProcess`, `onBuildComplete`, `onFileChanged`, `onDevServerStart` |
| Independent | `onPagesReady`, `onBeforeValidation`, `onAfterValidation`, `onFormatRendered` |
| Per page, distributed | `onPageRendered` |

`onPageRendered` is dispatched one page at a time and spread across worker processes for speed. See [Node Plugins](/plugins/node/) for what that means for plugin state.

### Timeouts

Each hook call is subject to the configured timeout (default 5000 milliseconds). A timed-out hook produces a warning, its modifications are discarded, and the build continues with the pre-hook payload.

```yaml
plugins:
  timeout: 5000
```

### Payload differences by plugin tier

QuickJS and Node plugins receive hook payloads directly. **WASM plugins receive an envelope** — `{ "event": "<hookName>", "payload": { … } }` — because a single `hook` export multiplexes every event. Return either the payload or the whole envelope; Alloy unwraps it. See [WASM Plugins](/plugins/wasm/) for the module-side contract.

## Hook Reference

### onConfig

Fires after config is loaded but before the build starts. Receives the full configuration object and must return it. Only fields on the mutable allowlist are applied back — all other fields are silently ignored.

```javascript
alloy.hook("onConfig", {}, (config) => {
  config.build.output = "dist";
  config.structure.content = "pages";
  return config;
});
```

**Mutable fields:**

| Field | Type | Description |
|---|---|---|
| `build.output` | string | Output directory |
| `build.clean` | boolean | Clean output before build |
| `structure.content` | string | Content directory |
| `structure.layouts` | string | Layouts directory |
| `structure.assets` | string | Assets directory |
| `structure.static` | string | Static files directory |
| `structure.data` | string | Data directory |
| `passthrough` | array | Passthrough file mappings (`[{ from, to }]`) |
| `plugins.workers` | number | Worker pool size |
| `plugins.timeout` | number | Hook timeout in milliseconds |

Fields not listed above (`title`, `baseURL`, `language`, `taxonomies`, etc.) are present in the payload for inspection but mutations have no effect.

**Return value rules:**

- Must return an object. Returning `null` or a non-object produces a build error.
- Multiple `onConfig` hooks chain in priority order — each receives the previous hook's return value.
- A timed-out hook's mutations are discarded; the next hook receives the pre-timeout value.

#### Path validation

Directory path fields and passthrough entries are validated before any are applied. If any field fails validation, the entire return value is rejected — no partial mutation.

**Rejected values for path fields:**

- Absolute paths (`/etc/shadow`, `C:\Windows`)
- `..` traversals that resolve above the project root (`../../evil`)
- `.` (current directory — would conflict with the project root)
- Empty strings
- On Windows: reserved device names (`NUL`, `CON`) and volume-relative paths

Relative paths with embedded `..` segments that resolve within the project are valid and cleaned before use (e.g., `subdir/../dist` becomes `dist`).

**Passthrough-specific rules:**

- `passthrough[N].from` follows the same rules as path fields. `from: "."` is rejected (would copy the entire project root into output).
- `passthrough[N].to` allows `"."` and `""` — these mean "root of the output directory," which is a valid destination.

Error messages include the field name and array index:

```text
onConfig: passthrough[2].from: path "../../secrets" traverses above the project root
```

### onDataFetched

Fires after external data sources are fetched and merged into site data. Modify or enrich the data.

```javascript
alloy.hook("onDataFetched", { data: ["team"] }, (data) => {
  if (data.team) {
    data.teamCount = data.team.length;
  }
  return data;
});
```

This is the primary mechanism for adding computed data that templates access via `site.data.*`.

### onPagesReady

Fires once per language batch, after the data cascade is applied but before taxonomy collection. This is the injection point for virtual pages that need to participate in taxonomies.

```javascript
alloy.hook("onPagesReady", { data: ["elements"], pages: false }, (payload) => {
  const elements = payload.siteData.elements || [];
  return {
    addPages: elements.map(el => ({
      path: `demos/${el.slug}.md`,
      url: `/demos/${el.slug}/`,
      frontMatter: { title: `${el.name} Demo`, layout: "demo", tags: [el.tagName] },
      content: `## ${el.name}\n\n${el.description}`
    }))
  };
});
```

Payload fields are `pages` and `siteData`.

**Virtual page fields:**

| Field | Required | Description |
|---|---|---|
| `path` | yes | Source-relative identifier (e.g., `demos/button.md`) |
| `url` | yes | Permalink (e.g., `/demos/button/`) |
| `frontMatter` | no | Page metadata, including taxonomy terms like `tags` |
| `content` | no | Raw markdown content (rendered through the pipeline) |
| `dependencies` | no | Project-root-relative file paths for incremental rebuild tracking |

Virtual pages flow through the full remaining pipeline: taxonomy collection, content rendering, layout resolution, and output writing.

Using `pages: false` with `{ addPages: [...] }` injects pages without round-tripping all existing pages through the plugin bridge.

#### Virtual page dependencies

During `alloy dev`, virtual pages are re-rendered on every incremental rebuild by default. Declare `dependencies` to limit that to changes in specific files:

| `dependencies` value | Incremental rebuild behavior |
|---|---|
| `["a.html", "b.css"]` | Re-render only when a listed file appears in the changed files |
| `[]` (empty array) | Never re-render — no file dependencies to invalidate |
| Omitted | Always re-render on every rebuild (default, safe fallback) |

Paths must be project-root-relative strings. Absolute paths, `..` traversals above the project root, and empty strings produce build errors.

On initial builds and for newly added virtual pages, `dependencies` has no effect — pages always render at least once before dependency filtering applies.

### onBeforeValidation

Fires before output path conflict detection. Return `{ addOutputs: { path: source } }` to register additional output paths that feed into conflict detection.

```javascript
alloy.hook("onBeforeValidation", {}, (payload) => {
  return {
    addOutputs: {
      "_redirects": "plugin:netlify-redirects",
      "_headers": "plugin:netlify-headers"
    }
  };
});
```

| Payload field | Type | Description |
|---|---|---|
| `outputPaths` | string[] | All computed page output paths |

| Return field | Type | Description |
|---|---|---|
| `addOutputs` | object | Map of additional output paths to source identifiers |

Unrecognized keys in the return value produce a build error.

### onAfterValidation

Fires after conflict detection passes. Return `{ cascade: { ... } }` to merge data into `siteData` for template rendering.

```javascript
alloy.hook("onAfterValidation", {}, (payload) => {
  return {
    cascade: {
      buildTimestamp: new Date().toISOString(),
      pageCount: payload.outputPaths.length
    }
  };
});
```

| Payload field | Type | Description |
|---|---|---|
| `outputPaths` | string[] | Validated output paths (including any added by `onBeforeValidation`) |
| `cascade` | object | Current site data cascade |

| Return field | Type | Description |
|---|---|---|
| `cascade` | object | Merged into `siteData` — keys overwrite existing values |

Returning `outputPaths` has no effect. Unrecognized keys produce a build error.

### onContentLoaded

Fires once with the full pages array. Modify `frontMatter` and `html` on existing pages. Other fields (`content`, `path`, `url`) are present for inspection but mutations are not applied back.

```javascript
alloy.hook("onContentLoaded", {
  pages: true,
  pageFields: ["frontMatter", "html", "url"]
}, (pages) => {
  pages.forEach(page => {
    if (page.frontMatter.draft) {
      page.frontMatter.noindex = true;
    }
    page.html = `<article>${page.html}</article>`;
  });
  return pages;
});
```

Changes to `html` are applied via `SetRenderedBody` — the modified HTML replaces the rendered content before layout rendering.

The return array must be the same length and order as the input. Virtual page injection is not supported here — use `onPagesReady` instead.

### onDataCascadeReady

Fires once with the full pages array after the data cascade is resolved. Each entry has `path` and `data`.

```javascript
alloy.hook("onDataCascadeReady", { pages: true }, (pages) => {
  pages.forEach(page => {
    page.data.generatedAt = new Date().toISOString();
  });
  return pages;
});
```

### onContentTransformed

Fires per page, after Markdown-to-HTML conversion but before layout rendering.

```javascript
alloy.hook("onContentTransformed", {}, (page) => {
  page.html = page.html.replace(/<img /g, '<img loading="lazy" ');
  return page;
});
```

| Field | Type | Description |
|---|---|---|
| `html` | string | Rendered content HTML |
| `frontMatter` | object | Page front matter |
| `path` | string | Source-relative file path |
| `url` | string | Page URL |
| `toc` | array | Heading structure — **omitted entirely for pages with no headings** |

Because `toc` is absent rather than empty on heading-less pages, guard before reading it:

```javascript
if (!page.toc || page.toc.length === 0) {
  page.toc = extractHeadingsFromHTML(page.html);
}
```

Return values can include `html`, `toc`, `frontMatter`, and `addDependencies`.

### onPageRendered

Fires per page, after template rendering produces the final HTML. Only `html` in the return is applied back.

```javascript
alloy.hook("onPageRendered", {}, (page) => {
  if (page.frontMatter.layout === "demo") return page;
  page.html = page.html.replace(/<h2/g, '<h2 class="styled"');
  return page;
});
```

| Field | Type | Mutable | Description |
|---|---|---|---|
| `html` | string | yes | Final rendered HTML |
| `frontMatter` | object | no | Page front matter (read-only context) |
| `url` | string | no | Page URL |
| `path` | string | no | Source-relative file path |

Return values can include `addDependencies` for incremental rebuilds:

```javascript
alloy.hook("onPageRendered", {}, (page) => {
  return {
    html: renderSSR(page.html),
    addDependencies: ["elements/rh-card/rh-card.js"],
  };
});
```

Pages whose `outputs` contains only non-HTML formats skip `onPageRendered` entirely and route through `onFormatRendered` instead.

### onFormatRendered

Fires once per non-HTML format body after layout rendering. Pages declaring non-HTML entries in `outputs` (e.g., `"json"`, `"xml"`) have each format body dispatched individually.

```javascript
alloy.hook("onFormatRendered", {}, (payload) => {
  if (payload.format === "json") {
    return { content: JSON.stringify(JSON.parse(payload.content)) };
  }
});
```

| Field | Type | Description |
|---|---|---|
| `format` | string | Output format extension (`"json"`, `"xml"`, etc.) |
| `content` | string | Rendered format body |
| `url` | string | Page URL |
| `path` | string | Source-relative file path |
| `frontMatter` | object | Page front matter (read-only context) |

**Return value:**

| Return | Effect |
|---|---|
| `{ content: "..." }` | Replaces the format body in output |
| `null` / `undefined` | Keeps the original content |
| Object without `content` key | Keeps the original content |

Only `content` is applied back. Formats fire in the order they appear in the page's `outputs` array.

**Relationship to `onPageRendered`:**

- `onPageRendered` fires only for pages whose `outputs` includes `"html"` (or defaults to it).
- `outputs: ["json"]` routes through `onFormatRendered` only.
- `outputs: ["html", "json"]` fires both hooks independently.

### onAssetProcess

Fires once per file in the assets directory during asset copy. Multiple `onAssetProcess` hooks chain — each receives the content returned by the previous.

```javascript
alloy.hook("onAssetProcess", {}, (asset) => {
  if (asset.path.endsWith('.css')) {
    return { content: minifyCSS(asset.content) };
  }
});
```

| Field | Type | Description |
|---|---|---|
| `path` | string | File path relative to the assets directory (forward slashes) |
| `content` | string | Raw file content |

**Return value:**

| Return | Effect |
|---|---|
| `{ content: "..." }` | Replaces the file content in output |
| `null` / `undefined` | Keeps the original content |
| Object without `content` key | Keeps the original content |

The `path` key in the return value is ignored — the file is always written to its original relative path. A hook error stops the build.

### onFileChanged

Fires once per file-watch batch during `alloy dev`. The payload is an array of change events, not a single path.

```javascript
alloy.hook("onFileChanged", {}, (events) => {
  const changed = events
    .filter(ev => ev.Path.startsWith("elements/") && ev.Path.endsWith(".js"))
    .map(ev => ev.Path);
  if (changed.length > 0) {
    return { invalidateByDependency: changed, restart: true };
  }
});
```

**Payload fields** (note the capitalized keys):

| Field | Type | Description |
|---|---|---|
| `Path` | string | File path relative to project root |
| `ChangeType` | number | Change category (1–8: content, layout, data, asset, static, component, passthrough, plugin) |
| `IsRemove` | boolean | `true` when the file was deleted |

**Return value:**

| Return field | Type | Description |
|---|---|---|
| `invalidateByDependency` | string[] | File paths to match against the dependency reverse index |
| `restart` | boolean | Restart Node bridge subprocesses before the rebuild |

`restart` must be a boolean. Non-boolean values are dropped with a warning.

### onBuildComplete

Fires after the build finishes. Return values are ignored.

```javascript
alloy.hook("onBuildComplete", {}, (result) => {
  console.log(`Built ${result.pageCount} pages in ${result.duration}`);
});
```

| Field | Type | Description |
|---|---|---|
| `pageCount` | number | Total pages built |
| `duration` | string | Build time as a formatted string (e.g., `"53ms"`) |
| `errors` | string[] | Build errors (empty array when the build succeeds) |
| `outputDir` | string | Output directory path |

Plugins that need page output should read from the output directory on disk — Alloy does not pipe rendered HTML to plugins over IPC.

### onDevServerStart

Fires when the dev server starts. Return values are ignored. The payload is the full site configuration object — there is no field with the server address.

```javascript
alloy.hook("onDevServerStart", {}, (config) => {
  console.log(`Dev server started for "${config.title}"`);
});
```

## Dependency Tracking

Both `onContentTransformed` and `onPageRendered` accept `addDependencies` in their return values, which drives targeted incremental rebuilds during `alloy dev`:

1. A plugin returns `addDependencies: ["path/to/file.js"]` from a per-page hook.
2. Alloy records each path in a reverse index keyed by page.
3. When a watched file changes, `onFileChanged` returns `invalidateByDependency` with the changed paths. Only pages whose reverse-index entries match are rebuilt.

Dependencies accumulate per page per build. If a plugin stops returning a path, that dependency drops from the index on the next rebuild.

Paths are normalized with `filepath.Clean` — `./data.json` and `data/../data.json` both resolve to `data.json`. Non-array `addDependencies` values produce a warning and are ignored.

## Related

- [Hook Scoping](/hooks/scoping/) -- control what data hooks receive
- [Plugin System](/plugins/) -- plugin tiers and registration
- [QuickJS Plugins](/plugins/quickjs/) -- embedded JS plugins
- [Node Plugins](/plugins/node/) -- subprocess plugins with npm access
