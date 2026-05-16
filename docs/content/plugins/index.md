---
layout: doc
title: Plugin System
---

Alloy plugins extend the build pipeline with custom filters, shortcodes, and lifecycle hooks. The plugin system is organized into three tiers, each trading sandboxing for capability.

```
plugins/
├── word-count.js        # Tier 2: QuickJS (microseconds)
├── image-opt.wasm       # Tier 2: WASM (microseconds)
└── postcss-minify.js    # Tier 3: Node (milliseconds, export runtime = "node")
```

Drop files into `plugins/` and they are discovered automatically. No configuration needed.

## Three tiers

| Tier | Runtime | Latency | Sandbox | Use case |
|---|---|---|---|---|
| **1. Go built-in** | Compiled into binary | Nanoseconds | N/A | Core filters and tags shipped with Alloy |
| **2. In-process** | QuickJS (.js) or WASM (.wasm) | ~1-50 microseconds | Full sandbox, no system access | Custom filters, shortcodes, computed data |
| **3. Node subprocess** | Node.js (.js + `runtime: "node"`) | Milliseconds | None -- full system access | npm packages, native addons, Node APIs |

### Tier 1: Go built-in filters

Built-in filters like `sort`, `where`, `slugify`, and `date` are compiled into the Alloy binary. They run at native speed with zero overhead. You cannot add Tier 1 plugins -- they are part of the Alloy source code.

### Tier 2: In-process plugins

Both QuickJS and WASM plugins run inside [wazero](https://wazero.io/), a pure-Go WebAssembly runtime with no CGo dependency. They execute in-process with strict sandboxing: no filesystem access, no network, no system calls.

- **QuickJS plugins** are plain `.js` files. No build step, no bundler. ~10-50 microsecond per call, ~10-50ms startup. See [QuickJS Plugins](/plugins/quickjs/).
- **WASM plugins** are `.wasm` binaries compiled from Rust, TinyGo, or AssemblyScript. ~1-10 microsecond per call. See [WASM Plugins](/plugins/wasm/).

### Tier 3: Node subprocess

A `.js` file that exports `runtime: "node"` runs as a Node.js subprocess. This gives you the full npm ecosystem, native addons, and Node APIs at the cost of millisecond-level IPC overhead and no sandboxing.

Alloy does not ship Node. You bring your own installation. See [Node Plugins](/plugins/node/).

## When to use each tier

**Start with QuickJS** for most custom filters and shortcodes. It is fast, requires no build tooling, and runs sandboxed. If your plugin is a pure data transformation -- string manipulation, date formatting, computed fields -- QuickJS is the right choice.

**Use WASM** when you need maximum per-call performance or want to write in Rust, TinyGo, or AssemblyScript. The ABI is minimal but the compile step adds friction. Best for hot-path filters called thousands of times per build.

**Use Node** when you need npm packages (PostCSS, Prettier, markdown-it plugins), native addons (sharp for image processing), or Node APIs (fetch, fs for external data). Accept the overhead -- it is still fast enough for most builds.

## Plugin discovery

Alloy scans the `plugins/` directory at startup. Files are matched by extension:

| Extension | Runtime |
|---|---|
| `.js` | QuickJS (Tier 2) by default |
| `.js` with `export const runtime = "node"` | Node (Tier 3) |
| `.wasm` | WASM (Tier 2) |

No registration in config is required. Add a file, restart the build, and it is active.

## Plugin load order

Plugins load in a deterministic order:

1. **Tier 1** -- Go built-in filters (always first)
2. **Tier 2** -- In-process plugins (QuickJS and WASM)
3. **Tier 3** -- Node subprocess plugins

Within each tier, plugins load in **alphabetical order** by filename.

## Name conflict resolution

If two plugins register a filter or shortcode with the same name, the **last one loaded wins** and Alloy emits a warning:

```
WARN: filter "minify" registered by plugins/minify-html.js overrides registration from plugins/clean.wasm
```

Since plugins load alphabetically within tiers and tiers load in order, a Tier 3 Node plugin always overrides a Tier 2 QuickJS plugin with the same filter name. Rename your files or consolidate registrations to avoid conflicts.

## Plugin API

All plugin tiers share a common API surface through the `alloy` object:

| Method | Description |
|---|---|
| `alloy.filter(name, fn)` | Register a template filter |
| `alloy.shortcode(name, fn)` | Register a shortcode tag |
| `alloy.hook(event, options, fn)` | Register a lifecycle hook |
| `alloy.on(event, options, fn)` | Alias for `alloy.hook()` |
| `alloy.data` | Read-only snapshot of site data |

See the individual plugin tier pages for API details and examples specific to each runtime.
