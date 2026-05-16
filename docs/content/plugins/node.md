---
layout: doc
title: Node Plugins
---

Node plugins run as subprocesses with full access to npm packages, native addons, and Node APIs. Mark a `.js` file as a Node plugin by exporting `runtime: "node"`:

```javascript
// plugins/css-minify.js
export const runtime = "node";

import postcss from "postcss";
import cssnano from "cssnano";

export default function(alloy) {
  alloy.filter("cssmin", async (css) => {
    const result = await postcss([cssnano]).process(css, { from: undefined });
    return result.css;
  });
}
```

```liquid
<style>{{ page.inlineCSS | cssmin }}</style>
```

## Requirements

**ESM only.** Node plugins must use ES module syntax (`import`/`export`). Your project needs `"type": "module"` in `package.json`, or use the `.mjs` extension.

**Bring your own Node.** Alloy does not ship a Node.js runtime. Node must be installed and available on `PATH`. If Node is not found, Tier 3 plugins are skipped with a warning.

## The runtime export

A `.js` file in `plugins/` runs on QuickJS by default. To opt into Node, export the `runtime` constant:

```javascript
export const runtime = "node";
```

This is how Alloy distinguishes QuickJS plugins from Node plugins. Without this export, the file runs sandboxed in QuickJS.

## JSON-RPC protocol

Node plugins communicate with Alloy over stdin/stdout using JSON-RPC with LSP-style framing (Content-Length headers). You do not interact with this protocol directly -- the `alloy` API object handles serialization. The protocol detail matters only if you are debugging or building alternative runtimes.

Messages flow bidirectionally:

```
Alloy → Node:  Content-Length: 82\r\n\r\n{"jsonrpc":"2.0","method":"filter","params":{"name":"cssmin","input":"..."},"id":1}
Node  → Alloy: Content-Length: 45\r\n\r\n{"jsonrpc":"2.0","result":"...output...","id":1}
```

Stderr is captured for logging. Do not write to stdout directly -- it will corrupt the RPC stream.

## Plugin API

The default export receives the `alloy` API object:

```javascript
export const runtime = "node";

export default function(alloy) {
  // Register filters, shortcodes, and hooks
  alloy.filter("name", fn);
  alloy.shortcode("name", fn);
  alloy.hook("event", options, fn);

  // Read-only site data
  console.error(alloy.data.site.title);
}
```

## Worker pool for per-page hooks

Per-page hooks (`onContentTransformed`, `onPageRendered`) fire once for every page. To avoid serial bottlenecks, Alloy runs these hooks through a worker pool that auto-scales based on available CPUs:

| Setting | Value |
|---|---|
| Workers | `NumCPU / 2`, capped at 8 |
| Scaling | Automatic |
| Scope | Per-page hooks only |

Batch hooks (`onPagesReady`, `onBuildComplete`, etc.) run on a single worker since they process aggregate data.

## Working directory and module resolution

The Node subprocess runs with `cwd` set to the **project root** (the directory containing `alloy.config.yaml`). This means `node_modules/` resolves naturally:

```javascript
export const runtime = "node";

// Resolves from project root node_modules/
import sharp from "sharp";
import { unified } from "unified";
```

Install dependencies in your project root as usual:

```bash
npm install sharp cssnano postcss
```

## Security model

Node plugins run with the same permissions as the user who invoked Alloy. They can:

- Read and write files anywhere on the filesystem
- Make network requests
- Execute child processes
- Access environment variables

This is the same trust model as 11ty, Jekyll, and other SSGs with plugin systems. Only run Node plugins from sources you trust.

## Example: CSS minification with PostCSS

```javascript
// plugins/postcss-minify.js
export const runtime = "node";

import postcss from "postcss";
import cssnano from "cssnano";
import autoprefixer from "autoprefixer";

export default function(alloy) {
  alloy.hook("onAssetProcess", { priority: 50 }, async (asset) => {
    if (!asset.path.endsWith(".css")) return asset;

    const result = await postcss([
      autoprefixer,
      cssnano({ preset: "default" }),
    ]).process(asset.content, {
      from: asset.path,
    });

    asset.content = result.css;
    return asset;
  });
}
```

## Example: CMS data source

```javascript
// plugins/cms-data.js
export const runtime = "node";

export default function(alloy) {
  alloy.hook("onDataFetched", { priority: 10, data: ["cms"] }, async (data) => {
    const response = await fetch("https://api.example.com/posts", {
      headers: { Authorization: `Bearer ${process.env.CMS_TOKEN}` },
    });
    const posts = await response.json();

    data.cms = { posts };
    return data;
  });
}
```

The fetched data is available in templates as `site.data.cms.posts`:

```liquid
{% for post in site.data.cms.posts %}
  <article>
    <h2>{{ post.title }}</h2>
    {{ post.body | markdownify }}
  </article>
{% endfor %}
```

## Debugging

Since Node plugins communicate over stdin/stdout, use `console.error()` for debug output -- it goes to stderr and appears in Alloy's log output without corrupting the RPC stream:

```javascript
export const runtime = "node";

export default function(alloy) {
  alloy.filter("debug_filter", (input) => {
    console.error("debug_filter called with:", input.substring(0, 100));
    return input;
  });
}
```
