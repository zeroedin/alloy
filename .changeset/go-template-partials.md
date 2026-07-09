---
type: minor
---

Add `partial` function to the Go template engine, providing parity with Liquid's `{% include %}` for cross-file template inclusion.

```gotemplate
{{ partial "partials/header" }}
{{ partial "partials/card" (dict "item" . "compact" true) }}
{{ $nav := partial "partials/nav" }}
```

Partials resolve from the layouts directory (`.html` extension, then raw name), support nested calls, and default to the current context when no argument is given.
