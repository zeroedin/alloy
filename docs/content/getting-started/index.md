---
layout: doc
title: Installation
---

Install Alloy as a single Go binary with no runtime dependencies.

```bash
go install github.com/zeroedin/alloy/cmd/alloy@latest
```

Verify the installation:

```bash
alloy --version
```

## Prerequisites

- **Go 1.21+** — [download from go.dev](https://go.dev/dl/)

Alloy compiles to a single binary. Once installed, Go is only needed for updates.

## Install via `go install`

The recommended way to install Alloy:

```bash
go install github.com/zeroedin/alloy/cmd/alloy@latest
```

This places the `alloy` binary in your `$GOPATH/bin` directory. Make sure it is on your `PATH`:

```bash
export PATH="$PATH:$(go env GOPATH)/bin"
```

## Build from source

Clone the repository and build directly:

```bash
git clone https://github.com/zeroedin/alloy.git
cd alloy
go build ./cmd/alloy
```

This produces an `alloy` binary in the current directory. Move it somewhere on your `PATH`, or run it in place:

```bash
./alloy --version
```

## Next steps

Once installed, follow the [Quickstart](/getting-started/quickstart/) to build your first site.
