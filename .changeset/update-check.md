---
type: minor
---

Check for newer Alloy releases with `alloy version --check`. Alloy queries the GitHub Releases API and compares the latest tag against the running binary.

```
alloy version --check
```

Set `updateCheck: true` in the config file to receive a one-line notification when `alloy dev` or `alloy serve` starts and a newer version exists. Alloy caches the result for 24 hours at `~/.config/alloy/update-check.json` (respects `XDG_CONFIG_HOME`) and runs the check in the background without blocking server startup. `alloy build` never checks for updates.

```yaml
# alloy.config.yaml
updateCheck: true
```

Update checking defaults to off. Alloy makes no outbound request unless you opt in via the config or use `--check`.
