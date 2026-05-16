---
layout: doc
title: WASM Plugins
---

WASM plugins are `.wasm` binaries compiled from Rust, TinyGo, or AssemblyScript. They run in-process via wazero with ~1-10 microsecond per-call latency -- the fastest option for custom filters on hot paths.

```
plugins/
└── slugify.wasm    # compiled from Rust, TinyGo, or AssemblyScript
```

## ABI

Every WASM plugin must export two functions:

```
alloc(size: i32) -> ptr: i32
filter(ptr: i32, len: i32) -> (ptr: i32, len: i32)
```

**`alloc`** allocates a buffer of `size` bytes in the WASM module's linear memory and returns a pointer. Alloy calls this to write input data into the module before invoking `filter`.

**`filter`** receives a pointer and length to the input data, processes it, and returns a pointer and length to the output data. The input and output are raw UTF-8 strings.

Return `(0, 0)` from `filter` to signal an error. If a `last_error` export is present, Alloy calls it to retrieve the error message.

## Optional exports

| Export | Signature | Data format | Purpose |
|---|---|---|---|
| `shortcode` | `(ptr, len) -> (ptr, len)` | JSON input, HTML output | Register a shortcode handler |
| `hooks` | `() -> (ptr, len)` | JSON array of event names | Declare which lifecycle events to subscribe to |
| `hook` | `(ptr, len) -> (ptr, len)` | JSON payload in, JSON payload out | Handle a lifecycle event |
| `last_error` | `() -> (ptr, len)` | UTF-8 error message | Called when `filter` or `shortcode` returns `(0, 0)` |

## Data formats

- **Filters** receive and return raw UTF-8 text. No JSON wrapping.
- **Shortcodes** receive a JSON object (`{"args": ["arg1", "arg2"], "body": "..."}`) and return an HTML string.
- **Hooks** receive and return JSON payloads matching the event schema. See [Lifecycle Events](/hooks/).

## Rust example

```rust
// src/lib.rs
use std::alloc::{alloc, Layout};
use std::slice;

#[no_mangle]
pub extern "C" fn alloc(size: i32) -> *mut u8 {
    let layout = Layout::from_size_align(size as usize, 1).unwrap();
    unsafe { alloc(layout) }
}

#[no_mangle]
pub extern "C" fn filter(ptr: i32, len: i32) -> u64 {
    let input = unsafe {
        let slice = slice::from_raw_parts(ptr as *const u8, len as usize);
        std::str::from_utf8(slice).unwrap()
    };

    let output = input.to_uppercase();
    let out_bytes = output.into_bytes();
    let out_ptr = alloc(out_bytes.len() as i32);

    unsafe {
        std::ptr::copy_nonoverlapping(
            out_bytes.as_ptr(),
            out_ptr,
            out_bytes.len(),
        );
    }

    // Pack (ptr, len) into a single u64 return
    ((out_ptr as u64) << 32) | (out_bytes.len() as u64)
}
```

Build:

```bash
cargo build --target wasm32-unknown-unknown --release
cp target/wasm32-unknown-unknown/release/my_filter.wasm plugins/
```

## TinyGo example

```go
package main

import "unsafe"

//export alloc
func alloc(size int32) *byte {
	buf := make([]byte, size)
	return &buf[0]
}

//export filter
func filter(ptr, length int32) uint64 {
	input := unsafe.Slice((*byte)(unsafe.Pointer(uintptr(ptr))), length)
	// Reverse the string
	output := make([]byte, len(input))
	for i, b := range input {
		output[len(input)-1-i] = b
	}
	outPtr := alloc(int32(len(output)))
	copy(unsafe.Slice(outPtr, len(output)), output)
	return uint64(uintptr(unsafe.Pointer(outPtr)))<<32 | uint64(len(output))
}

func main() {}
```

Build:

```bash
tinygo build -o plugins/reverse.wasm -target wasi -no-debug .
```

## AssemblyScript example

```typescript
// assembly/index.ts
export function alloc(size: i32): i32 {
  return heap.alloc(size) as i32;
}

export function filter(ptr: i32, len: i32): u64 {
  const input = String.UTF8.decodeUnsafe(ptr, len);
  const output = input.toLowerCase();
  const encoded = String.UTF8.encode(output);
  const outPtr = alloc(encoded.byteLength);
  memory.copy(outPtr, changetype<i32>(encoded), encoded.byteLength);
  return (u64(outPtr) << 32) | u64(encoded.byteLength);
}
```

Build:

```bash
npx asc assembly/index.ts -o plugins/lowercase.wasm --optimize
```

## Error handling

Return `(0, 0)` from `filter` or `shortcode` to signal an error. If your module exports `last_error`, Alloy calls it to retrieve the error message:

```rust
static mut LAST_ERROR: Option<String> = None;

#[no_mangle]
pub extern "C" fn filter(ptr: i32, len: i32) -> u64 {
    match process(ptr, len) {
        Ok(result) => result,
        Err(msg) => {
            unsafe { LAST_ERROR = Some(msg); }
            0 // (0, 0) signals error
        }
    }
}

#[no_mangle]
pub extern "C" fn last_error() -> u64 {
    let msg = unsafe { LAST_ERROR.take().unwrap_or_default() };
    let bytes = msg.into_bytes();
    let ptr = alloc(bytes.len() as i32);
    unsafe {
        std::ptr::copy_nonoverlapping(bytes.as_ptr(), ptr, bytes.len());
    }
    ((ptr as u64) << 32) | (bytes.len() as u64)
}
```

Without `last_error`, Alloy logs a generic "WASM filter returned error" message.

## Performance

| Metric | Typical value |
|---|---|
| Module load | ~1-5ms (once, at build start) |
| Per-call latency | ~1-10 microseconds |
| Memory overhead | ~1-2MB per module |

WASM plugins are the fastest plugin tier. For a filter called 10,000 times in a build, total overhead is 10-100ms. Use WASM when per-call latency matters -- heavy string processing, content transforms on large sites.

## Sandboxing

WASM plugins run in the same wazero sandbox as QuickJS plugins:

- No filesystem access
- No network access
- No system calls
- Communication only through the defined ABI exports

WASM modules from any source are safe to run. They cannot access anything outside their own linear memory unless Alloy explicitly passes data through the ABI.
