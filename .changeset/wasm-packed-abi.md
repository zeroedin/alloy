---
type: minor
---

**Breaking:** WASM plugins must return a packed `i64` (`result = ptr << 32 | len`) from all data-returning exports (`filter`, `shortcode`, `hook`, `hooks`, `last_error`). Alloy rejects the previous multi-value `(result i32 i32)` form at load time.

```rust
#[no_mangle]
pub extern "C" fn filter(ptr: i32, len: i32) -> u64 {
    // ... transform input ...
    ((result_ptr as u64) << 32) | (result_len as u64)
}
```

Load-time validation checks every data-returning export's signature. Modules using the old multi-value ABI or the sret form (`(param i32 i32 i32) -> ()` produced by Rust >= 1.82 and TinyGo tuple returns) get an error naming the export and its actual signature.

The packed form works with current Rust, TinyGo, and AssemblyScript without unstable compiler flags.
