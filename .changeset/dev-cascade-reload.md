---
type: patch
---

`alloy dev` now picks up `_data.yaml` edits without a restart. Adding, changing or deleting a `_data.yaml` or `_data.yml` re-renders every page in that folder and its subfolders, using the folder data on disk:

```yaml
# content/blog/_data.yaml
badge: two   # was: one
```

```text
GET /blog/p/        badge=[two]
GET /blog/deep/q/   badge=[two]
GET /docs/guide/    badge=[docs]   (not re-rendered)
```

Previously `alloy dev` read every `_data.yaml` once, at startup. An edit, including a misspelled `layout`, changed nothing in the browser until you restarted `alloy dev`, even after you edited a page in that folder.

No change is needed on your side. Edits to files in `data/` were already picked up.
