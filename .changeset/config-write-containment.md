---
type: minor
---

**Breaking:** Alloy now refuses to write outside your project. `build.output` must resolve inside the project root, and a passthrough `to` must resolve inside the output directory. Both were unchecked, so a config file could scatter files anywhere on disk while the build reported success.

```yaml
passthrough:
  - from: vendor
    to: "../../somewhere-else"    # wrote outside the project, exit 0
```

```text
Error: validation error: passthrough[0].to: path "../../somewhere-else" writes outside the output directory
Error: validation error: build.output: path "../../elsewhere" writes outside the project root
```

An absolute `to` is rejected rather than quietly reinterpreted. `to: "/srv/assets"` used to become `_site/srv/assets/…` — a path you never asked for.

**The most dangerous case this closes is `build.output: "."`.** That made the project its own output directory, and because `build.clean` defaults to true, the build deleted your content, layouts, and config file, then reported success. Any value that resolves to the project root is now rejected, however it is written.

**Reading from outside the project still works, and is unchanged.** A passthrough `from` may be absolute or use `../` to share assets across projects — that is a deliberate feature. The rule is about direction: Alloy may read from anywhere you point it, but only writes where you'd expect.

Paths are checked after they are resolved, so `to: "subdir/../dist"` is still fine — it lands inside the output directory. Only escaping the boundary is an error.

Validation runs before anything is created or cleaned, so a rejected config leaves your files untouched.
