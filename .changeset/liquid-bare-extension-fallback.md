---
type: minor
---

Liquid layout resolution now falls back to bare extensions when `.liquid` files are missing. For each candidate in the lookup chain, the Liquid engine tries `.liquid` first then the bare extension (e.g., `default.html`, `single.json`) and parses the result as Liquid.

This applies to standard page layouts, output format layouts (`single.json.liquid` → `single.json`), taxonomy layouts, and parent layouts in layout chains.

Layout names with a recognized extension (e.g., `layout: "base.html"`, `layout: "feed.xml"`) are now used as literal filenames — no engine extension is appended. Bare names without an extension (e.g., `layout: "base"`) get the `.liquid` → `.html` fallback.

Explicit layout names set via front matter `layout:` or `_data.yaml` cascade are strict — only the `.liquid` extension is tried, and a missing file produces a build error rather than silently falling through.
