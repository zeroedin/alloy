---
type: patch
---

Tighten `{% inline %}` tag validation to match the PLAN.md specification. Two changes:

1. **Path prefix required.** Paths must start with `./` or `../` — bare relative paths like `{% inline "diagram.svg" %}` are now rejected with a clear error message. Use `{% inline "./diagram.svg" %}` instead.

2. **Allowlist replaces denylist.** Only the 12 text-based extensions listed in the spec are accepted: `.svg`, `.html`, `.htm`, `.txt`, `.css`, `.js`, `.json`, `.xml`, `.toml`, `.yaml`, `.yml`, `.md`. Previously, any extension not on a binary denylist was accepted, which allowed unintended file types like `.env` or extensionless files through.

Binary file types (images, fonts, media, archives) now include usage guidance in the error message — e.g., "use `<img>` instead" for image extensions.
