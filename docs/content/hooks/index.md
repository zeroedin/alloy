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

Fires after config is loaded but before the build starts. The hook receives the full configuration object and must return it. Only fields on the mutable allowlist are applied back — all other fields are silently ignored.

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

##### Path validation

Directory path fields (`build.output`, `structure.content`, `structure.layouts`, `structure.assets`, `structure.static`, `structure.data`) and passthrough entries are validated before any are applied to the config. If any field fails validation, the entire return value is rejected — no partial mutation.

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

Error messages include the field name and array index for passthrough entries:

```
onConfig: passthrough[2].from: path "../../secrets" traverses above the project root
```

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

Fires before output path conflict detection. The payload contains all computed output paths. Return `{ addOutputs: { path: source } }` to register additional output paths that feed into conflict detection.

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

Unrecognized keys in the return value produce a build error. Each plugin runs independently via `RunEachWithTimeout` — plugins do not see each other's additions.

#### onAfterValidation

Fires after conflict detection passes. The payload includes the validated output paths and the site data cascade. Return `{ cascade: { ... } }` to merge data into `siteData` for template rendering.

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

Returning `outputPaths` in the return value has no effect. Unrecognized keys produce a build error. Each plugin runs independently.

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
| `dependencies` | no | Array of project-root-relative file paths for incremental rebuild tracking |

Virtual pages injected here flow through the full remaining pipeline: taxonomy collection, content rendering, layout resolution, and output writing.

When using `pages: false` in the options, return `{ addPages: [...] }` to inject pages without round-tripping all existing pages through the plugin bridge.

##### Virtual page dependencies

During `alloy dev`, virtual pages are re-rendered on every incremental rebuild by default. Declare `dependencies` to tell Alloy which source files a virtual page depends on — it will only re-render when one of those files changes.

```javascript
alloy.hook("onPagesReady", { data: ["elements"], pages: false }, (payload) => {
  const elements = payload.siteData.elements || [];
  return {
    addPages: elements.map(el => ({
      path: `demos/${el.slug}.html`,
      url: `/demos/${el.slug}/`,
      dependencies: [`elements/${el.slug}/demo/index.html`],
      frontMatter: { layout: "demo", markdown: false },
      content: fs.readFileSync(`elements/${el.slug}/demo/index.html`, "utf-8")
    }))
  };
});
```

| `dependencies` value | Incremental rebuild behavior |
|---|---|
| `["a.html", "b.css"]` | Re-render only when a listed file appears in the changed files |
| `[]` (empty array) | Never re-render — no file dependencies to invalidate |
| Omitted | Always re-render on every rebuild (default, safe fallback) |

Paths must be project-root-relative strings. Absolute paths, `..` traversals above the project root, and empty strings produce build errors.

On initial builds and for newly added virtual pages, `dependencies` has no effect — pages always render at least once before dependency filtering applies.

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

**Mutating html:**

```javascript
alloy.hook("onContentLoaded", {
  pages: true,
  pageFields: ["html", "url"]
}, (pages) => {
  pages.forEach(page => {
    page.html = `<article>${page.html}</article>`;
  });
  return pages;
});
```

Both `frontMatter` and `html` can be mutated in the same hook call. Changes to `html` are applied via `SetRenderedBody` — the modified HTML replaces the rendered content for that page before layout rendering.

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

#### onAssetProcess

Fires once per file in the assets directory during asset copy. Each invocation receives a single file's path and content. Multiple `onAssetProcess` hooks chain — each receives the content returned by the previous hook.

```javascript
alloy.hook("onAssetProcess", {}, (asset) => {
  if (asset.path.endsWith('.css')) {
    return { content: minifyCSS(asset.content) };
  }
  // Return null or omit content key to keep the original
});
```

| Field | Type | Description |
|---|---|---|
| `path` | string | File path relative to the assets directory (forward slashes, e.g., `css/main.css`) |
| `content` | string | Raw file content |

**Return value:**

| Return | Effect |
|---|---|
| `{ content: "..." }` | Replaces the file content in output |
| `null` / `undefined` | Keeps the original content |
| Object without `content` key | Keeps the original content |

The `path` key in the return value is ignored — the file is always written to its original relative path in the output directory. A hook error stops the build.

### Read-Only Hooks

Return values are ignored. Plugins observe but cannot modify.

#### onBuildComplete

Fires after the build finishes. The payload uses PascalCase keys (the `BuildResult` struct has no JSON tag overrides). `Duration` is raw nanoseconds — divide by `1e6` for milliseconds.

```javascript
alloy.hook("onBuildComplete", {}, (result) => {
  const ms = (result.Duration / 1e6).toFixed(0);
  console.log(`Built ${result.PageCount} pages in ${ms}ms`);
});
```

| Field | Type | Description |
|---|---|---|
| `OutputDir` | string | Output directory path |
| `PageCount` | number | Total pages built |
| `PagesSkipped` | number | Pages skipped during incremental rebuilds |
| `Duration` | number | Build time in nanoseconds |

#### onDevServerStart

Fires when the dev server starts. The payload is the full site configuration object — there is no `url` field with the server address.

```javascript
alloy.hook("onDevServerStart", {}, (config) => {
  console.log(`Dev server started for "${config.Title}"`);
});
```

#### onFileChanged

Fires once per file-watch batch during `alloy dev`. The payload is an array of change events, not a single file path.

```javascript
alloy.hook("onFileChanged", {}, (events) => {
  for (const event of events) {
    console.log(`${event.Path} changed (removed: ${event.IsRemove})`);
  }
});
```

| Field | Type | Description |
|---|---|---|
| `Path` | string | File path relative to project root |
| `ChangeType` | number | Change category (1–8: content, template, data, asset, etc.) |
| `IsRemove` | boolean | `true` when the file was deleted |

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
