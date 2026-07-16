---
type: patch
---

`onConfig` hooks that set `passthrough[].from` or `passthrough[].to` to an absolute path or a `..` traversal above the project root now produce a build error. `from: "."` is also rejected (would copy the entire project root into the output directory). `to: "."` and `to: ""` remain valid — they mean "root of the output directory." Error messages include the zero-based array index and field name (e.g. `passthrough[2].from`).

Passthrough path validation runs before any config fields are applied, so a bad passthrough entry cannot leave `build.output` or `structure.*` fields partially mutated.

Previously, a plugin could set `passthrough[].from` to `/etc/shadow` to exfiltrate files into the output directory, or `passthrough[].to` to `../../evil` to write files outside it.
