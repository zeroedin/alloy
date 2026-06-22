# Changesets

This directory contains changeset files that describe unreleased changes.

## Creating a changeset

Run the dev tool:

```sh
go run ./tools/changeset
```

Or create a file manually in this directory with any `.md` filename:

```markdown
---
type: patch
---

Description of the change.
```

## Bump types

- `patch` — bug fixes, minor improvements
- `minor` — new features, non-breaking additions
- `major` — breaking changes

When multiple changesets are pending, the highest bump type wins for the version calculation.

## When to add a changeset

Add one when your PR includes user-facing changes (features, fixes, breaking changes). Documentation, CI, and chore changes don't need a changeset.
