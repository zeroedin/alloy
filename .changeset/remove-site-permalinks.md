---
type: minor
---

The `permalinks:` key in `alloy.config.yaml` has been removed. Permalink patterns are now set exclusively via `_data.yaml` cascade files, as specified by PLAN.md. Existing config files that still contain a `permalinks:` key will load without error — the key is silently ignored.

Before (no longer supported):

```yaml
# alloy.config.yaml
permalinks:
  blog: "/:year/:month/:slug/"
```

After (use _data.yaml cascade instead):

```yaml
# content/blog/_data.yaml
permalink: "/:year/:month/:slug/"
```
