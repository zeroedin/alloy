---
type: minor
---

**Breaking:** A page that names a layout that doesn't exist now fails the build, whether the name is in front matter, in a `_data.yaml` cascade, or extension-bearing like `layout: "missing.html"`. The error names the layout and the page:

```text
Error: layout "nonexistent" not found for page "about.md"
```

Previously the build logged a warning, wrote the page without its layout, and exited 0, so the broken page was easy to miss.

Fix the name, add the layout, or set `layout: false` to build the page without one. `alloy dev` shows the error in its overlay when it rebuilds.
