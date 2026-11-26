---
type: patch
---

`alloy dev`'s live reload now uses `github.com/gorilla/websocket` at the commit that fixes [GO-2026-6278](https://pkg.go.dev/vuln/GO-2026-6278), in place of v1.5.3. The advisory is a weak random number generator for the WebSocket mask key. `govulncheck` passes again.
