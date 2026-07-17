---
type: patch
---

Fix omitted `pages` scope defaulting to "all pages" instead of "none". Hooks registered with `{}` or `{data: ["elements"]}` produced a spurious validation warning on pageless events like `onConfig` and `onBuildComplete`. Batch hooks also serialized all pages when the plugin had not requested page data. Plugins that want pages must declare `pages: true`.
