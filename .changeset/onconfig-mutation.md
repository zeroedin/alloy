---
type: minor
---

`onConfig` hooks can mutate pipeline config. The return value is applied back to `cfg` for a mutable allowlist: `build.output`, `build.clean`, `structure.content`, `structure.layouts`, `structure.assets`, `structure.static`, `structure.data`, `passthrough`, `plugins.workers`, and `plugins.timeout`.

```javascript
alloy.hook("onConfig", {}, (config) => {
  config.build.output = "dist";
  config.structure.content = "pages";
  return config;
});
```

Fields outside the allowlist (`title`, `baseURL`, `language`, etc.) are silently ignored. Returning a non-object produces a build error. Multiple `onConfig` hooks from separate plugins chain in priority order — each receives the previous hook's return value.

Previously the return value was discarded and mutations had no effect.
