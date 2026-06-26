---
type: patch
---

Fix Liquid delimiters in code blocks being interpreted as template syntax when render hooks replace the default `<code>` element. Delimiters are now entity-encoded in `markup.inner` before reaching the hook template.
