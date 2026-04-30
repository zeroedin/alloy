# Alloy SSG: Codebase vs Spec/Plan Audit

## Context

The Alloy implementation is feature-complete with 697 passing tests across 21 packages. This audit compares the running code on `main` against PLAN.md (the specification) and IMPLEMENTATION.md (the implementation guide) to find deviations — places where the code diverged from what the spec prescribes, or where the plan documents are stale.

Deviations fall into two categories:
- **Spec deviations** (code doesn't match PLAN.md) — need code fixes filed as issues
- **Plan drift** (IMPLEMENTATION.md is stale) — need plan updates

As the architect, I edit spec/plan/test files only. Code fixes are filed as issues for the developer.

---

## Priority 1: Behavioral Deviations (File Issues)

### A1: Feed generation not wired into pipeline
- **Spec**: PLAN.md §1d says feeds are opt-in via template placement. IMPLEMENTATION.md Phase 3D describes `ResolveFeedTemplates` and `RenderFeedTemplate`
- **Code**: `output/feed.go` has both functions implemented and tested. But `pipeline/build.go` **never calls them**. Additionally, `RenderFeedTemplate` uses manual string substitution instead of the template engine
- **Impact**: Users placing `feed.xml` in `layouts/` get no output
- **Files**: `internal/pipeline/build.go` (missing call), `internal/output/feed.go` (manual substitution)
- **Action**: File issue for (1) wiring `ResolveFeedTemplates` into Build() after layout rendering, (2) fixing `RenderFeedTemplate` to use the template engine

### A2: ApplyDefaults force-sets Build.Clean and TemplateTags
- **Spec**: PLAN.md says `clean: true` and `templateTags: true` are *defaults*, not forced values
- **Code**: `config/config.go:218` unconditionally sets `TemplateTags = true`, line 229 unconditionally sets `Build.Clean = true`. A user writing `clean: false` in their config gets silently overridden
- **Root cause**: Go's bool zero value is `false`, so unmarshaling can't distinguish "not set" from "explicitly false"
- **Files**: `internal/config/config.go` lines 218, 229
- **Action**: File issue. Fix: change fields to `*bool` so `nil` = "not set" and `ApplyDefaults` only sets when nil

### A3: Template engine config name mismatch — `"go"` vs `"gotemplate"`
- **Spec**: PLAN.md line 102 says `engine: "go"` for Go html/template
- **Code**: `pipeline/build.go:95` checks `cfg.Templates.Engine == "gotemplate"`, `output/formats.go:22` checks `engine == "gotemplate"`
- **Impact**: A user writing `engine: "go"` per the spec gets the Liquid engine (default fallback) instead of Go templates
- **Files**: `internal/pipeline/build.go:95`, `internal/output/formats.go:22`, `plans/PLAN.md:102`
- **Action**: File issue. Either update code to accept `"go"` (matching spec) or update PLAN.md to say `"gotemplate"` (matching code). Recommend code matches spec since PLAN.md is the user-facing document

### A4: `onDataCascadeReady` hook missing from PLAN.md lifecycle table
- **Spec**: PLAN.md lifecycle events table (lines 1735-1743) lists 9 hooks. `onDataCascadeReady` is NOT in the table but IS referenced in prose (line 1084) and IS implemented in `plugin/hooks.go` and called in `pipeline/build.go`
- **Impact**: Plugin authors consulting the lifecycle table won't know this hook exists
- **Files**: `plans/PLAN.md` lines 1735-1743
- **Action**: Add `onDataCascadeReady` to the lifecycle events table between `onContentLoaded` and `onContentTransformed`

---

## Priority 2: Plan Document Updates

### B1: IMPLEMENTATION.md test counts are stale
Every package has grown beyond original estimates. Update these:

| Package | IMPL.md Says | Actual | Delta |
|---------|-------------|--------|-------|
| `internal/data` | 8 | 9 | +1 |
| `internal/config` | 49 | 57 | +8 |
| `internal/content` | 75→(already updated) | 82 | +7 |
| `internal/permalink` | 22 | 28 | +6 |
| `internal/collection` | 21 | 27 | +6 |
| `internal/template` (3C) | ~27 | 39 | +12 |
| `internal/static` | 6 | 8 | +2 |
| `internal/plugin` | 58 | 59 | +1 |
| `internal/output` | 21 | 22 | +1 |
| `internal/server` | 45 | 48 | +3 |
| `cmd/` | 15 | 19 | +4 |
| `test/integration` | 16 | 23 | +7 |
| **Summary table total** | ~605 | **697** | **+92** |

**Files**: `plans/IMPLEMENTATION.md` — update test counts in section headers and summary table

### B2: Liquid engine evaluation language is stale
- IMPLEMENTATION.md Phase 0 references `osteele/liquid` as the dependency. Code uses `Notifuse/liquidgo` (go.mod line 7). The evaluation concluded and liquidgo won.
- IMPLEMENTATION.md Key Risks #1 says "Evaluate osteele/liquid vs Notifuse/liquidgo" — this is done
- **Files**: `plans/IMPLEMENTATION.md` Phase 0 (line 20), Key Risks #1 (line 309)
- **Action**: Update Phase 0 to show `Notifuse/liquidgo`, mark Key Risks #1 as resolved

### B3: Template engine test targets were aspirational
- IMPLEMENTATION.md targets ~20 Liquid and ~12 Go template tests. Actual: 14 Liquid, 7 Go template
- BUT total template package has 124 tests (far exceeds combined ~59 target). Coverage is distributed across filters (36), layout (19), context (13), etc.
- **Action**: Update Phase 4A/4B notes to reflect actual distribution

### B4: IMPLEMENTATION.md Phase 5D serve test note
- Main branch IMPLEMENTATION.md may have stale serve test note referencing `Execute()` instead of `Find()`. PR #77 changed the test but the note may not have been updated on main.
- **Files**: `plans/IMPLEMENTATION.md` Phase 5D serve section
- **Action**: Verify and update if stale

### B5: Document SSR marker simplification
- PLAN.md §8 describes hash-based SSR markers (`<!--alloy-ssr:hash-->`) for incremental rebuild optimization
- Code's `BuildPhase2` directly transforms custom elements without marker comments — correct output but no dedup/caching
- **Action**: Add note in IMPLEMENTATION.md that marker-based SSR dedup is deferred to post-v1

---

## Priority 3: Acceptable Drift (No Action Required)

These are technically deviations but architecturally correct:

- **C1**: Build step ordering — data loading runs once before language loop (not at step 7 position). Correct: data is global.
- **C2**: `ResolveLayoutWithCascade` exists but pipeline uses cascade flattening into FrontMatter instead. Both approaches are valid; flattening is simpler.
- **C3**: `BuildCollectionsWithMode` exists but pipeline pre-filters via `FilterByLifecycle` before calling `BuildCollections`. Same result, explicit is clearer.
- **C4**: `FeedConfig` type was dead code — removed in PR #426.
- **C5**: `--root` flag fully implemented on main (PR #90 merged). Previously suspected gap is resolved.

---

## Execution Plan

Work through items one at a time, creating separate issues/PRs per item:

1. **A1-A3**: File 3 issues for behavioral deviations (feed wiring, ApplyDefaults force-set, engine name mismatch)
2. **A4**: Update PLAN.md lifecycle table (add `onDataCascadeReady`)
3. **B1**: Update IMPLEMENTATION.md test counts and summary table
4. **B2-B5**: Update remaining stale IMPLEMENTATION.md sections

### Verification

After all updates:
- `go test ./...` — all 697 tests still pass (we only edit spec/plan files)
- `grep -c "It(" test files` counts match updated IMPLEMENTATION.md
- PLAN.md lifecycle table includes all hooks that exist in `plugin/hooks.go`

---

## Critical Files

| File | Changes Needed |
|------|---------------|
| `plans/PLAN.md` | A4 (lifecycle table), A3 (engine name, if we update spec side) |
| `plans/IMPLEMENTATION.md` | B1 (test counts), B2 (liquidgo), B3 (template targets), B4 (serve note), B5 (SSR note) |
| GitHub Issues | A1 (feed wiring), A2 (ApplyDefaults), A3 (engine name) |
