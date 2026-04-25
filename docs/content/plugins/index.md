---
title: "Plugins"
layout: "doc"
weight: 1
section: "plugins"
description: "Alloy's tiered plugin system — QuickJS, WASM, and Node.js plugins."
---

## Plugin Tiers

Alloy has a three-tier plugin system. Drop a file in `plugins/` and it's loaded automatically — no configuration needed.

| Tier | Runtime | Speed | File type | Use case |
|---|---|---|---|---|
| **Tier 2** | QuickJS (in-process) | ~microseconds | `.js` | Most plugins — filters, shortcodes, hooks |
| **Tier 2** | WASM (in-process) | ~nanoseconds | `.wasm` | Performance-critical filters |
| **Tier 3** | Node subprocess | ~milliseconds | `.js` with `runtime: "node"` | Plugins needing npm packages, native addons |

All tiers support filters, shortcodes, and lifecycle hooks. The difference is performance and capabilities.

## Quick Start

Create a file in `plugins/`:

```js
// plugins/word-count.js
export default function(alloy) {
    alloy.filter("wordCount", (content) => {
        return content.split(/\s+/).filter(Boolean).length;
    });
}
```

Use in templates: `{{ page.content | wordCount }}`

## Guides

- [WASM Plugins](/plugins/wasm/) — High-performance compiled plugins in Rust, TinyGo, or AssemblyScript
