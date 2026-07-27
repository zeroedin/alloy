---
layout: doc
title: WASM Plugins
nav_weight: 30
description: "Compiled WASM plugins run sandboxed inside the Alloy process via wazero — fast filters, shortcodes, and hooks with no subprocess."
---

WASM plugins are compiled binaries that run WebAssembly instructions inside the Alloy process. They execute faster than QuickJS plugins, making them suited to filters and transforms called on every page.

```text
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

WASM plugins run in isolated memory via wazero. They cannot access the filesystem, network, or system resources — wazero provides no host function imports, so modules have no way to reach outside their own linear memory. This makes WASM the most isolated plugin tier, though Alloy does not currently enforce execution time or memory limits.

## Toolchain Support

Any language that compiles to `wasm32-unknown-unknown` and can emit a packed `i64` return works. The three tested toolchains:

| Toolchain | Build target | Notes |
|---|---|---|
| **Rust** | `wasm32-unknown-unknown` | `cargo build --target wasm32-unknown-unknown --release` |
| **TinyGo** | `wasm-unknown` | Use `-target wasm-unknown`, not `-target wasi` (WASI imports are not provided) |
| **AssemblyScript** | default | Build with `--runtime stub --use abort=` (Alloy does not provide `abort`) |

WAT (WebAssembly Text Format) also works for small plugins. Compile with [wabt](https://github.com/WebAssembly/wabt): `wat2wasm plugin.wat -o plugins/plugin.wasm`.

## Your First Plugin

Start with the smallest module that satisfies the [ABI contract](#abi-reference). Every module must export `memory` and `alloc`. Data-returning exports (`filter`, `shortcode`, `hook`, `hooks`, `last_error`) return a single packed `i64` encoding a pointer and length: `result = (ptr << 32) | len`.

Here's an echo filter — it returns its input unchanged, enough to prove the wiring works end to end before you add real logic.

<wa-tab-group>
<wa-tab slot="nav" panel="first-rust" active>Rust</wa-tab>
<wa-tab slot="nav" panel="first-tinygo">TinyGo</wa-tab>
<wa-tab slot="nav" panel="first-assemblyscript">AssemblyScript</wa-tab>
<wa-tab slot="nav" panel="first-wat">WAT</wa-tab>

<wa-tab-panel name="first-rust" active>

<alloy-code language="rust" filename="src/lib.rs">use std::alloc::{alloc as alloc_bytes, Layout};

#[no_mangle]
pub extern "C" fn alloc(size: i32) -> i32 {
    let layout = Layout::from_size_align(size as usize, 1).unwrap();
    unsafe { alloc_bytes(layout) as i32 }
}

#[no_mangle]
pub extern "C" fn echo(ptr: i32, len: i32) -> u64 {
    // Return input unchanged: pack ptr and len into a single i64
    ((ptr as u64) << 32) | (len as u64)
}
</alloy-code>

<alloy-code language="toml" filename="Cargo.toml">[lib]
crate-type = ["cdylib"]
</alloy-code>

```bash
cargo build --target wasm32-unknown-unknown --release
cp target/wasm32-unknown-unknown/release/echo.wasm plugins/
```

</wa-tab-panel>
<wa-tab-panel name="first-tinygo">

<alloy-code language="go" filename="main.go">package main

import "unsafe"

//export alloc
func alloc(size int32) int32 {
	if size == 0 {
		return 0
	}
	buf := make([]byte, size)
	return int32(uintptr(unsafe.Pointer(&buf[0])))
}

//export echo
func echo(ptr, length int32) uint64 {
	return (uint64(ptr) << 32) | uint64(length)
}

func main() {}
</alloy-code>

```bash
tinygo build -o plugins/echo.wasm -target wasm-unknown .
```

</wa-tab-panel>
<wa-tab-panel name="first-assemblyscript">

<alloy-code language="typescript" filename="src/echo.ts">export function alloc(size: i32): i32 {
  return heap.alloc(size) as i32;
}

export function echo(ptr: i32, len: i32): u64 {
  return (u64(ptr) << 32) | u64(len);
}
</alloy-code>

```bash
asc src/echo.ts -o plugins/echo.wasm --runtime stub --use abort=
```

</wa-tab-panel>
<wa-tab-panel name="first-wat">

<alloy-code language="wasm" filename="echo.wat">(module
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

  ;; echo(ptr, len) → packed i64
  (func $echo (export "echo") (param $ptr i32) (param $len i32) (result i64)
    local.get $ptr
    i64.extend_i32_u
    i64.const 32
    i64.shl
    local.get $len
    i64.extend_i32_u
    i64.or
  )
)
</alloy-code>

```bash
wat2wasm echo.wat -o plugins/echo.wasm
```

</wa-tab-panel>
</wa-tab-group>

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

The filter is called `echo`, not `echo.wasm` — **the template name is the exported function's name**, not the filename. See [Filters](#filters).

## Filters

Every exported function becomes a filter named after the export, except a reserved set used by the runtime itself:

`memory`, `alloc`, `last_error`, `hook`, `hooks`, `shortcode`, `_start`, `_initialize`, `__data_end`, `__heap_base`, `__stack_pointer`, `__dso_handle`, `__global_base`

A module exporting `word_count` and `reading_time` provides two filters under those names. The module filename is not used for naming — it appears only in error messages.

A filter receives a UTF-8 string as `(ptr, len)` and returns a packed `i64` for the result. Input and output are raw UTF-8.

<wa-tab-group>
<wa-tab slot="nav" panel="filter-rust" active>Rust</wa-tab>
<wa-tab slot="nav" panel="filter-tinygo">TinyGo</wa-tab>
<wa-tab slot="nav" panel="filter-wat">WAT</wa-tab>

<wa-tab-panel name="filter-rust" active>

<alloy-code language="rust">#[no_mangle]
pub extern "C" fn shout(ptr: i32, len: i32) -> u64 {
    let input = unsafe {
        std::slice::from_raw_parts(ptr as *const u8, len as usize)
    };
    let upper = std::str::from_utf8(input)
        .unwrap_or_default()
        .to_uppercase();
    let bytes = upper.into_bytes();
    let result_ptr = bytes.as_ptr() as u64;
    let result_len = bytes.len() as u64;
    std::mem::forget(bytes);
    (result_ptr << 32) | result_len
}
</alloy-code>

</wa-tab-panel>
<wa-tab-panel name="filter-tinygo">

<alloy-code language="go">import (
	"strings"
	"unsafe"
)

//export shout
func shout(ptr, length int32) uint64 {
	if length == 0 {
		return 0
	}
	input := unsafe.String((*byte)(unsafe.Pointer(uintptr(ptr))), length)
	result := strings.ToUpper(input)
	buf := []byte(result)
	resultPtr := uint64(uintptr(unsafe.Pointer(&buf[0])))
	resultLen := uint64(len(buf))
	return (resultPtr << 32) | resultLen
}
</alloy-code>

</wa-tab-panel>
<wa-tab-panel name="filter-wat">

<alloy-code language="wasm">;; echo filter — returns input unchanged
(func $shout (export "shout") (param $ptr i32) (param $len i32) (result i64)
  local.get $ptr
  i64.extend_i32_u
  i64.const 32
  i64.shl
  local.get $len
  i64.extend_i32_u
  i64.or
)
</alloy-code>

</wa-tab-panel>
</wa-tab-group>

A filter that fails should return `0` (packed `i64` with both ptr and len zero). See [Error Handling](#error-handling).

## Shortcodes

Export `shortcode` to handle shortcodes. Input is a JSON object; output is a UTF-8 HTML string.

```json
{ "name": "youtube", "args": ["abc123"], "content": "" }
```

Unlike filters, the export is always named `shortcode` — the shortcode's own name arrives in the payload's `name` field, so one export handles every shortcode your module provides.

## Hooks

Export `hooks` to declare which lifecycle events you handle, and `hook` to receive them.

- **`hooks()`** — called once at module load, no input. Returns a packed `i64` pointing to a JSON array of hook names or registration objects.
- **`hook(ptr, len)`** — input is an envelope wrapping the hook payload, so a single export can serve every event you registered:

  ```json
  { "event": "onContentTransformed", "payload": { "html": "…", "url": "…" } }
  ```

  Read `event` to decide what to do, and find the hook's own fields under `payload` — not at the top level. Return a packed `i64` pointing to the modified payload. Returning the whole envelope also works; Alloy unwraps it.

<wa-tab-group>
<wa-tab slot="nav" panel="hooks-rust" active>Rust</wa-tab>
<wa-tab slot="nav" panel="hooks-wat">WAT</wa-tab>

<wa-tab-panel name="hooks-rust" active>

<alloy-code language="rust">static HOOKS_JSON: &[u8] = b"[\"onContentTransformed\"]";

#[no_mangle]
pub extern "C" fn hooks() -> u64 {
    let ptr = HOOKS_JSON.as_ptr() as u64;
    let len = HOOKS_JSON.len() as u64;
    (ptr << 32) | len
}

#[no_mangle]
pub extern "C" fn hook(ptr: i32, len: i32) -> u64 {
    // Echo the envelope back unchanged — Alloy unwraps it,
    // so the page is left as-is. Parse it to modify the payload.
    ((ptr as u64) << 32) | (len as u64)
}
</alloy-code>

</wa-tab-panel>
<wa-tab-panel name="hooks-wat">

<alloy-code language="wasm">;; Data section holds the JSON: ["onContentTransformed"]
(data (i32.const 0) "[\"onContentTransformed\"]")

;; hooks() → packed i64 pointing to the JSON array
(func $hooks (export "hooks") (result i64)
  i32.const 0
  i64.extend_i32_u
  i64.const 32
  i64.shl
  i32.const 24
  i64.extend_i32_u
  i64.or
)

;; hook(ptr, len) → packed i64
(func $hook (export "hook") (param $ptr i32) (param $len i32) (result i64)
  local.get $ptr
  i64.extend_i32_u
  i64.const 32
  i64.shl
  local.get $len
  i64.extend_i32_u
  i64.or
)
</alloy-code>

</wa-tab-panel>
</wa-tab-group>

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

WASM plugins run in an isolated sandbox — they can't call Alloy functions directly, and Alloy can't reach into the plugin's internals. The ABI is the contract both sides agree on: which functions the module exports, and how data crosses the boundary through linear memory.

All data-returning exports return a single packed `i64`:

```text
result = (ptr << 32) | len
```

The upper 32 bits are the pointer into linear memory. The lower 32 bits are the byte length. Alloy reads `len` bytes starting at `ptr` to get the result.

### Exports

| Export | Required | Signature |
|---|---|---|
| `memory` | yes | exported linear memory |
| `alloc` | yes | `alloc(size i32) -> i32` |
| *your filters* | — | `name(ptr i32, len i32) -> i64` |
| `shortcode` | no | `shortcode(ptr i32, len i32) -> i64` |
| `hooks` | no | `hooks() -> i64` |
| `hook` | no | `hook(ptr i32, len i32) -> i64` |
| `last_error` | no | `last_error() -> i64` |

Alloy validates every data-returning export's signature at load time. Modules using the old multi-value ABI (`(result i32 i32)`) or the sret form (`(param i32 i32 i32) -> ()` produced by some Rust and TinyGo configurations) are rejected with an error naming the export and its actual signature.

### Calling Sequence

For every call from Alloy to a WASM export:

1. Alloy calls `alloc(inputLen)` to get a write offset in WASM memory
2. Alloy writes input bytes at the returned pointer
3. Alloy calls the target export (e.g., `filter(ptr, len)`)
4. The module reads input, processes it, writes the result to its own memory
5. The module returns a packed `i64`: `(resultPtr << 32) | resultLen`
6. Alloy unpacks the `i64` and reads result bytes from WASM memory

### Error Handling

Return `0` (a packed `i64` where both ptr and len are zero) to signal an error. If the module exports `last_error()`, Alloy calls it and surfaces the returned message. Otherwise, Alloy reports a generic error.

A filter that throws or traps fails the build with the filter name and source file in the error message.

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
