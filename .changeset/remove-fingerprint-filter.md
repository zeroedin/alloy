---
type: patch
---

Remove the broken `fingerprint` template filter. It hashed the path string instead of file contents and emitted no renamed file, so fingerprinted URLs would 404. Use `cachebust` for query-string cache busting or a plugin for filename-rewriting fingerprinting.
