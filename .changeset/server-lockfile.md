---
type: minor
---

`alloy dev` and `alloy serve` write a lockfile at `.alloy/server.lock` on startup. If another alloy process is already watching the same project directory, a warning prints to stderr with the conflicting PID, port, mode, and a `kill` command. Startup continues without blocking.

```
warning: another alloy process (PID 4659, alloy serve on port 3003, started 2026-07-14T13:00:00-04:00) is watching this directory
warning: concurrent instances writing to _site/ will cause missing pages and 404s
warning: kill the other process with: kill 4659
```

Stale lockfiles from crashed processes (dead PID or corrupt JSON) are removed on the next startup. The lockfile is removed on clean shutdown via signal handler.

Previously, a backgrounded `alloy serve` and a new `alloy dev` session would silently fight over `_site/`, with `clean: true` full rebuilds wiping incremental output. Pages vanished with no errors in either console.
