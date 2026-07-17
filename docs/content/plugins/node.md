---
layout: doc
title: Node Plugins
nav_weight: 40
description: "Run plugins in a Node.js subprocess with full access to npm packages, the filesystem, and native addons."
---

Node plugins run in a subprocess with full access to the Node.js runtime, npm packages, and native addons. Use them when your plugin needs capabilities beyond pure computation -- filesystem access, network requests, or npm dependencies.

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

## Marking a Plugin as Node

Any `.js` file in `plugins/` runs on embedded QuickJS by default. To use the Node subprocess, export `runtime: "node"`:

```javascript
export const runtime = "node";
```

Without this marker, your plugin runs sandboxed on QuickJS with no system access.

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

## IPC Protocol

Node plugins communicate with Alloy via length-prefixed JSON-RPC over stdin/stdout (LSP-style framing).

```
Content-Length: 82\r\n
\r\n
{"id": 1, "type": "hook", "name": "onContentTransformed", "payload": [...]}
```

You never interact with this protocol directly — the `alloy` API object handles serialization.

### stdout isolation

Stdout is reserved for the plugin protocol. The bridge script intercepts all in-process writes before any plugin code loads:

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

### Troubleshooting

If you see this error:

```
plugin bridge protocol error: expected Content-Length header, got "..." —
a plugin or one of its dependencies wrote non-protocol output to stdout
```

A plugin or one of its dependencies is writing to stdout at the file descriptor level, bypassing the bridge's `process.stdout.write` patch. Common causes:

- A child process spawned with `stdio: 'inherit'`
- A native addon writing directly to fd 1
- `require('fs').writeSync(1, ...)` or similar fd-level writes

Fix by redirecting the child's stdio as shown above.

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
<alloy-code lang="liquid">{{ page.content | smartQuotes }}</alloy-code>
</wa-tab-panel>
<wa-tab-panel name="nodefilt-go">
<alloy-code lang="html">{{ smartQuotes .page.content }}</alloy-code>
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
<alloy-code lang="liquid">{{ "hero.jpg" | imageUrl: 800, "webp" }}</alloy-code>
</wa-tab-panel>
<wa-tab-panel name="nodeimg-go">
<alloy-code lang="html">{{ imageUrl "hero.jpg" 800 "webp" }}</alloy-code>
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

Node plugins can register any lifecycle hook:

```javascript
// plugins/lit-ssr.js
export const runtime = "node";

import { render } from '@lit-labs/ssr';
import { html } from 'lit';

export default function(alloy) {
  alloy.hook("onPageRendered", { priority: 90 }, async (pageHtml) => {
    // SSR Lit components in the final HTML
    const result = render(html`${pageHtml}`);
    return collectResult(result);
  });
}
```

### Hook Priority

Control execution order with the `priority` option:

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

See [Hook Scoping](/hooks/scoping/) for the full scoping API.

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

## Worker Pool

For per-page hooks (`onPageRendered`, `onContentTransformed`), Alloy distributes pages across multiple Node subprocess workers to parallelize the work:

```yaml
# alloy.config.yaml
plugins:
  workers: auto    # default -- auto-scale based on CPU count
  # workers: 4    # explicit override
```

Auto-scaling uses `min(CPU_count / 2, 8)` with a floor of 2. Each worker loads the same plugins via ESM `import()` so Node's module cache prevents side-effect collisions.

Only Tier 3 (Node subprocess) plugins use the worker pool -- Tier 2 plugins run in-process.

## Module Resolution

The Alloy bridge script is written to `.alloy/bridge.js` in your project root. This ensures ESM `import()` resolves packages from your project's `node_modules/`. Both `import` and dynamic `import()` work:

```javascript
export const runtime = "node";

import postcss from 'postcss';              // static import
const cssnano = await import('cssnano');    // dynamic import

export default function(alloy) {
  // Both packages are available
}
```

## Plugin Timeout

Each plugin call respects the configured timeout (default 5 seconds):

```yaml
plugins:
  timeout: 5000    # milliseconds
```

A timed-out call produces a warning and continues with unmodified data. Plugin process crashes return an error.

## Security

Node plugins run with the same permissions as the user. They have full access to:

- Filesystem (`fs`, `path`)
- Network (`fetch`, `http`, `net`)
- Environment variables (`process.env`)
- Child processes (`child_process`)

Only install plugins you have reviewed or that come from trusted sources.

## Related

- [Plugin System](/plugins/) -- overview and tier comparison
- [QuickJS Plugins](/plugins/quickjs/) -- sandboxed JS plugins
- [WASM Plugins](/plugins/wasm/) -- compiled plugins for maximum performance
- [Lifecycle Events](/hooks/) -- all hook events and payloads
