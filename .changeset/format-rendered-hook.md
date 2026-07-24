---
type: minor
---

`onFormatRendered` fires once per non-HTML format body after layout rendering with `{ format, content, url, path, frontMatter }`. Return an object with a `content` key to replace the rendered body. The build ignores all other keys in the return value.

```javascript
alloy.hook("onFormatRendered", {}, (payload) => {
  if (payload.format === "json") {
    return { content: JSON.stringify(JSON.parse(payload.content)) };
  }
});
```

`onPageRendered` skips pages whose `outputs` contains only non-HTML formats. A page with `outputs: ["json"]` routes through `onFormatRendered` instead. Both hooks fire independently when a page declares HTML and non-HTML outputs together.

Return `null`, `undefined`, or an object without a `content` key to keep the original format body.
