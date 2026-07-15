---
type: minor
---

`onConfig` hooks can now mutate config and have changes applied to the build pipeline. Previously, the hook's return value was silently discarded. The pipeline now captures the returned config object and applies changes for the mutable allowlist: `build.output`, `build.clean`, `structure.content`, `structure.layouts`, `structure.assets`, `structure.static`, `structure.data`, `passthrough`, `plugins.workers`, and `plugins.timeout`. Fields outside the allowlist (`title`, `baseURL`, `language`, etc.) are silently preserved. Non-object returns produce a build error identifying `onConfig` as the source. Multiple `onConfig` hooks chain correctly across plugins in priority order.
