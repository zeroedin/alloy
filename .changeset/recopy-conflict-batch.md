---
type: patch
---

`alloy serve` and `alloy dev` judge an output path conflict from a changed static, asset or passthrough file once the whole batch of changes is applied, as a build of the final files would. Replacing a file within one batch no longer looks like a conflict, whether the old owner was a static, asset or passthrough file, or a page:

```text
assets/site.css created, then static/site.css deleted
GET /site.css   asset{}
```

Previously each change was checked the moment it was applied, so the new file was blocked by the old one's claim, and the later delete never released it. `alloy serve` exited with `Error: output path conflict: site.css is claimed by static/site.css and assets/site.css`, and `alloy dev` returned 404 for `/site.css` and showed the stale conflict until the next rebuild. A file created and deleted in the same batch was reported the same way. In `alloy dev`, a static file added in the same batch that deleted the page at its path was never copied, and the deleted page was still served.

A conflict still there at the end of the batch is reported as before: `alloy serve` exits 1, and `alloy dev` shows it in the error overlay.
