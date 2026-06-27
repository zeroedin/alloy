---
layout: doc
title: WASM Plugins
nav_weight: 30
description: "Compile WASM plugins from Rust, TinyGo, or AssemblyScript for filters that run 5-10x faster than QuickJS."
---

WASM plugins are compiled binaries that run native WebAssembly instructions inside the Alloy process. They execute 5-10x faster than QuickJS plugins, making them ideal for filters and transforms called on every page.

```
plugins/
  custom-slugify.wasm    # Compiled from Rust, TinyGo, or AssemblyScript
```

Drop a `.wasm` file in `plugins/` and Alloy loads it automatically via wazero (pure Go, zero CGo).

## When to Use WASM

WASM plugins are worth the compilation step when:

- A filter runs on every page in a large site (thousands of calls per build)
- You need maximum throughput for data transforms
- You prefer Rust, Go, or AssemblyScript over JavaScript

For one-off or low-frequency operations, [QuickJS plugins](/plugins/quickjs/) are simpler (no build step).

## ABI Contract

WASM plugins run in an isolated sandbox — they can't call Alloy functions directly, and Alloy can't reach into the plugin's internals. The ABI (Application Binary Interface) is the contract both sides agree on to exchange data. It defines which functions the plugin must export, which functions the host provides, and how data is passed between them through shared memory. Think of it as the narrow doorway in the sandbox wall — everything flows through it in a predictable format.

In practice, this means Alloy and your plugin communicate through linear memory using a pointer/length convention. Your module must export specific functions that Alloy calls during the build.

### Required Export: `alloc`

Your module must export an `alloc` function that returns a pointer to a block of memory. Alloy calls this to write input data into your module's linear memory before invoking any other export.

<wa-tab-group>
<wa-tab slot="nav" panel="alloc-wat" active>WAT</wa-tab>
<wa-tab slot="nav" panel="alloc-rust">Rust</wa-tab>
<wa-tab slot="nav" panel="alloc-tinygo">TinyGo</wa-tab>
<wa-tab slot="nav" panel="alloc-as">AssemblyScript</wa-tab>

<wa-tab-panel name="alloc-wat" active>
<alloy-code lang="wasm">alloc(size i32) -> ptr i32</alloy-code>
</wa-tab-panel>
<wa-tab-panel name="alloc-rust">
<alloy-code lang="rust">#[no_mangle]
pub extern "C" fn alloc(size: i32) -> i32 {
    let layout = std::alloc::Layout::from_size_align(size as usize, 1).unwrap();
    unsafe { std::alloc::alloc(layout) as i32 }
}</alloy-code>
</wa-tab-panel>
<wa-tab-panel name="alloc-tinygo">
<alloy-code lang="go">//export alloc
func alloc(size int32) int32 {
	buf := make([]byte, size)
	return int32(uintptr(unsafe.Pointer(&buf[0])))
}</alloy-code>
</wa-tab-panel>
<wa-tab-panel name="alloc-as">
<alloy-code lang="typescript">export function alloc(size: i32): i32 {
  return heap.alloc(size) as i32;
}</alloy-code>
</wa-tab-panel>
</wa-tab-group>

### Filter Export: `filter`

Receives a UTF-8 string at the given pointer/length. Returns a pointer/length pair for the result string. Input and output are raw UTF-8 — the filter transforms the value and returns the transformed value.

<wa-tab-group>
<wa-tab slot="nav" panel="filter-wat" active>WAT</wa-tab>
<wa-tab slot="nav" panel="filter-rust">Rust</wa-tab>
<wa-tab slot="nav" panel="filter-tinygo">TinyGo</wa-tab>
<wa-tab slot="nav" panel="filter-as">AssemblyScript</wa-tab>

<wa-tab-panel name="filter-wat" active>
<alloy-code lang="wasm">filter(ptr i32, len i32) -> (ptr i32, len i32)</alloy-code>
</wa-tab-panel>
<wa-tab-panel name="filter-rust">
<alloy-code lang="rust">#[no_mangle]
pub extern "C" fn filter(ptr: i32, len: i32) -> u64 {
    let input = unsafe {
        std::str::from_utf8_unchecked(
            std::slice::from_raw_parts(ptr as *const u8, len as usize)
        )
    };
    let result = input.to_uppercase();
    let result_ptr = alloc(result.len() as i32);
    unsafe {
        std::ptr::copy_nonoverlapping(result.as_ptr(), result_ptr as *mut u8, result.len());
    }
    ((result_ptr as u64) << 32) | (result.len() as u64)
}</alloy-code>
</wa-tab-panel>
<wa-tab-panel name="filter-tinygo">
<alloy-code lang="go">//export filter
func filter(ptr, length int32) uint64 {
	input := ptrToString(ptr, length)
	result := strings.ToUpper(input)
	resultPtr := alloc(int32(len(result)))
	copy(unsafe.Slice((*byte)(unsafe.Pointer(uintptr(resultPtr))), len(result)), result)
	return uint64(resultPtr)&lt;&lt;32 | uint64(len(result))
}</alloy-code>
</wa-tab-panel>
<wa-tab-panel name="filter-as">
<alloy-code lang="typescript">export function filter(ptr: i32, len: i32): u64 {
  const input = String.UTF8.decodeUnsafe(ptr, len);
  const result = input.toUpperCase();
  const resultBuf = String.UTF8.encode(result);
  const resultPtr = alloc(resultBuf.byteLength);
  memory.copy(resultPtr, changetype&lt;usize&gt;(resultBuf), resultBuf.byteLength);
  return (u64(resultPtr) &lt;&lt; 32) | u64(resultBuf.byteLength);
}</alloy-code>
</wa-tab-panel>
</wa-tab-group>

### Optional Exports

```wasm
shortcode(ptr i32, len i32) -> (ptr i32, len i32)
hooks() -> (ptr i32, len i32)
hook(ptr i32, len i32) -> (ptr i32, len i32)
last_error() -> (ptr i32, len i32)
```

- **`shortcode`**: Input is a JSON object `{ "name": "youtube", "args": ["abc123"], "content": "" }`. Output is a UTF-8 HTML string.
- **`hooks`**: Called once at module load, no input. Returns a JSON array of hook names (strings) or registration objects (see [Hook Priority and Scope](#hook-priority-and-scope)).
- **`hook`**: Input is a JSON payload with an `"event"` key. Output is the modified JSON payload.
- **`last_error`**: Called when any export returns `(0, 0)`. Returns an error message string.

### Calling Sequence

For every call from Alloy to a WASM export:

1. Alloy calls `alloc(inputLen)` to get a write offset in WASM memory
2. Alloy writes input bytes at the returned pointer
3. Alloy calls the target export (e.g., `filter(ptr, len)`)
4. The module reads input, processes it, writes the result to its own memory
5. The module returns `(resultPtr, resultLen)`
6. Alloy reads result bytes from WASM memory

### Error Handling

If any export returns `(0, 0)`, Alloy treats it as an error. If the module exports `last_error()`, Alloy reads and surfaces the error message. No silent fallback to the original input.

If `hooks()` returns invalid JSON (not an array of strings/objects), module loading fails. If `hook()` returns non-JSON bytes, the hook call returns an error.

## Rust Example

```rust
use std::alloc::{alloc, Layout};
use std::slice;
use std::str;

// Required: memory allocator for host writes
#[no_mangle]
pub extern "C" fn alloc(size: i32) -> i32 {
    let layout = Layout::from_size_align(size as usize, 1).unwrap();
    unsafe { alloc(layout) as i32 }
}

// Filter: convert text to uppercase
#[no_mangle]
pub extern "C" fn filter(ptr: i32, len: i32) -> u64 {
    let input = unsafe {
        let slice = slice::from_raw_parts(ptr as *const u8, len as usize);
        str::from_utf8(slice).unwrap()
    };

    let result = input.to_uppercase();
    let result_bytes = result.as_bytes();
    let result_ptr = alloc(result_bytes.len() as i32);

    unsafe {
        std::ptr::copy_nonoverlapping(
            result_bytes.as_ptr(),
            result_ptr as *mut u8,
            result_bytes.len(),
        );
    }

    // Pack (ptr, len) into a single i64 return value
    ((result_ptr as u64) << 32) | (result_bytes.len() as u64)
}
```

Build with:

```bash
cargo build --target wasm32-unknown-unknown --release
cp target/wasm32-unknown-unknown/release/my_filter.wasm plugins/
```

## TinyGo Example

```go
package main

import "unsafe"

// Required: memory allocator
//export alloc
func alloc(size int32) int32 {
    buf := make([]byte, size)
    return int32(uintptr(unsafe.Pointer(&buf[0])))
}

// Filter: count words in text
//export filter
func filter(ptr, length int32) (int32, int32) {
    input := ptrToString(ptr, length)
    words := 0
    inWord := false
    for _, c := range input {
        if c == ' ' || c == '\n' || c == '\t' {
            inWord = false
        } else if !inWord {
            inWord = true
            words++
        }
    }
    result := itoa(words)
    return stringToPtr(result)
}

func ptrToString(ptr, length int32) string {
    return unsafe.String((*byte)(unsafe.Pointer(uintptr(ptr))), length)
}

func stringToPtr(s string) (int32, int32) {
    buf := []byte(s)
    ptr := &buf[0]
    return int32(uintptr(unsafe.Pointer(ptr))), int32(len(buf))
}

func itoa(n int) string {
    if n == 0 {
        return "0"
    }
    s := ""
    for n > 0 {
        s = string(rune('0'+n%10)) + s
        n /= 10
    }
    return s
}

func main() {}
```

Build with:

```bash
tinygo build -o plugins/word-count.wasm -target wasi .
```

## AssemblyScript Example

```typescript
// src/word-count.ts

// Required: memory allocator
export function alloc(size: i32): i32 {
  return heap.alloc(size) as i32;
}

// Filter: count words
export function filter(ptr: i32, len: i32): u64 {
  const input = String.UTF8.decodeUnsafe(ptr, len);
  const words = input.trim().split(" ").filter(w => w.length > 0);
  const result = words.length.toString();

  const resultBuf = String.UTF8.encode(result);
  const resultPtr = alloc(resultBuf.byteLength);
  memory.copy(resultPtr, changetype<usize>(resultBuf), resultBuf.byteLength);

  return (u64(resultPtr) << 32) | u64(resultBuf.byteLength);
}
```

Build with:

```bash
asc src/word-count.ts -o plugins/word-count.wasm
```

## Hook Support

WASM modules can register lifecycle hooks by exporting `hooks()` and `hook()`:

```rust
use serde_json::{json, Value};

#[no_mangle]
pub extern "C" fn hooks() -> u64 {
    let names = json!(["onContentTransformed"]);
    let bytes = names.to_string().into_bytes();
    let ptr = alloc(bytes.len() as i32);
    unsafe {
        std::ptr::copy_nonoverlapping(
            bytes.as_ptr(), ptr as *mut u8, bytes.len()
        );
    }
    ((ptr as u64) << 32) | (bytes.len() as u64)
}

#[no_mangle]
pub extern "C" fn hook(ptr: i32, len: i32) -> u64 {
    let input = unsafe {
        let slice = std::slice::from_raw_parts(ptr as *const u8, len as usize);
        std::str::from_utf8(slice).unwrap()
    };

    let mut payload: Value = serde_json::from_str(input).unwrap();

    if payload["event"] == "onContentTransformed" {
        if let Some(html) = payload["html"].as_str() {
            let modified = html.replace("<img ", "<img loading=\"lazy\" ");
            payload["html"] = json!(modified);
        }
    }

    let result = payload.to_string().into_bytes();
    let result_ptr = alloc(result.len() as i32);
    unsafe {
        std::ptr::copy_nonoverlapping(
            result.as_ptr(), result_ptr as *mut u8, result.len()
        );
    }
    ((result_ptr as u64) << 32) | (result.len() as u64)
}
```

## Hook Priority and Scope

The `hooks()` export can return a mix of strings and registration objects. Strings default to priority 50 with no scope filtering. Objects let you control execution order and limit the data payload.

### Format

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

Only `name` is required in registration objects. All other fields are optional:

| Field | Default | Description |
|---|---|---|
| `priority` | 50 | Lower runs first. Controls order relative to other plugins. |
| `pages` | all pages | `true` (all), `false` (none), glob string, or `{"taxonomy": ["terms"]}` |
| `data` | all site data | Array of site data keys to include |
| `pageFields` | all fields | Array of per-page fields to include |

Scope filtering reduces the data serialized across the WASM memory boundary, which matters on large sites.

Taxonomy filtering (`{"taxonomy": ["terms"]}`) is only available on hooks that fire after taxonomy indices are built. Hooks like `onPagesReady` that fire before indexing reject taxonomy scope with an error — use `"pages": "blog/**"` instead. See [Lifecycle Events](/hooks/) for hook execution order.

### Rust Example

```rust
#[no_mangle]
pub extern "C" fn hooks() -> u64 {
    let hooks = serde_json::json!([
        "onBuildComplete",
        {
            "name": "onContentTransformed",
            "priority": 10,
            "pages": "blog/**",
            "data": ["navigation"]
        }
    ]);
    let bytes = hooks.to_string().into_bytes();
    let ptr = alloc(bytes.len() as i32);
    unsafe {
        std::ptr::copy_nonoverlapping(
            bytes.as_ptr(), ptr as *mut u8, bytes.len()
        );
    }
    ((ptr as u64) << 32) | (bytes.len() as u64)
}
```

## Compilation Cache

Alloy caches compiled WASM modules in `.alloy/wasm-cache/` so subsequent builds skip the compilation step. The cache persists across builds.

## Sandboxing

WASM plugins run in isolated memory via wazero. They cannot access the filesystem, network, or system resources. Safe to run untrusted community plugins.

## Performance Comparison

| Runtime | Per-call | Best For |
|---|---|---|
| QuickJS (JS) | ~10-50 microseconds | Prototyping, low-frequency filters |
| WASM (compiled) | ~1-10 microseconds | Hot-path filters on every page |
| Node (Tier 3) | ~1-5 milliseconds | npm packages, system access |

## Related

- [Plugin System](/plugins/) -- overview and tier comparison
- [QuickJS Plugins](/plugins/quickjs/) -- JS plugins with no build step
- [Node Plugins](/plugins/node/) -- full Node.js access
- [Lifecycle Events](/hooks/) -- all hook events and payloads
