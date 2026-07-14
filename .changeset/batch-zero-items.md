---
type: patch
---

Fix batch hooks firing a spurious timeout warning when called with 0 items. The effective timeout was calculated as `timeout * itemCount`, which produced a 0ms timeout that expired instantly. Alloy now skips the hook when there are no payloads to process. This surfaces during incremental rebuilds where scope filtering leaves 0 pages for post-render hooks like `onPageRendered`.
