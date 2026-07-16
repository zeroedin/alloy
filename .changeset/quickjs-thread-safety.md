---
type: patch
---

Plugin hooks that exceed their timeout no longer cause a panic during build teardown. `Close()` waits for any in-flight hook, filter, or shortcode call to finish before releasing the QuickJS runtime.

Previously, a timed-out plugin hook could trigger an `out of bounds memory access` panic at the end of `Build()` because the runtime was freed while the hook was still executing.
