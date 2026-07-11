---
type: patch
---

Fix language-specific `_data.yaml` permalink patterns being ignored in multi-language builds. A `permalink` set in `content/es/blog/_data.yaml` now correctly applies to pages in `content/es/blog/`, instead of falling back to the default path-based URL.
