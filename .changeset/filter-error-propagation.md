---
type: patch
---

Fix plugin filter and shortcode call errors being silently swallowed during template rendering. A filter that throws now fails the build with the filter name and source file path in the error message, instead of passing through the original input unchanged. Shortcode errors fail instead of producing empty output.

Previously, `{{ "hello" | myFilter }}` where `myFilter` threw an exception would silently output `hello` — the most dangerous failure mode, producing incorrect output with no warning.
