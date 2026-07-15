---
type: patch
---

Fix Node bridge protocol corruption when a plugin or its dependencies call `process.stdout.write()`. The bridge captures the real `process.stdout.write` at startup and redirects subsequent stdout writes to stderr. `sendMessage` retains the only reference to real stdout, so plugin code and npm dependencies cannot inject bytes into the JSON-RPC channel.

When non-frame bytes reach the Go frame reader (e.g., a child process inheriting stdout), the error message includes a bounded snippet of the offending output and points to stdout pollution as the cause. Replaces the generic "missing Content-Length header" message.
