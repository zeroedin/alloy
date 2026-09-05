---
type: minor
---

**Breaking:** Two sources writing the same output path is now a build error, whatever kind of file it is. Previously only rendered pages, taxonomy pages, and plugin-registered outputs were checked — files copied from `static/`, `assets/`, a passthrough mapping, or alongside your content were not, so a collision between them quietly resolved by copy order and the last one written won.

The worst case was silent: a stray `content/about/index.html` sitting next to `content/about.md` replaced your rendered About page, and the build still reported success.

```text
Error: output path conflict detected:
  css/styles.css is claimed by:
    1. static/css/styles.css
    2. assets/css/styles.css
    3. passthrough "vendor-css" → "css"
    4. content/css/styles.css (colocated)

Resolve by renaming one source, adjusting a passthrough "to" path, or removing one source.
```

Every claimant is listed, and every conflict is reported — not just the first one, so you can fix them in one pass rather than one build at a time.

**Sharing a directory is still fine.** Only identical paths collide. `static/css/` and a passthrough writing into `css/` merge exactly as before, as long as the filenames differ.

A passthrough file that an `exclude` pattern skips is never copied, so it never conflicts with anything.

`alloy dev` and `alloy serve` check too. Adding a colliding file while the server is running shows the conflict in the browser error overlay and skips that one copy instead of overwriting; the server keeps running, and the next rebuild clears the overlay once you resolve it.

**You may need to change something.** A site that builds today can start failing — those are the collisions that were already losing a file without telling you. Rename one of the sources, or point the passthrough `to` somewhere else.
