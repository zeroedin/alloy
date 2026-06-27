---
type: patch
---

Fix spurious warnings during `alloy dev` and `alloy serve` when atomic-write editors create temp files that vanish before the debounced watcher copy runs. Transient `os.ErrNotExist` errors are now silently skipped.
