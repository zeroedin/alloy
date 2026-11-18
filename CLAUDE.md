# Working on Alloy

## Conventional Commits
Commit messages and PR titles follow [Conventional Commits](https://www.conventionalcommits.org/):
`type(scope): summary (#issue)`, with the summary in lowercase and the imperative mood.

- Types: `feat`, `fix`, `perf`, `docs`, `test`, `spec`, `refactor`, `ci`, `chore`.
- The scope is optional: the area changed, such as `dev`, `serve` or `plugin`.
- Examples:
  - `fix(dev): keep list and tag pages current (#1320)`
  - `spec: build site.pages only when a template reads it (#1360)`
  - `docs: note the site.pages gap in alloy dev (#1320)`

## Test-driven development
- Tests come first. A change starts with tests that fail without it.
- A change is done when those tests pass and `go test -race ./...` passes.
- Never skip, disable or loosen a test to get a change through.
- `go vet` and `gofmt` are clean before a push.

## GitHub
- Every change has an issue. Its PR says `Closes #N`.
- A PR does one thing.
- Merge with a merge commit; don't rewrite history on a shared branch.
- Read the whole review before replying, including findings outside the diff.
  Answer each thread, and resolve it once addressed.
- A PR into a branch other than `main` doesn't close its issue: close it by hand,
  with a comment.

## Changesets and docs
- Every user-visible change has a changeset: `.changeset/<name>.md`, with `type: patch`,
  `minor` or `major`.
- Examples in docs and changesets come from a real build.
