---
type: patch
---

Remove dead `LogPath()` method from `NodeBridge`. Plugin `console.log`, `console.warn`, and `process.stdout.write` output goes to the user's terminal (Alloy's stderr), not `.alloy/plugin.log`. This was already the runtime behavior — this change removes the vestigial method that referenced the old log file path.
