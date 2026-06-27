---
type: patch
---

Make the plugins directory configurable via `structure.plugins` in the config file and `--plugins` flag in `alloy init`. Previously, the plugins directory was hardcoded to `plugins/` while all other managed directories were configurable. Also fixes plugin file changes not being detected by the dev server watcher, and a bug where nested plugin paths (e.g. `tools/plugins`) broke Node runtime project root derivation.
