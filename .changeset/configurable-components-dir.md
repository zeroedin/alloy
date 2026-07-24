---
type: minor
---

The SSR component source directory is configurable via `structure.components`, matching the pattern used by all other structure directories.

```yaml
# alloy.config.yaml
structure:
  components: "elements"
```

Set `structure.components` to the directory containing your web component source files. The file watcher, change classifier, and incremental SSR invalidation all read from this config. When omitted, Alloy defaults to `components/`, preserving existing behavior.

Projects with component source in a non-standard directory (e.g., `elements/`) get correct `ComponentChange` classification and SSR re-rendering during `alloy serve` without workarounds.
