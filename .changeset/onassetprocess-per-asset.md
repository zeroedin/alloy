---
type: minor
---

`onAssetProcess` fires once per asset file with `{path, content}` instead of once per build with directory paths. The returned `content` key replaces the file in the output directory.

```javascript
alloy.hook("onAssetProcess", {}, (asset) => {
  if (asset.path.endsWith('.css')) {
    return { content: minifyCSS(asset.content) };
  }
  return asset;
});
```

Return `null`, `undefined`, or an object without a `content` key to keep the original file. The build ignores any `path` key you return. Hook errors stop the build.

Before this change, the hook received `{assetsDir, outputDir}` directory paths and discarded the return value. Plugins that followed the docs were silent no-ops.
