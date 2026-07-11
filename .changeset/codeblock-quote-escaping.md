---
type: patch
---

Fix codeblock render hook over-escaping `"` and `'` to `&#34;` and `&#39;` in `markup.inner`, breaking shiki and other downstream highlighters that don't decode quote entities. Codeblock inner content is element content (inside `<alloy-code>…</alloy-code>`), where only `&`, `<`, and `>` need escaping — quote characters are safe and must pass through as literal characters. The `markup.language` field continues to use full HTML attribute escaping since it lands in a `lang="…"` attribute.
