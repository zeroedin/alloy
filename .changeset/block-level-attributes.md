---
type: minor
---

Markdown block elements now support `{.class #id key=value}` attribute syntax beyond headings. Fenced code blocks accept attributes on the opening fence line, while blockquotes and tables accept them on the trailing line.

````markdown
```go {.highlight #example}
fmt.Println("hello")
```

> This is important
{.callout}

| Name  | Role     |
| ----- | -------- |
| Alice | Engineer |
{.striped}
````

Attributes are available in render hooks via `markup.attributes` for all block element types — headings, fenced code blocks, blockquotes, and tables. When no attributes are present, `markup.attributes` is an empty map.

```liquid
<!-- layouts/render-codeblock.liquid -->
<pre class="{{ markup.attributes.class }}" data-lang="{{ markup.language }}">{{ markup.inner }}</pre>
```

Block-level attributes are automatically enabled when `autoHeadingID` is true (the default). No additional configuration is needed.
