---
type: patch
---

QuickJS plugin runtime operations are now serialized with a mutex. `Close()` waits for any in-flight hook, filter, or shortcode call to finish before freeing the WASM instance. Methods called after `Close()` return safely instead of panicking.

Previously, when a plugin hook exceeded its timeout, `RunWithTimeout` abandoned the goroutine while it was still executing inside the WASM context. The deferred `Close()` at the end of `Build()` then freed the runtime mid-execution, causing an intermittent `out of bounds memory access` panic during build teardown.
