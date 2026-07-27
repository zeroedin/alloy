---
type: patch
---

Fix WASM and Node hooks losing `url`, `path`, and `frontMatter` when chained with other hooks on the same event. A QuickJS hook chaining after a WASM or Node hook received only the mutable field (`html` or `content`) — context fields were silently dropped. All three runtimes now preserve context through the full hook chain.
