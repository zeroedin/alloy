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

Disabling TOC is independent of `autoHeadingID` — headings retain their `id` attributes regardless of the `toc` setting. For non-Markdown content, plugins can build TOC data via the `onContentTransformed` hook.
