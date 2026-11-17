---
type: minor
---

**Breaking:** `alloy serve` now stops with exit code 1 on any error that fails `alloy build`. That covers a rebuild after a file change, and an output path conflict when a changed static, asset or passthrough file is copied, as well as the startup build. The server prints the error, removes its lockfile and exits:

```text
Error: rebuild failed: layout "nonexistent" not found for page "about.md"
```

```text
Error: output path conflict: about/index.html is claimed by content/about.md and static/about/index.html
```

Previously a failed rebuild left `alloy serve` running over an emptied `_site`, so every page returned 404 and the error showed only in the terminal. A changed static, asset or passthrough file was copied over whatever was already at its path, with no check at all, although `alloy build` fails on the same conflict.

Fix the error and start `alloy serve` again. A rebuild that succeeds keeps the server running, as before.
