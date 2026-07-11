---
type: patch
---

Remove dead `ResolveForSection` function from the permalink package. After issue #910 wired all permalink resolution through `ResolveFromCascade`, `ResolveForSection` had zero production call sites. Its flat `map[string]string` section→pattern lookup silently dropped nested `_data.yaml` permalink patterns — all coverage has been ported to `ResolveFromCascade` tests.
