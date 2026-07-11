---
type: patch
---

Remove the extensionless (raw name) fallback from partial/include resolution in both template engines. Previously, `{% include "widget" %}` and `{{ include "widget" }}` would try `widget.liquid`, `widget.html`, and then `widget` (no extension) as a final fallback. A template file without an extension has no clear use case and is almost certainly a mistake.

Both engines now try only recognized extensions:
- **Liquid:** `widget.liquid`, then `widget.html`
- **Go templates:** `widget.html` only

Sites relying on extensionless template files will see a build error referencing the partial name.
