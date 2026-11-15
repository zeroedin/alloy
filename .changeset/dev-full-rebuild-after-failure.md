---
type: patch
---

`alloy dev` never reports success while most of the site is missing. After a full rebuild fails, the next rebuild is full too, whatever changed, so `rebuild complete` means the whole site is back:

```text
rebuild failed: layout "nonexistent" not found for page "n5.md"
rebuild complete
GET /n1/  /n12/  /site.css  /sitemap.xml   200
```

Previously a full rebuild that failed left `_site` empty, and the edit that fixed it ran a small rebuild that rendered only the changed page. It reported `rebuild complete (incremental)` and cleared the error while every other page, the static files and `sitemap.xml` still returned 404 until you restarted `alloy dev`. The same happened when the startup build failed.

No change is needed on your side. A full rebuild is the startup build, a change to 10 or more files, or a plugin or component change.
