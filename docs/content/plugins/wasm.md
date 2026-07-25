---
layout: doc
title: WASM Plugins
nav_weight: 30
description: "Compiled WASM plugins run sandboxed inside the Alloy process via wazero — fast filters, shortcodes, and hooks with no subprocess."
---

WASM plugins are compiled binaries that run WebAssembly instructions inside the Alloy process. They execute faster than QuickJS plugins, making them suited to filters and transforms called on every page.

```
plugins/
  word-count.wasm    # any .wasm file is loaded automatically
```

Drop a `.wasm` file in `plugins/` and Alloy loads it via wazero (pure Go, zero CGo).

## When to Use WASM

WASM plugins are worth the compilation step when:

- A filter runs on every page in a large site (thousands of calls per build)
- You need maximum throughput for data transforms
- You want a sandboxed plugin you can run untrusted

For one-off or low-frequency operations, [QuickJS plugins](/plugins/quickjs/) are simpler — no build step. For npm packages or filesystem access, use [Node plugins](/plugins/node/).

## Sandboxing

WASM plugins run in isolated memory via wazero. They cannot access the filesystem, network, or system resources. This makes them the only tier that is safe to run from untrusted sources.

## Toolchain Support

Read this before choosing a language. Alloy's ABI requires every plugin export to return **two separate `i32` values** (`ptr`, `len`). Not every toolchain can emit that, because multi-value returns require explicit support.

| Toolchain | Status |
|---|---|
| Hand-written WAT (via `wat2wasm`) | Works — `(result i32 i32)` compiles directly |
| Stable Rust (`extern "C"`) | **Cannot currently satisfy the ABI** |
| TinyGo, AssemblyScript | Depends on whether your toolchain emits multi-value returns |

Stable Rust cannot produce a conforming export by any documented route. Returning a packed `u64` (`ptr << 32 | len`) yields one return value and fails with `wasm filter ABI mismatch: expected 2 return values (ptr, len), got 1`. Returning a tuple `-> (i32, i32)` is not FFI-safe: rustc lowers the aggregate through an sret out-pointer prepended as the first parameter, so the export becomes a three-parameter function and fails with `expected 3 params, but passed 2`. Building with `-C target-feature=+multivalue` does not change this.

Whether Alloy should also accept a single packed `i64` — the form most toolchains emit naturally — is being tracked in [#1222](https://github.com/zeroedin/alloy/issues/1222). Until that is resolved, hand-written WAT is the reliable path, and the examples on this page use it.

## Your First Plugin

The smallest module that satisfies the contract. It exports `alloc` (which Alloy calls to write input into your module's memory) and one filter that returns its input unchanged — enough to prove the wiring end to end before you add an algorithm.

```wasm
;; echo.wat
(module
  (memory (export "memory") 1)

  ;; Bump allocator — starts past any data section
  (global $bump (mut i32) (i32.const 256))

  ;; alloc(size) → ptr
  (func $alloc (export "alloc") (param $size i32) (result i32)
    global.get $bump
    global.get $bump
    local.get $size
    i32.add
    global.set $bump
  )

  ;; echo(ptr, len) → (ptr, len)
  (func $echo (export "echo") (param $ptr i32) (param $len i32) (result i32 i32)
    local.get $ptr
    local.get $len
  )
)
```

Compile it into `plugins/` with [wabt](https://github.com/WebAssembly/wabt):

```bash
wat2wasm echo.wat -o plugins/echo.wasm
```

Multi-value returns are standard in current wabt releases; older versions need `--enable-multi-value`.

Then use it in a template:

{% raw %}
<wa-tab-group>
<wa-tab slot="nav" panel="wasmfirst-liquid" active>Liquid</wa-tab>
<wa-tab slot="nav" panel="wasmfirst-go">Go templates</wa-tab>

<wa-tab-panel name="wasmfirst-liquid" active>
<alloy-code language="liquid">{{ "hello" | echo }}</alloy-code>
</wa-tab-panel>
<wa-tab-panel name="wasmfirst-go">
<alloy-code language="html">{{ echo "hello" }}</alloy-code>
</wa-tab-panel>
</wa-tab-group>
{% endraw %}

Note the filter is called `echo`, not `echo.wasm` — **the template name is the exported function's name**, not the filename. See [Filters](#filters).

## Filters

Every exported function becomes a filter named after the export, except a reserved set used by the runtime itself:

`memory`, `alloc`, `last_error`, `hook`, `hooks`, `shortcode`, `_start`, `_initialize`, `__data_end`, `__heap_base`, `__stack_pointer`, `__dso_handle`, `__global_base`

So a module exporting `word_count` and `reading_time` provides two filters under those names. The module filename is not used for naming — it appears only in error messages.

A filter receives a UTF-8 string as `(ptr, len)` and returns `(ptr, len)` for the result. Input and output are raw UTF-8; the filter transforms the value and returns the transformed value.

```wasm
(func $shout (export "shout") (param $ptr i32) (param $len i32) (result i32 i32)
  ;; read input at $ptr, write result somewhere in memory,
  ;; return the result's pointer and length
  local.get $ptr
  local.get $len
)
```

## Shortcodes

Export `shortcode` to handle shortcodes. Input is a JSON object; output is a UTF-8 HTML string.

```json
{ "name": "youtube", "args": ["abc123"], "content": "" }
```

Unlike filters, the export is always named `shortcode` — the shortcode's own name arrives in the payload's `name` field, so one export handles every shortcode your module provides.

## Hooks

Export `hooks` to declare which lifecycle events you handle, and `hook` to receive them.

- **`hooks()`** — called once at module load, no input. Returns a JSON array of hook names or registration objects.
- **`hook(ptr, len)`** — input is a JSON payload with an `"event"` key. Output is the modified JSON payload.

```wasm
;; Data section holds the JSON: ["onContentTransformed"]
(data (i32.const 0) "[\"onContentTransformed\"]")

(func $hooks (export "hooks") (result i32 i32)
  i32.const 0
  i32.const 24)
```

If `hooks()` returns invalid JSON — anything that isn't an array of strings or objects — module loading fails. If `hook()` returns non-JSON bytes, the hook call returns an error. Declaring hook names without also exporting `hook` is an error.

### Hook Priority and Scope

The `hooks()` export can return a mix of strings and registration objects. Strings default to priority 50 with no scope filtering. Objects let you control execution order and limit the data payload.

```json
[
  "onBuildComplete",
  {
    "name": "onContentTransformed",
    "priority": 10,
    "pages": "blog/**",
    "data": ["navigation", "team"],
    "pageFields": ["title", "url", "tags"]
  }
]
```

Only `name` is required. All other fields are optional:

| Field | Default | Description |
|---|---|---|
| `priority` | 50 | Lower runs first. Controls order relative to other plugins. |
| `pages` | all pages | `true` (all), `false` (none), glob string, or `{"taxonomy": ["terms"]}` |
| `data` | all site data | Array of site data keys to include |
| `pageFields` | all fields | Array of per-page fields to include |

Scope filtering reduces the data serialized across the WASM memory boundary, which matters on large sites.

Taxonomy filtering (`{"taxonomy": ["terms"]}`) is only available on hooks that fire after taxonomy indices are built. Hooks like `onPagesReady` that fire before indexing reject taxonomy scope with an error — use `"pages": "blog/**"` instead. See [Lifecycle Events](/hooks/) for hook execution order.

## ABI Reference

WASM plugins run in an isolated sandbox — they can't call Alloy functions directly, and Alloy can't reach into the plugin's internals. The ABI is the contract both sides agree on: which functions the module exports, and how data crosses the boundary through linear memory using a pointer/length convention.

### Exports

| Export | Required | Signature |
|---|---|---|
| `memory` | yes | exported linear memory |
| `alloc` | yes | `alloc(size i32) -> ptr i32` |
| *your filters* | — | `name(ptr i32, len i32) -> (ptr i32, len i32)` |
| `shortcode` | no | `shortcode(ptr i32, len i32) -> (ptr i32, len i32)` |
| `hooks` | no | `hooks() -> (ptr i32, len i32)` |
| `hook` | no | `hook(ptr i32, len i32) -> (ptr i32, len i32)` |
| `last_error` | no | `last_error() -> (ptr i32, len i32)` |

Every data-returning export uses the same two-value `(ptr, len)` return. See [Toolchain Support](#toolchain-support) for which languages can express it.

### Calling Sequence

For every call from Alloy to a WASM export:

1. Alloy calls `alloc(inputLen)` to get a write offset in WASM memory
2. Alloy writes input bytes at the returned pointer
3. Alloy calls the target export (e.g., `filter(ptr, len)`)
4. The module reads input, processes it, writes the result to its own memory
5. The module returns `(resultPtr, resultLen)`
6. Alloy reads result bytes from WASM memory

### Error Handling

Returning `(0, 0)` signals an error. If the module exports `last_error()`, Alloy reads it and surfaces the message.

**Current limitation:** a filter that fails does not fail the build. Verified with a module whose export returns the wrong number of values — `alloy build` reported success and the page rendered with the filter silently skipped, passing the unfiltered input through. The error is only visible when calling the runtime directly. Tracked in [#1222](https://github.com/zeroedin/alloy/issues/1222); until it is fixed, verify a new plugin's output rather than assuming a clean build means it ran.

## Compilation Cache

Alloy caches compiled WASM modules in `.alloy/wasm-cache/` so subsequent builds skip the compilation step. The cache persists across builds.

## Performance

| Runtime | Per-call | Best For |
|---|---|---|
| QuickJS (JS) | ~10-50 microseconds | Prototyping, low-frequency filters |
| WASM (compiled) | ~1-10 microseconds | Hot-path filters on every page |
| Node (subprocess) | ~1-5 milliseconds | npm packages, system access |

## Related

- [Plugin System](/plugins/) -- overview and tier comparison
- [QuickJS Plugins](/plugins/quickjs/) -- JS plugins with no build step
- [Node Plugins](/plugins/node/) -- full Node.js access
- [Lifecycle Events](/hooks/) -- all hook events and payloads
