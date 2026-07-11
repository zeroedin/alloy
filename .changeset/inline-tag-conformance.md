---
type: patch
---

Tighten `{% inline %}` tag validation. Paths must now start with `./` or `../` — bare paths like `{% inline "diagram.svg" %}` are rejected; use `{% inline "./diagram.svg" %}` instead. Only text-based extensions are accepted: `.svg`, `.html`, `.htm`, `.txt`, `.css`, `.js`, `.json`, `.xml`, `.toml`, `.yaml`, `.yml`, `.md`. Other file types produce a clear error, and binary types like `.png` suggest using `<img>` instead.
