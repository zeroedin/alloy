---
type: patch
---

A build that fails validation no longer deletes your previous output. Alloy used to empty the output directory before it checked the build was valid, so a conflict or a plugin error left you with nothing — the new build refused, and the old one was already gone.

This is most visible in `alloy dev`. A colliding file added mid-session is handled without a full rebuild, so the site keeps serving. But the next change that triggers a full rebuild — a plugin or component edit, unrelated to the colliding files — used to take every page down until you fixed the conflict:

```text
_site after the failed rebuild:   empty
GET /about/                        404
```

Now the previous output stays put and the site keeps serving while you fix it:

```text
_site after the failed rebuild:   all pages intact
GET /about/                        200
```

The error is unchanged — you still get the same message naming what collided.

This covers failures Alloy catches *before* it starts rendering: output path conflicts, alias and permalink problems, and errors from `onAfterValidation`. A failure while rendering — a broken template, a filter that throws — still happens after the clean, so the output directory is emptied in those cases.

`build.clean: false` is unchanged and still skips cleaning entirely.
