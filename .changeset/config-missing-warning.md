---
type: minor
---

Running `alloy build`, `alloy dev`, or `alloy serve` from a directory without a config file produces an actionable error instead of falling back to empty defaults. The error includes the directory Alloy searched, the expected file names (`alloy.config.yaml`, `.yml`, `.toml`, `.json`), and a `--config` suggestion for non-standard layouts.

When `--config` points to a file that does not exist, the error reports the specified path without repeating the `--config` suggestion.
