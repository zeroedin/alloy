---
type: patch
---

While a build error is active, `alloy dev` shows it on missing pages too. A page that returns 404 is an HTML page with the error overlay, and it reloads once the error is fixed:

```text
GET /n1/   404   <div id="alloy-error-overlay"> … layout "nonexistent" not found for page "n5.md"
```

Previously a failed full rebuild emptied `_site`, and every page answered with a bare `404 page not found`. The error showed only in the terminal, and the page didn't reload after the fix.

The status stays 404. With no active error, and always in `alloy serve`, the 404 is unchanged.
