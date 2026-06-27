---
type: patch
---

Fix `alloy dev` not rebuilding pages when layout partials change. Editing files like `layouts/partials/header.liquid` now correctly triggers a full rebuild instead of silently skipping all pages.
