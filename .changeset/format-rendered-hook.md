---
type: minor
---

`onFormatRendered` fires once per non-HTML format body after layout rendering. The payload carries `format`, `content`, `url`, `path`, and `frontMatter`. Return an object with a `content` key to replace the rendered body; other fields are read-only.

```javascript
alloy.hook("onFormatRendered", {}, (payload) => {
  if (payload.format === "json") {
    return { content: JSON.stringify(JSON.parse(payload.content)) };
  }
});
```

`onPageRendered` no longer fires for pages whose `outputs` contains only non-HTML formats. Pages with `outputs: ["json"]` route through `onFormatRendered` instead. Pages with both HTML and non-HTML outputs fire `onPageRendered` for the HTML body and `onFormatRendered` for each non-HTML format body.

Return `null`, `undefined`, or an object without a `content` key to keep the original format body. Non-string `content` values are ignored. Plugins that process `onPageRendered` are unaffected when the page includes HTML output.
