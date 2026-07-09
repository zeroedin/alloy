---
type: minor
---

Add `include` function to the Go template engine, providing parity with Liquid's `{% include %}` for cross-file template inclusion.

```gotemplate
{{ include "partials/header" }}
{{ include "partials/card" (dict "item" . "compact" true) }}
{{ $nav := include "partials/nav" }}
```

Includes resolve from the layouts directory (`.html` extension, then raw name), support nested calls, and default to the current context when no argument is given.
