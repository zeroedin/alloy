---
type: patch
---

QuickJS CallHook now builds JS objects directly for HookRenderedPayload, HookFormatRenderedPayload, and HookTransformPayload instead of JSON-serializing the entire struct. Large string fields (html, content) transfer as native strings with zero JSON overhead. Only small metadata fields (frontMatter, toc) use JSON. Eliminates ~3.2MB of JSON churn per 800KB page on the RHDS benchmark, recovering the 4.3x post-render hook regression introduced by the v0.6.0 object API.
