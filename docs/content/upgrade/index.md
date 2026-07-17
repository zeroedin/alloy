---
layout: doc
title: Upgrading Alloy
nav_weight: 90
description: "How to upgrade Alloy to the latest version for each install method."
---

Upgrade instructions for each install method. Run `alloy version` after upgrading to confirm the new version.

## Homebrew

```bash
brew update && brew upgrade alloy-ssg
```

`brew update` refreshes tap metadata first. Third-party taps like `zeroedin/alloy-ssg` can go stale without it — `brew upgrade` may report "already installed" even when a new version is available.

## Go install

```bash
go install github.com/zeroedin/alloy@latest
```

## Binary download

Download the latest release for your platform from the [GitHub releases page](https://github.com/zeroedin/alloy/releases/latest) and replace the existing binary in your `PATH`.

## From source

```bash
git pull origin main
go build -o alloy .
```

Move the built binary to your `PATH` if it is not already there.

## Verifying the upgrade

```bash
alloy version
```
