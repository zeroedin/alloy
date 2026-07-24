---
type: minor
---

`structure.components` controls where Alloy looks for SSR component source files. Defaults to `components/` when omitted.

```yaml
# alloy.config.yaml
structure:
  components: "elements"
```

During `alloy serve`, Alloy watches this directory for changes and re-renders pages that use the affected components. Projects that keep component source outside `components/` previously got no SSR invalidation on file changes.
