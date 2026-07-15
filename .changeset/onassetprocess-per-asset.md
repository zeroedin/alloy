---
type: minor
---

`onAssetProcess` now fires once per asset file with `{path, content}` instead of once per build with directory paths. The return value's `content` key replaces the asset content written to the output directory.

```javascript
alloy.hook("onAssetProcess", {}, (asset) => {
  if (asset.path.endsWith('.css')) {
    return { content: minifyCSS(asset.content) };
  }
  return asset;
});
```

Returning `null`, `undefined`, or an object without a `content` key preserves the original file content. The `path` key in the return value is ignored — only `content` is applied back. Hook errors halt the build.

Previously the hook received `{assetsDir, outputDir}` directory paths and discarded the return value, making every plugin written from the docs a silent no-op.
