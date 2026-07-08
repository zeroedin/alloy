---
type: minor
---

TOC generation can now be disabled site-wide with `content.markdown.toc: false` in the config file. When omitted, `page.toc` is populated for all Markdown pages by default.

```yaml
# alloy.config.yaml
content:
  markdown:
    toc: false
```
