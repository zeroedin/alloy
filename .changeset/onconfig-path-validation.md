---
type: patch
---

`onConfig` hooks that set `build.output` or any `structure.*` field to an absolute path, a `..` traversal above the project root, `.`, or an empty string now produce a build error. Valid relative paths with embedded `..` segments that resolve within the project (e.g. `subdir/../dist`) are accepted and cleaned before use. On Windows, reserved device names (`NUL`, `CON`) and volume-relative paths (`C:..`) are also rejected via `filepath.IsLocal`.

All path fields are validated before any are applied to the config, so a validation failure on one field cannot leave the config partially mutated.

Previously, a plugin could set `structure.content = "/etc"` or `build.output = "../../evil"` via `onConfig` and the values flowed through `resolveDir` unchecked. With `clean: true` (the default), `CleanOutputDir` would run `os.RemoveAll` on directories outside the project tree.
