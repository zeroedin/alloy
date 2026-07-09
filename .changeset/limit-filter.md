---
type: minor
---

Add `limit` array filter that returns the first N elements, giving Go templates parity with Liquid's `{% for ... limit: N %}` clause. Available in both engines: `{{ range limit .collections.blog 5 }}` (Go) and `{{ collections.blog | limit: 5 }}` (Liquid).
