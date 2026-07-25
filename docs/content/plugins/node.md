---
layout: doc
title: Node Plugins
nav_weight: 40
description: "Run plugins in a Node.js subprocess with full access to npm packages, the filesystem, and native addons."
---

Node plugins run in a real Node.js subprocess, so they can do anything a Node script can — read files, make network requests, and use any npm package, including ones with native addons.

Reach for a Node plugin when your plugin needs capabilities beyond pure computation. If it only transforms strings or data, a [QuickJS plugin](/plugins/quickjs/) runs in-process with no subprocess overhead and no Node.js requirement. See [Plugin System](/plugins/) for the full comparison.

## Prerequisites

Node plugins require:

- **Node.js** installed and available in `PATH`
- **ESM project**: `"type": "module"` in your `package.json`
- **Dependencies installed**: run `npm install` in your project root

Alloy does not ship Node.js, manage `package.json`, or run `npm install`. If Node plugins exist but `node` is not found, the build fails:

```
[alloy] ERROR Node.js not found in PATH.
        Node plugins require Node.js to be installed.
        Build aborted.
```

## Marking a Plugin as Node

Any `.js` file in `plugins/` runs on embedded QuickJS by default. To use the Node subprocess, export `runtime: "node"`:

```javascript
export const runtime = "node";
```

Without this marker, your plugin runs sandboxed on QuickJS with no system access.

## Your First Node Plugin

Setting `width` and `height` on an `<img>` prevents layout shift, but the dimensions live in the image file. This filter reads them at build time with the `image-size` package:

```bash
npm install image-size
```

```javascript
// plugins/image-dimensions.js
export const runtime = "node";

import { imageSize } from 'image-size';
import { readFileSync } from 'node:fs';
import { join } from 'node:path';

export default function(alloy) {
  alloy.filter("dimensions", (src) => {
    // Plugins run with the project root as the working directory.
    const { width, height } = imageSize(readFileSync(join('static', src)));
    return `width="${width}" height="${height}"`;
  });
}
```

Use it in a template:

{% raw %}
<wa-tab-group>
<wa-tab slot="nav" panel="nodehello-liquid" active>Liquid</wa-tab>
<wa-tab slot="nav" panel="nodehello-go">Go templates</wa-tab>

<wa-tab-panel name="nodehello-liquid" active>
<alloy-code language="liquid">&lt;img src="/hero.jpg" {{ "hero.jpg" | dimensions }} alt="Hero"&gt;</alloy-code>
</wa-tab-panel>
<wa-tab-panel name="nodehello-go">
<alloy-code language="html">&lt;img src="/hero.jpg" {{ dimensions "hero.jpg" }} alt="Hero"&gt;</alloy-code>
</wa-tab-panel>
</wa-tab-group>
{% endraw %}

This is what Node plugins are for, and it needs both halves of the runtime: an npm package to parse the image header, and filesystem access to read the file. Neither is available to a QuickJS plugin, and neither can be done from a template. Drop the `runtime = "node"` marker and this plugin stops working.

Plugins run with your project root as the working directory, so relative paths like `static/hero.jpg` resolve the way you would expect. A missing file throws — wrap the read in a `try`/`catch` if you would rather emit nothing than fail the build.

Run `alloy build`. If you add a `console.log()` inside the filter, it prints to your terminal alongside the rest of the build output.

## Registering Filters

```javascript
// plugins/smart-quotes.js
export const runtime = "node";

import smartypants from 'smartypants';

export default function(alloy) {
  alloy.filter("smartQuotes", (text) => {
    return smartypants(text, 1);
  });
}
```

{% raw %}
<wa-tab-group>
<wa-tab slot="nav" panel="nodefilt-liquid" active>Liquid</wa-tab>
<wa-tab slot="nav" panel="nodefilt-go">Go templates</wa-tab>

<wa-tab-panel name="nodefilt-liquid" active>
<alloy-code language="liquid">{{ page.content | smartQuotes }}</alloy-code>
</wa-tab-panel>
<wa-tab-panel name="nodefilt-go">
<alloy-code language="html">{{ smartQuotes .page.content }}</alloy-code>
</wa-tab-panel>
</wa-tab-group>
{% endraw %}

Filter arguments are passed as additional parameters:

```javascript
alloy.filter("imageUrl", (path, width, format) => {
  return `https://cdn.example.com/${path}?w=${width}&fmt=${format}`;
});
```

{% raw %}
<wa-tab-group>
<wa-tab slot="nav" panel="nodeimg-liquid" active>Liquid</wa-tab>
<wa-tab slot="nav" panel="nodeimg-go">Go templates</wa-tab>

<wa-tab-panel name="nodeimg-liquid" active>
<alloy-code language="liquid">{{ "hero.jpg" | imageUrl: 800, "webp" }}</alloy-code>
</wa-tab-panel>
<wa-tab-panel name="nodeimg-go">
<alloy-code language="html">{{ imageUrl "hero.jpg" 800 "webp" }}</alloy-code>
</wa-tab-panel>
</wa-tab-group>
{% endraw %}

## Registering Shortcodes

```javascript
// plugins/code-playground.js
export const runtime = "node";

import { highlight } from 'some-highlighter';

export default function(alloy) {
  // Inline shortcode
  alloy.shortcode("highlight", (args) => {
    const [code, lang] = args;
    return highlight(code, { language: lang });
  });

  // Block shortcode (receives inner content)
  alloy.shortcode("playground", (args, content) => {
    const lang = args[0] || "javascript";
    return `<div class="playground">
      <div class="code">${highlight(content, { language: lang })}</div>
      <iframe srcdoc="${content}"></iframe>
    </div>`;
  });
}
```

## Registering Hooks

Node plugins can register any lifecycle hook. `alloy.hook()` takes three arguments — name, options object, and handler:

```javascript
// plugins/css-minifier.js
export const runtime = "node";

import postcss from 'postcss';
import cssnano from 'cssnano';

export default function(alloy) {
  alloy.hook("onAssetProcess", {}, async (file) => {
    if (file.path.endsWith('.css')) {
      const result = await postcss([cssnano]).process(file.content, {
        from: file.path
      });
      return { ...file, content: result.css };
    }
    return file;
  });
}
```

The options object is required even when empty — pass `{}`. Calling `alloy.hook("onPageRendered", fn)` throws an error telling you the options object is missing. Registering the same hook name twice in one plugin logs a warning and the last registration wins.

Page hooks receive an object, not a bare string. `onPageRendered` gets `{ html, frontMatter, url, path }` — mutate what you need and return the object:

```javascript
// plugins/lit-ssr.js
export const runtime = "node";

import { render } from '@lit-labs/ssr';
import { html } from 'lit';

export default function(alloy) {
  alloy.hook("onPageRendered", { priority: 90 }, async (page) => {
    // SSR Lit components in the final HTML
    const result = render(html`${page.html}`);
    page.html = await collectResult(result);
    return page;
  });
}
```

See [Lifecycle Events](/hooks/) for every hook and its payload shape.

### Hook Priority

Control execution order with the `priority` option (default 50, lower runs first):

```javascript
// Runs first (priority 10)
alloy.hook("onPageRendered", { priority: 10 }, earlyTransform);

// Runs at default position (priority 50)
alloy.hook("onPageRendered", {}, defaultTransform);

// Runs last (priority 100)
alloy.hook("onPageRendered", { priority: 100 }, finalTransform);
```

### Hook Scoping

Declare what data your hook needs to minimize serialization overhead:

```javascript
alloy.hook("onContentLoaded", {
  data: ["navigation"],           // only these site.data keys
  pages: "/blog/**",              // only blog pages
  pageFields: ["frontMatter", "url"]  // only these fields per page
}, (pages) => {
  // Process only what you need
  return pages;
});
```

Every page and data key you ask for is serialized and sent to your plugin, so scoping is the main lever on plugin performance for large sites. See [Hook Scoping](/hooks/scoping/) for the full scoping API.

### Avoid Module State Across Per-Page Hooks

Per-page hooks are distributed across [multiple worker processes](#worker-pool). Each process loads your plugin separately, so module-level variables are **not shared** between them:

```javascript
// BROKEN — each worker process gets its own `seen` array,
// and onBuildComplete only sees one of them.
const seen = [];

export default function(alloy) {
  alloy.hook("onPageRendered", {}, (page) => {
    seen.push(page.url);        // splits across processes
    return page;
  });
  alloy.hook("onBuildComplete", {}, () => {
    console.log(seen.length);   // only this process's share
  });
}
```

This fails silently — you get partial results, not an error. To collect data across all pages, write each page's contribution to disk as you go and merge the files in `onBuildComplete`.

## Data Source Plugins

The built-in `rest` and `graphql` source types handle simple, unauthenticated, single-request fetches. For anything beyond that — authentication, pagination, retries, multi-endpoint aggregation, database access — use `type: "plugin"`. The plugin owns the entire data acquisition lifecycle and returns the final dataset. Alloy caches the result and injects it into the data cascade. For a comparison table, see [Built-in types vs plugin sources](/content/data-files/#built-in-types-vs-plugin-sources).

Register a source handler:

```javascript
// plugins/cms-posts.js
export const runtime = "node";

export default function(alloy) {
  alloy.source("cms-posts", async () => {
    const API_URL = process.env.CMS_API_URL;
    const TOKEN = process.env.CMS_TOKEN;

    let allPosts = [];
    let page = 1;
    let hasMore = true;

    while (hasMore) {
      const response = await fetch(`${API_URL}/posts?page=${page}`, {
        headers: { Authorization: `Bearer ${TOKEN}` }
      });
      const json = await response.json();
      allPosts = allPosts.concat(json.data);
      hasMore = json.meta.nextPage !== null;
      page++;
    }

    return allPosts;
  });
}
```

Configure the source in `alloy.config.yaml`:

```yaml
sources:
  blog:
    type: "plugin"
    plugin: "cms-posts"
    cache: 3600
    as: "blog"
```

The fetched data is available as `site.data.blog` in templates and can drive [virtual page generation](/hooks/) via pagination.

## Using npm Packages

The Alloy bridge script is written to `.alloy/bridge.js` in your project root. This ensures ESM `import()` resolves packages from your project's `node_modules/`. Both `import` and dynamic `import()` work:

```javascript
export const runtime = "node";

import postcss from 'postcss';              // static import
const cssnano = await import('cssnano');    // dynamic import

export default function(alloy) {
  // Both packages are available
}
```

For heavy dependencies, load them lazily inside the handler so the cost is only paid when the hook actually fires.

## Debugging

### Where Your Output Goes

Plugin `console.log`, `console.warn`, `console.info`, and `console.debug` — plus anything a dependency writes via `process.stdout.write` — are redirected to stderr and appear in **your terminal** alongside Alloy's own build output. There is no log file.

The redirect exists because stdout is reserved for the Alloy-to-plugin protocol channel. Because the bridge patches both `console.*` and `process.stdout.write` before any plugin code loads, ordinary logging cannot corrupt the protocol. See [stdout isolation](#stdout-isolation) for the mechanism and its one gap.

### Common Errors

| Symptom | Cause | Fix |
|---|---|---|
| `Node.js not found in PATH` | Node not installed or not on `PATH` | Install Node.js |
| `Cannot use import statement outside a module` | Project is not ESM | Add `"type": "module"` to `package.json` |
| `Cannot find package '...'` | Dependencies not installed | Run `npm install` in your project root |
| `expected Content-Length header, got ...` | Something wrote to stdout at the file-descriptor level | See [stdout isolation](#stdout-isolation) |
| `alloy.hook() requires options object as second argument` | Handler passed as second argument | Use `alloy.hook(name, {}, fn)` |
| `expected page object with html key, got string` | Plugin returns a bare string from a page hook | Return the page object: `page.html = ...; return page` |
| Hook produces partial results | Module state split across workers | See [module state](#avoid-module-state-across-per-page-hooks) |

## Plugin Timeout

Each plugin call respects the configured timeout (default 5000 milliseconds):

```yaml
plugins:
  timeout: 5000    # milliseconds
```

A timed-out call produces a warning and continues with unmodified data. Plugin process crashes return an error.

If a plugin legitimately needs longer — a slow API in a data source, for example — raise the timeout rather than splitting the work.

## Worker Pool

For per-page hooks (`onPageRendered`, `onFormatRendered`, `onContentTransformed`), Alloy distributes pages across multiple Node subprocess workers to parallelize the work:

```yaml
# alloy.config.yaml
plugins:
  workers: auto    # default -- auto-scale based on CPU count
  # workers: 4    # explicit override
```

Auto-scaling uses `min(CPU_count / 2, 8)` with a floor of 2. Each worker loads the same plugins via ESM `import()` so Node's module cache prevents side-effect collisions.

Only Node subprocess plugins use the worker pool — QuickJS and WASM plugins run in-process. Note the [module state caveat](#avoid-module-state-across-per-page-hooks) that comes with multiple worker processes.

## Restarting on File Changes

Return `{ restart: true }` from an `onFileChanged` hook to kill and respawn all Node bridge workers before the rebuild. Alloy re-imports every plugin file, which clears Node's ESM module cache so `import()` loads the changed code.

SSR plugins that import component definitions at startup need this. Without it, the workers keep serving stale modules.

```javascript
// plugins/element-watcher.js
export const runtime = "node";

export default function(alloy) {
  alloy.hook("onFileChanged", {}, (events) => {
    const changed = events
      .filter(ev => ev.Path.startsWith("elements/") && ev.Path.endsWith(".js"))
      .map(ev => ev.Path);
    if (changed.length > 0) {
      return { invalidateByDependency: changed, restart: true };
    }
  });
}
```

`restart` only affects Node plugins. QuickJS and WASM plugins run in-process with no subprocess state.

See [`onFileChanged`](/hooks/#onfilechanged) for the full return value API.

## Security

Node plugins run with the same permissions as the user. They have full access to:

- Filesystem (`fs`, `path`)
- Network (`fetch`, `http`, `net`)
- Environment variables (`process.env`)
- Child processes (`child_process`)

Only install plugins you have reviewed or that come from trusted sources.

## Under the Hood

You never interact with any of this directly — the `alloy` API object handles it. This section is for diagnosing unusual failures and understanding performance.

### IPC Protocol

Node plugins communicate with Alloy via length-prefixed JSON-RPC over stdin/stdout (LSP-style framing). Each message is a `Content-Length` header, a blank line, then a JSON body:

```text
Content-Length: 75\r\n
\r\n
{"id": 1, "type": "hook", "name": "onContentTransformed", "payload": [...]}
```

The header is the exact byte count of the JSON body that follows.

Length-prefixed framing is used instead of newline-delimited JSON because HTML payloads routinely contain literal newlines, which would break line-based parsing.

### stdout isolation

Stdout is reserved for the plugin protocol. The bridge script intercepts JS-level writes before any plugin code loads:

- `console.log`, `console.warn`, `console.info`, `console.debug` → stderr
- `process.stdout.write` → stderr

Plugin output and library logging appear in the terminal alongside Alloy's own output, not in a log file. Plugins cannot corrupt the protocol by logging.

**Known limitation:** A child process spawned with `stdio: 'inherit'` writes to the real stdout file descriptor, bypassing the JS-level patch. This can corrupt the protocol. Spawn children with explicit stdio instead:

```javascript
import { spawn } from 'child_process';

// Correct — child output goes to stderr, not stdout
const child = spawn('cmd', args, {
  stdio: ['ignore', 'pipe', 'pipe']
});
child.stdout.pipe(process.stderr);
```

If you see this error:

```text
plugin bridge protocol error: expected Content-Length header, got "..." —
a plugin or one of its dependencies wrote non-protocol output to stdout
```

A plugin or one of its dependencies is writing to stdout at the file descriptor level, bypassing the bridge's `process.stdout.write` patch. Common causes:

- A child process spawned with `stdio: 'inherit'`
- A native addon writing directly to fd 1
- `require('fs').writeSync(1, ...)` or similar fd-level writes

The remedy depends on which one you have. For a child process, redirect its stdio as shown above. Native addons and direct fd-level writes cannot be intercepted from JavaScript at all — you have to remove the offending call, configure the library to log to stderr instead, or move that work into a separate process whose stdout you control.

### How Hooks Are Dispatched

Per-page hooks are sent as one message per page, distributed across the worker pool — so peak memory stays proportional to a single page, not the whole site. Whole-site hooks like `onBuildComplete` run on the primary bridge only.

Because each worker is a separate process that loaded your plugin independently, per-page hooks have no shared memory. This is the mechanism behind the [module state caveat](#avoid-module-state-across-per-page-hooks).

### Serialization Cost

Every hook payload crosses a process boundary as JSON. Hooks that receive pages serialize every page they are given, on every build. [Hook scoping](#hook-scoping) is what keeps this cost proportional to what your plugin actually reads.

## Related

- [Plugin System](/plugins/) -- overview and tier comparison
- [QuickJS Plugins](/plugins/quickjs/) -- sandboxed JS plugins
- [WASM Plugins](/plugins/wasm/) -- compiled plugins for maximum performance
- [Lifecycle Events](/hooks/) -- all hook events and payloads
