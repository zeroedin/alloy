---
type: patch
---

Accept `"go"` as an alias for `"gotemplate"` in the `templates.engine` config field, and reject unknown engine values with a clear error instead of silently falling through to Liquid.
