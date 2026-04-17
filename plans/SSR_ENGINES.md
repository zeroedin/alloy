# Compatible SSR Engines

Alloy's SSR Phase 2 is engine-agnostic. Any CLI tool that accepts a component HTML fragment on **stdin** and returns SSR'd HTML (with Declarative Shadow DOM) on **stdout** works with the `ssr.render` config.

This is intentional — Alloy does not import any SSR engine as a Go dependency. The `ssr:` config block is the only integration point.

```yaml
ssr:
  render: "<command that reads stdin, writes stdout>"
```

---

## golit (recommended)

**Repo**: [github.com/zeroedin/golit](https://github.com/zeroedin/golit)

Pure Go Lit SSR engine using QuickJS via WebAssembly (wazero). Zero CGo, single binary. No Node.js required.

```yaml
ssr:
  render: "golit render --defs bundles/"
  serve:
    cmd: "golit serve --defs bundles/"
    endpoint: "http://localhost:9777/render"
```

**Why recommended:**
- Fast cold start — well-suited for Alloy's per-instance render model (one invocation per unique component instance)
- Purpose-built for Alloy's workflow
- HTTP serve mode for dev server integration
- Multiple component discovery modes (HTML auto-discovery, import map, source directory, pre-bundled)

**Limitation:** Own rendering implementation — not the official @lit-labs/ssr. Conformance testing against the official implementation is tracked in zeroedin/golit#22.

---

## lit-ssr-wasm

**Repo**: [github.com/bennypowers/lit-ssr-wasm](https://github.com/bennypowers/lit-ssr-wasm)
**Author**: Benny Powers
**Status**: v0.1.0 (March 2026), early stage

Pure Go library + CLI that wraps the official `@lit-labs/ssr` package (compiled to WASM via Javy, executed on wazero). No Node.js required.

```yaml
ssr:
  render: "lit-ssr --dir ./components/"
```

With long-running process for better performance:

```yaml
ssr:
  render: "lit-ssr --dir ./components/"
  serve:
    cmd: "lit-ssr --dir ./components/"
    protocol: "stdio"
```

**When to use:**
- When guaranteed rendering fidelity with the official Lit SSR implementation matters (e.g., complex components using reactive controllers, `@query` decorators, `createRef()`, or Lit 4.x features)
- When you need the exact same SSR output that @lit-labs/ssr produces

**Trade-offs:**
- ~350ms cold start per invocation (WASM bootstrap + component bundling). With 100 unique component instances, that's ~35 seconds of startup overhead in one-shot mode.
- The `ssr.serve` config with `protocol: "stdio"` keeps the process warm and amortizes cold start — recommended for lit-ssr-wasm.
- Supports bytecode pre-compilation (`CompileFiles()`) which cuts cold start by ~2.5x.

---

## Comparison

| Capability | golit | lit-ssr-wasm |
|---|---|---|
| Pure Go, no Node.js | Yes | Yes |
| SSRs Lit components | Yes | Yes |
| Declarative Shadow DOM output | Yes | Yes |
| WASM runtime | wazero (QuickJS) | wazero (QuickJS via Javy) |
| Rendering engine | Own implementation | Official @lit-labs/ssr |
| Cold start (one-shot) | Fast | ~350ms |
| Per-render latency (warm) | <1ms | ~0.32ms |
| Component bundling | Built-in (esbuild) | Built-in (esbuild) |

Full competitive analysis: zeroedin/golit#22

---

## Adding a new engine

Any tool that follows this contract works with Alloy:

1. Accept a single component HTML fragment on **stdin**
2. Return the SSR'd HTML (with `<template shadowrootmode="open">`) on **stdout**
3. Exit 0 on success, non-zero on failure (stderr for error messages)

Example:

```
echo '<ds-button variant="primary">Click Me</ds-button>' | your-ssr-tool --defs ./components/
```

Expected output:

```html
<ds-button variant="primary">
  <template shadowrootmode="open">
    <style>:host { ... }</style>
    <button class="btn btn--primary"><slot></slot></button>
  </template>
  Click Me
</ds-button>
```

Configure in `alloy.config.yaml`:

```yaml
ssr:
  render: "your-ssr-tool --defs ./components/"
```

Optionally, if the tool supports a long-running process for dev server use:

```yaml
ssr:
  render: "your-ssr-tool --defs ./components/"
  serve:
    cmd: "your-ssr-tool serve --defs ./components/"
    endpoint: "http://localhost:PORT/render"    # HTTP mode
    # or
    protocol: "stdio"                           # NUL-delimited stdin/stdout mode
```
