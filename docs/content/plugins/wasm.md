---
title: "WASM Plugins"
layout: "doc"
weight: 3
section: "plugins"
description: "Write high-performance Alloy plugins in Rust, TinyGo, or AssemblyScript using the WASM ABI."
---

## Overview

WASM plugins are compiled to WebAssembly and run at near-native speed inside Alloy's wazero runtime. They're ~5-10x faster than QuickJS plugins — worth it for filters or shortcodes called thousands of times per build.

Drop a `.wasm` file in `plugins/` and it's loaded automatically. No configuration needed.

## Supported Languages

| Language | Compiler | Build command |
|---|---|---|
| Rust | wasm-pack | `wasm-pack build --target bundler -d ../plugins/` |
| TinyGo | tinygo | `tinygo build -o plugins/my-filter.wasm -target wasi .` |
| AssemblyScript | asc | `asc src/my-filter.ts -o plugins/my-filter.wasm` |

## Calling Convention (ABI)

WASM modules operate on linear memory — they cannot access Alloy's memory directly. All data exchange happens through the module's own memory using a pointer/length convention.

### Required Exports

```
alloc(size i32) -> ptr i32
filter(ptr i32, len i32) -> (ptr i32, len i32)
```

- **`alloc`** — Allocates a block of memory in the WASM module's linear memory. The host calls this to get a safe write offset before passing data in. Required to avoid writing at hardcoded offsets that could collide with the module's stack, heap, or data section.
- **`filter`** — Receives a pointer and length to the input string in WASM memory. Returns a pointer and length to the result string. Input and output are raw UTF-8 bytes.

### Optional Exports

```
shortcode(ptr i32, len i32) -> (ptr i32, len i32)
hook(ptr i32, len i32) -> (ptr i32, len i32)
last_error() -> (ptr i32, len i32)
```

- **`shortcode`** — Input is a JSON object: `{ "name": "youtube", "args": ["abc123"], "content": "" }`. Output is a UTF-8 HTML string.
- **`hook`** — Input is a JSON payload (shape depends on the hook type). Output is the modified JSON payload.
- **`last_error`** — Called by the host after any export returns `(0, 0)`. Returns a pointer and length to a UTF-8 error message.

### Calling Sequence

1. Host calls `alloc(inputLen)` to get a pointer in WASM memory
2. Host writes input bytes at the returned pointer
3. Host calls `filter(ptr, len)` — WASM reads input, processes it, writes result to its own memory
4. WASM returns `(resultPtr, resultLen)` — host reads result bytes from WASM memory

### Error Handling

If any export returns `(0, 0)`, the host treats it as an error. If the module exports `last_error()`, the host reads the error message and surfaces it. No silent fallback to the original input.

## Rust Example

```rust
use std::alloc::{alloc, Layout};
use std::slice;
use std::str;

#[no_mangle]
pub extern "C" fn alloc(size: i32) -> i32 {
    let layout = Layout::from_size_align(size as usize, 1).unwrap();
    unsafe { alloc(layout) as i32 }
}

#[no_mangle]
pub extern "C" fn filter(ptr: i32, len: i32) -> u64 {
    let input = unsafe {
        let slice = slice::from_raw_parts(ptr as *const u8, len as usize);
        str::from_utf8(slice).unwrap()
    };

    // Example: uppercase filter
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

    // Pack ptr and len into a single i64 return value
    ((result_ptr as u64) << 32) | (result_bytes.len() as u64)
}
```

Build: `cargo build --target wasm32-unknown-unknown --release`
Copy: `cp target/wasm32-unknown-unknown/release/my_filter.wasm plugins/`

## TinyGo Example

```go
package main

import "unsafe"

//export alloc
func alloc(size int32) int32 {
    buf := make([]byte, size)
    return int32(uintptr(unsafe.Pointer(&buf[0])))
}

//export filter
func filter(ptr, length int32) (int32, int32) {
    // Read input from WASM memory
    input := unsafe.Slice((*byte)(unsafe.Pointer(uintptr(ptr))), length)
    src := string(input)

    // Example: reverse filter
    runes := []rune(src)
    for i, j := 0, len(runes)-1; i < j; i, j = i+1, j-1 {
        runes[i], runes[j] = runes[j], runes[i]
    }
    result := []byte(string(runes))

    resultPtr := alloc(int32(len(result)))
    copy(unsafe.Slice((*byte)(unsafe.Pointer(uintptr(resultPtr))), len(result)), result)

    return resultPtr, int32(len(result))
}

func main() {}
```

Build: `tinygo build -o plugins/reverse.wasm -target wasi .`

## AssemblyScript Example

```typescript
export function alloc(size: i32): i32 {
  return heap.alloc(size) as i32;
}

export function filter(ptr: i32, len: i32): u64 {
  // Read input string from memory
  const input = String.UTF8.decodeUnsafe(ptr, len);

  // Example: word count filter
  const count = input.trim().split(" ").length;
  const result = count.toString();

  const resultBytes = String.UTF8.encode(result);
  const resultPtr = alloc(resultBytes.byteLength);
  memory.copy(resultPtr, changetype<i32>(resultBytes), resultBytes.byteLength);

  // Pack ptr and len into i64
  return (resultPtr as u64) << 32 | (resultBytes.byteLength as u64);
}
```

Build: `asc src/word-count.ts -o plugins/word-count.wasm`

## Sandboxing

WASM plugins run in an isolated memory space via wazero. They cannot access the filesystem, network, or any system resources unless explicitly granted by Alloy. Safe to run untrusted community plugins.

## When to Use WASM vs QuickJS

| | QuickJS (`.js`) | WASM (`.wasm`) |
|---|---|---|
| **Speed** | ~microseconds | ~nanoseconds (5-10x faster) |
| **Setup** | Drop a `.js` file | Compile, then drop `.wasm` file |
| **Debugging** | JS stack traces, familiar tooling | WASM traps, language-specific debugging |
| **Use when** | Prototyping, most plugins | Hot-path filters called per-page, performance critical |

For most plugins, QuickJS is simpler and fast enough. Use WASM when a filter is called thousands of times per build and profiling shows it's a bottleneck.
