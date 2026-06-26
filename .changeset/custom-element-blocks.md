---
type: minor
---

Custom elements (HTML tags with hyphens like `<alloy-code>`, `<wa-tab-group>`) are now treated as block-level HTML in Goldmark. Content inside is preserved verbatim — no markdown processing, no smart quotes, no `<p>` wrapping — and blank lines do not terminate the block.

Configurable via `content.markdown.goldmark.customElements` (default: `true`).

```yaml
# alloy.config.yaml
content:
  markdown:
    goldmark:
      customElements: true     # treat custom elements as block-level HTML (default: true)
```

```markdown
<!-- content/example.md -->
<wa-tab-group>
<wa-tab panel="one">Tab 1</wa-tab>

<wa-tab-panel name="one">
Panel content with "quotes" and blank lines — all preserved verbatim.
</wa-tab-panel>
</wa-tab-group>
```
