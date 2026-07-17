---
type: minor
---

`onBeforeValidation` receives `{ outputPaths: [...] }` and runs immediately before conflict detection. Plugins register additional output paths via `addOutputs`, and those paths feed into `DetectConflicts()`.

```javascript
alloy.hook("onBeforeValidation", {}, (payload) => {
  return {
    addOutputs: {
      "_redirects": "plugin:netlify-redirects",
      "_headers": "plugin:netlify-headers"
    }
  };
});
```

`onAfterValidation` receives `{ outputPaths: [...], cascade: { ...siteData... } }` after conflict detection passes. Cascade mutations merge into site data for templates. The pipeline ignores `outputPaths` changes in the return.

```javascript
alloy.hook("onAfterValidation", {}, (payload) => {
  payload.cascade.buildTimestamp = new Date().toISOString();
  return payload;
});
```

Both hooks reject unrecognized return keys and type-check `addOutputs`/`cascade` as maps. Omitting a return value is a valid no-op for observation-only use.

Previously, the pipeline fired both hooks before content discovery with a stub payload and threw away the return values.
