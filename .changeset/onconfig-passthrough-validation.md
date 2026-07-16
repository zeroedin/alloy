---
type: patch
---

`onConfig` hooks that set `passthrough[].from` or `passthrough[].to` to an absolute path or a `..` traversal above the project root now produce a build error. The validator rejects `from: "."` (would copy the entire project root into the output directory). `to: "."` and `to: ""` remain valid, meaning "root of the output directory." Error messages include the zero-based array index and field name (e.g. `passthrough[2].from`).

Passthrough path validation runs before the config is applied, so a bad passthrough entry cannot half-mutate `build.output` or `structure.*` fields.

Previously, a plugin could set `passthrough[].from` to `/etc/shadow` to exfiltrate files into the output directory, or `passthrough[].to` to `../../evil` to write files outside it.
