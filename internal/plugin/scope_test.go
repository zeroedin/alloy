package plugin_test

import (
	"context"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/zeroedin/alloy/internal/plugin"
)

var _ = Describe("Declarative hook payload scoping (issue #528)", func() {

	// ── HookScope struct ───────────────────────────────────────────
	// Plugins must declare what data they need at registration time.
	// The pipeline uses the scope to serialize only the requested subset.

	Describe("HookScope struct", func() {
		It("exists with Data, Pages, and PageFields fields", func() {
			scope := plugin.HookScope{
				Data: []string{"elements", "tokens"},
				Pages: plugin.PagesScope{
					Mode: plugin.PagesScopeAll,
				},
				PageFields: []string{"frontMatter", "url"},
			}

			Expect(scope.Data).To(Equal([]string{"elements", "tokens"}),
				"Data field must hold siteData key names (issue #528)")
			Expect(scope.Pages.Mode).To(Equal(plugin.PagesScopeAll),
				"Pages field must be PagesScope with Mode (issue #528)")
			Expect(scope.PageFields).To(Equal([]string{"frontMatter", "url"}),
				"PageFields field must hold per-page field names (issue #528)")
		})
	})

	// ── PagesScopeMode constants ────────────────────────────────────

	Describe("PagesScopeMode constants", func() {
		It("PagesScopeNone, PagesScopeAll, PagesScopeGlob, PagesScopeTaxonomy are distinct", func() {
			modes := []plugin.PagesScopeMode{
				plugin.PagesScopeNone,
				plugin.PagesScopeAll,
				plugin.PagesScopeGlob,
				plugin.PagesScopeTaxonomy,
			}
			seen := make(map[plugin.PagesScopeMode]bool)
			for _, m := range modes {
				Expect(seen[m]).To(BeFalse(),
					"each PagesScopeMode constant must have a distinct value (issue #528)")
				seen[m] = true
			}
		})

		It("PagesScopeNone is the zero value", func() {
			var mode plugin.PagesScopeMode
			Expect(mode).To(Equal(plugin.PagesScopeNone),
				"PagesScopeNone must be iota zero — unset PagesScope defaults to skip pages (issue #528)")
		})
	})

	// ── PagesScope struct ──────────────────────────────────────────

	Describe("PagesScope struct", func() {
		It("represents glob scope with Mode and Glob fields", func() {
			ps := plugin.PagesScope{
				Mode: plugin.PagesScopeGlob,
				Glob: "/blog/**",
			}
			Expect(ps.Mode).To(Equal(plugin.PagesScopeGlob))
			Expect(ps.Glob).To(Equal("/blog/**"),
				"Glob field must hold the path pattern for Go-side filtering (issue #528)")
		})

		It("represents taxonomy scope with Mode and Taxonomies fields", func() {
			ps := plugin.PagesScope{
				Mode: plugin.PagesScopeTaxonomy,
				Taxonomies: map[string][]string{
					"tags":     {"component", "form"},
					"category": {"ui"},
				},
			}
			Expect(ps.Mode).To(Equal(plugin.PagesScopeTaxonomy))
			Expect(ps.Taxonomies).To(HaveKeyWithValue("tags", []string{"component", "form"}),
				"multiple terms within same taxonomy are OR'd — union (issue #528)")
			Expect(ps.Taxonomies).To(HaveKeyWithValue("category", []string{"ui"}),
				"multiple taxonomies are AND'd — intersection (issue #528)")
		})
	})

	// ── RegisterWithOptions ────────────────────────────────────────

	Describe("RegisterWithOptions", func() {
		It("registers hook that executes via RunWithTimeout", func() {
			registry := plugin.NewHookRegistry()
			scope := plugin.HookScope{
				Data: []string{"elements"},
			}
			called := false
			fn := func(_ context.Context, payload interface{}) (interface{}, error) {
				called = true
				return "scoped-result", nil
			}
			registry.RegisterWithOptions(plugin.OnPagesReady, fn, scope, 50)

			Expect(registry.HasHooks(plugin.OnPagesReady)).To(BeTrue(),
				"HasHooks must return true after RegisterWithOptions (issue #528)")

			result, err := registry.RunWithTimeout(plugin.OnPagesReady, "input")
			Expect(err).NotTo(HaveOccurred())
			Expect(called).To(BeTrue(), "hook registered via RegisterWithOptions must be callable")
			Expect(result).To(Equal("scoped-result"))
		})

		It("preserves priority ordering with scoped hooks", func() {
			registry := plugin.NewHookRegistry()
			var order []string

			fn1 := func(_ context.Context, p interface{}) (interface{}, error) {
				order = append(order, "high")
				return p, nil
			}
			fn2 := func(_ context.Context, p interface{}) (interface{}, error) {
				order = append(order, "low")
				return p, nil
			}

			scope := plugin.HookScope{Pages: plugin.PagesScope{Mode: plugin.PagesScopeAll}}
			registry.RegisterWithOptions(plugin.OnContentLoaded, fn1, scope, 100)
			registry.RegisterWithOptions(plugin.OnContentLoaded, fn2, scope, 10)

			_, err := registry.RunWithTimeout(plugin.OnContentLoaded, nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(order).To(Equal([]string{"low", "high"}),
				"lower priority must run first, same as RegisterWithPriority (issue #528)")
		})
	})

	// ── RegisterBatchWithOptions ───────────────────────────────────

	Describe("RegisterBatchWithOptions", func() {
		It("registers batch-capable hook with scope", func() {
			registry := plugin.NewHookRegistry()
			scope := plugin.HookScope{
				Pages:      plugin.PagesScope{Mode: plugin.PagesScopeAll},
				PageFields: []string{"html"},
			}
			singleFn := func(_ context.Context, p interface{}) (interface{}, error) {
				return p, nil
			}
			batchCalled := false
			batchFn := func(_ context.Context, ps []interface{}, _ plugin.BatchProgressFunc) ([]interface{}, error) {
				batchCalled = true
				return ps, nil
			}
			registry.RegisterBatchWithOptions(plugin.OnPageRendered, singleFn, batchFn, scope, 50)

			Expect(registry.HasHooks(plugin.OnPageRendered)).To(BeTrue())

			_, err := registry.RunBatchWithTimeout(plugin.OnPageRendered, []interface{}{"<html>"})
			Expect(err).NotTo(HaveOccurred())
			Expect(batchCalled).To(BeTrue(),
				"batch function must be called via RunBatchWithTimeout (issue #528)")
		})
	})

	// ── ScopeFor ───────────────────────────────────────────────────

	Describe("ScopeFor", func() {
		It("returns scope for registered hooks in priority order", func() {
			registry := plugin.NewHookRegistry()
			scope := plugin.HookScope{
				Data:  []string{"elements"},
				Pages: plugin.PagesScope{Mode: plugin.PagesScopeNone},
			}
			fn := func(_ context.Context, p interface{}) (interface{}, error) {
				return p, nil
			}
			registry.RegisterWithOptions(plugin.OnPagesReady, fn, scope, 50)

			scopes := registry.ScopeFor(plugin.OnPagesReady)
			Expect(scopes).To(HaveLen(1),
				"ScopeFor must return one scope per registered hook (issue #528)")
			Expect(scopes[0]).NotTo(BeNil())
			Expect(scopes[0].Data).To(Equal([]string{"elements"}))
			Expect(scopes[0].Pages.Mode).To(Equal(plugin.PagesScopeNone))
		})

		It("returns nil for events with no hooks", func() {
			registry := plugin.NewHookRegistry()
			scopes := registry.ScopeFor(plugin.OnPagesReady)
			Expect(scopes).To(BeNil(),
				"ScopeFor must return nil when no hooks registered (issue #528)")
		})

		It("returns nil scope entries for unscoped hooks registered via Register", func() {
			registry := plugin.NewHookRegistry()
			fn := func(_ context.Context, p interface{}) (interface{}, error) {
				return p, nil
			}
			registry.Register(plugin.OnContentLoaded, fn)

			scopes := registry.ScopeFor(plugin.OnContentLoaded)
			Expect(scopes).To(HaveLen(1),
				"ScopeFor must return entries for all hooks including unscoped (issue #528)")
			Expect(scopes[0]).To(BeNil(),
				"unscoped hooks (registered via Register) must have nil scope (issue #528)")
		})
	})

	// ── Multiple hooks with independent scopes ─────────────────────

	Describe("Multiple hooks with independent scopes", func() {
		It("two hooks on same event have distinct scopes retrievable via ScopeFor in priority order", func() {
			registry := plugin.NewHookRegistry()
			fn := func(_ context.Context, p interface{}) (interface{}, error) {
				return p, nil
			}

			scope1 := plugin.HookScope{
				Data:  []string{"elements"},
				Pages: plugin.PagesScope{Mode: plugin.PagesScopeNone},
			}
			scope2 := plugin.HookScope{
				Data:       []string{"*"},
				Pages:      plugin.PagesScope{Mode: plugin.PagesScopeAll},
				PageFields: []string{"frontMatter"},
			}

			// Register higher priority first to prove ScopeFor returns priority order, not insertion order
			registry.RegisterWithOptions(plugin.OnContentLoaded, fn, scope2, 20)
			registry.RegisterWithOptions(plugin.OnContentLoaded, fn, scope1, 10)

			scopes := registry.ScopeFor(plugin.OnContentLoaded)
			Expect(scopes).To(HaveLen(2))

			Expect(scopes[0].Data).To(Equal([]string{"elements"}),
				"first scope (priority 10) must have Data=[elements] (issue #528)")
			Expect(scopes[0].Pages.Mode).To(Equal(plugin.PagesScopeNone))

			Expect(scopes[1].Data).To(Equal([]string{"*"}),
				"second scope (priority 20) must have Data=[*] (issue #528)")
			Expect(scopes[1].Pages.Mode).To(Equal(plugin.PagesScopeAll))
			Expect(scopes[1].PageFields).To(Equal([]string{"frontMatter"}))
		})
	})

	// ── ValidateScope ──────────────────────────────────────────────

	Describe("ValidateScope", func() {

		// ── Pageless hook rejection ──────────────────────────────────

		It("rejects PagesScopeAll on pageless hooks", func() {
			scope := plugin.HookScope{
				Pages: plugin.PagesScope{Mode: plugin.PagesScopeAll},
			}
			for _, event := range []plugin.HookName{
				plugin.OnConfig,
				plugin.OnBeforeValidation,
				plugin.OnAfterValidation,
				plugin.OnDataFetched,
				plugin.OnAssetProcess,
				plugin.OnBuildComplete,
				plugin.OnDevServerStart,
				plugin.OnFileChanged,
			} {
				err := plugin.ValidateScope(event, scope)
				Expect(err).To(HaveOccurred(),
					"PagesScopeAll on pageless hook %s must error — hook has no pages (issue #528)", event)
				Expect(err.Error()).To(ContainSubstring("pages"),
					"error must mention pages (issue #528)")
			}
		})

		It("rejects PagesScopeGlob on pageless hooks", func() {
			scope := plugin.HookScope{
				Pages: plugin.PagesScope{
					Mode: plugin.PagesScopeGlob,
					Glob: "/blog/**",
				},
			}
			for _, event := range []plugin.HookName{
				plugin.OnConfig,
				plugin.OnBeforeValidation,
				plugin.OnAfterValidation,
				plugin.OnDataFetched,
				plugin.OnAssetProcess,
				plugin.OnBuildComplete,
				plugin.OnDevServerStart,
				plugin.OnFileChanged,
			} {
				err := plugin.ValidateScope(event, scope)
				Expect(err).To(HaveOccurred(),
					"PagesScopeGlob on pageless hook %s must error — hook has no pages (issue #528)", event)
			}
		})

		It("rejects PagesScopeTaxonomy on pageless hooks", func() {
			scope := plugin.HookScope{
				Pages: plugin.PagesScope{
					Mode:       plugin.PagesScopeTaxonomy,
					Taxonomies: map[string][]string{"tags": {"component"}},
				},
			}
			for _, event := range []plugin.HookName{
				plugin.OnConfig,
				plugin.OnBeforeValidation,
				plugin.OnAfterValidation,
				plugin.OnDataFetched,
				plugin.OnAssetProcess,
				plugin.OnBuildComplete,
				plugin.OnDevServerStart,
				plugin.OnFileChanged,
			} {
				err := plugin.ValidateScope(event, scope)
				Expect(err).To(HaveOccurred(),
					"PagesScopeTaxonomy on pageless hook %s must error (issue #528)", event)
			}
		})

		// ── Pre-taxonomy rejection (hooks that have pages but no taxonomy indices) ──

		It("rejects taxonomy filtering on onPagesReady", func() {
			scope := plugin.HookScope{
				Pages: plugin.PagesScope{
					Mode:       plugin.PagesScopeTaxonomy,
					Taxonomies: map[string][]string{"tags": {"component"}},
				},
			}
			err := plugin.ValidateScope(plugin.OnPagesReady, scope)
			Expect(err).To(HaveOccurred(),
				"taxonomy filtering on onPagesReady must error — taxonomy indices not built yet (issue #528)")
			Expect(err.Error()).To(ContainSubstring("taxonomy"),
				"error must mention taxonomy (issue #528)")
		})

		// ── Post-taxonomy acceptance ──────────────────────────────────

		It("accepts taxonomy filtering on post-taxonomy hooks", func() {
			scope := plugin.HookScope{
				Pages: plugin.PagesScope{
					Mode:       plugin.PagesScopeTaxonomy,
					Taxonomies: map[string][]string{"tags": {"component"}},
				},
			}
			for _, event := range []plugin.HookName{
				plugin.OnContentLoaded,
				plugin.OnDataCascadeReady,
				plugin.OnContentTransformed,
				plugin.OnPageRendered,
			} {
				err := plugin.ValidateScope(event, scope)
				Expect(err).NotTo(HaveOccurred(),
					"taxonomy filtering on post-taxonomy hook %s must be valid (issue #528)", event)
			}
		})

		It("accepts glob filtering on onPagesReady", func() {
			scope := plugin.HookScope{
				Pages: plugin.PagesScope{
					Mode: plugin.PagesScopeGlob,
					Glob: "/blog/**",
				},
			}
			err := plugin.ValidateScope(plugin.OnPagesReady, scope)
			Expect(err).NotTo(HaveOccurred(),
				"glob filtering is valid on page-aware hooks — no dependency on taxonomy indices (issue #528)")
		})

		It("accepts PagesScopeNone on any hook", func() {
			scope := plugin.HookScope{
				Pages: plugin.PagesScope{Mode: plugin.PagesScopeNone},
			}
			for _, event := range []plugin.HookName{
				plugin.OnConfig,
				plugin.OnBeforeValidation,
				plugin.OnAfterValidation,
				plugin.OnDataFetched,
				plugin.OnAssetProcess,
				plugin.OnBuildComplete,
				plugin.OnDevServerStart,
				plugin.OnFileChanged,
				plugin.OnPagesReady,
				plugin.OnContentLoaded,
				plugin.OnDataCascadeReady,
				plugin.OnContentTransformed,
				plugin.OnPageRendered,
			} {
				err := plugin.ValidateScope(event, scope)
				Expect(err).NotTo(HaveOccurred(),
					"PagesScopeNone (skip pages) must be valid on all hooks (issue #528)")
			}
		})

		It("accepts PagesScopeAll on hooks that receive pages", func() {
			scope := plugin.HookScope{
				Pages: plugin.PagesScope{Mode: plugin.PagesScopeAll},
			}
			for _, event := range []plugin.HookName{
				plugin.OnPagesReady,
				plugin.OnContentLoaded,
				plugin.OnDataCascadeReady,
				plugin.OnContentTransformed,
				plugin.OnPageRendered,
			} {
				err := plugin.ValidateScope(event, scope)
				Expect(err).NotTo(HaveOccurred(),
					"PagesScopeAll must be valid on hooks that receive pages (issue #528)")
			}
		})
	})

	// ── HasHooks with scoped hooks ─────────────────────────────────

	Describe("HasHooks with scoped hooks", func() {
		It("returns true when only scoped hooks are registered", func() {
			registry := plugin.NewHookRegistry()
			scope := plugin.HookScope{
				Data: []string{"elements"},
			}
			fn := func(_ context.Context, p interface{}) (interface{}, error) {
				return p, nil
			}
			registry.RegisterWithOptions(plugin.OnPagesReady, fn, scope, 50)

			Expect(registry.HasHooks(plugin.OnPagesReady)).To(BeTrue(),
				"HasHooks must work with scoped hooks — they are still hooks (issue #528)")
			Expect(registry.HasHooks(plugin.OnContentLoaded)).To(BeFalse(),
				"HasHooks must return false for events with no hooks (issue #528)")
		})
	})

	// ── Duplicate hookScope clobber warning (issue #544) ─────────────
	// When a plugin registers the same hook name twice with different
	// scopes, bridge.js must detect the duplicate at registration time
	// and include a warning in the eval response. Go surfaces it via
	// EvalWarnings(). Last-wins semantics are preserved.

	Describe("Duplicate hookScope clobber warning (issue #544)", func() {
		It("EvalFile warns when a plugin registers the same hook twice", func() {
			rt := plugin.NewNodeRuntime()
			DeferCleanup(rt.Close)

			err := rt.EvalFile(filepath.Join(testdataDir(), "single-files", "duplicate-hook.js"))
			Expect(err).NotTo(HaveOccurred(),
				"EvalFile must succeed even when a plugin registers duplicate hooks — "+
					"duplicate registration is a warning, not an error (issue #544)")

			warnings := rt.EvalWarnings()
			Expect(warnings).NotTo(BeEmpty(),
				"EvalWarnings must contain at least one warning when a plugin "+
					"registers the same hook name twice with different scopes — "+
					"currently returns nil because bridge.js does not detect duplicates (issue #544)")
			Expect(warnings[0]).To(ContainSubstring("duplicate"),
				"warning message must mention 'duplicate' to identify the problem (issue #544)")
			Expect(warnings[0]).To(ContainSubstring("onContentTransformed"),
				"warning message must include the hook name (issue #544)")
		})

		It("last registration wins for hookScopes on duplicate", func() {
			rt := plugin.NewNodeRuntime()
			DeferCleanup(rt.Close)

			err := rt.EvalFile(filepath.Join(testdataDir(), "single-files", "duplicate-hook.js"))
			Expect(err).NotTo(HaveOccurred())

			details := rt.RegisteredHookDetails()
			var found *plugin.HookRegistration
			for i := range details {
				if details[i].Name == "onContentTransformed" {
					found = &details[i]
					break
				}
			}
			Expect(found).NotTo(BeNil(),
				"onContentTransformed must be registered even with duplicate declarations (issue #544)")
			Expect(found.Scope).NotTo(BeNil(),
				"scope must be present for duplicate-registered hook (issue #544)")
			Expect(found.Scope.Pages.Mode).To(Equal(plugin.PagesScopeAll),
				"last registration must win — second registration used pages: true "+
					"(PagesScopeAll), not pages: false (PagesScopeNone) from the first (issue #544)")
		})
	})

	// ── Within-plugin hook dedup (issue #555) ────────────────────────
	// EvalFile appends hook names to r.hooks unconditionally. If the
	// bridge response (or a future protocol change) delivers duplicate
	// hook names, RegisteredHookDetails must deduplicate so each hook
	// fires once, not N times.

	Describe("Within-plugin hook dedup (issue #555)", func() {
		It("RegisteredHookDetails deduplicates hook names", func() {
			rt := plugin.NewNodeRuntime()
			plugin.AppendHook(rt, "onContentTransformed")
			plugin.AppendHook(rt, "onContentTransformed")

			details := rt.RegisteredHookDetails()
			count := 0
			for _, d := range details {
				if d.Name == "onContentTransformed" {
					count++
				}
			}
			Expect(count).To(Equal(1),
				"RegisteredHookDetails must deduplicate within-plugin hook names — "+
					"two entries for the same name must produce one registration, not two (issue #555)")
		})
	})

	// ── QuickJS duplicate hookScope clobber warning (issue #558) ─────
	// Mirrors the Node runtime tests from issue #544 for the Tier 2
	// QuickJS runtime. PR #560 adds EvalWarnings() and duplicate
	// detection in __registerHook. These tests validate that path.

	Describe("QuickJS duplicate hookScope clobber warning (issue #558)", func() {
		It("EvalFile warns when a QuickJS plugin registers the same hook twice", func() {
			rt := plugin.NewQuickJSRuntime()
			DeferCleanup(rt.Close)
			Expect(rt.Init()).To(Succeed())

			err := rt.EvalFile(filepath.Join(testdataDir(), "single-files", "duplicate-hook.js"))
			Expect(err).NotTo(HaveOccurred(),
				"EvalFile must succeed even when a plugin registers duplicate hooks — "+
					"duplicate registration is a warning, not an error (issue #558)")

			warnings := rt.EvalWarnings()
			Expect(warnings).NotTo(BeEmpty(),
				"EvalWarnings must contain at least one warning when a QuickJS plugin "+
					"registers the same hook name twice with different scopes (issue #558)")
			Expect(warnings[0]).To(ContainSubstring("duplicate"),
				"warning message must mention 'duplicate' to identify the problem (issue #558)")
			Expect(warnings[0]).To(ContainSubstring("onContentTransformed"),
				"warning message must include the hook name (issue #558)")
		})

		It("last registration wins for hookScopes on duplicate in QuickJS", func() {
			rt := plugin.NewQuickJSRuntime()
			DeferCleanup(rt.Close)
			Expect(rt.Init()).To(Succeed())

			err := rt.EvalFile(filepath.Join(testdataDir(), "single-files", "duplicate-hook.js"))
			Expect(err).NotTo(HaveOccurred())

			details := rt.RegisteredHookDetails()
			var found *plugin.HookRegistration
			for i := range details {
				if details[i].Name == "onContentTransformed" {
					found = &details[i]
					break
				}
			}
			Expect(found).NotTo(BeNil(),
				"onContentTransformed must be registered even with duplicate declarations (issue #558)")
			Expect(found.Scope).NotTo(BeNil(),
				"scope must be present for duplicate-registered hook (issue #558)")
			Expect(found.Scope.Pages.Mode).To(Equal(plugin.PagesScopeAll),
				"last registration must win — second registration used pages: true "+
					"(PagesScopeAll), not pages: false (PagesScopeNone) from the first (issue #558)")
		})

		It("registerRuntime forwards QuickJS EvalWarnings to HookRegistry.Warnings via EvalWarner", func() {
			rt := plugin.NewQuickJSRuntime()
			DeferCleanup(rt.Close)
			Expect(rt.Init()).To(Succeed())

			err := rt.EvalFile(filepath.Join(testdataDir(), "single-files", "duplicate-hook.js"))
			Expect(err).NotTo(HaveOccurred())

			Expect(rt.EvalWarnings()).NotTo(BeEmpty(),
				"precondition: QuickJS runtime must have eval warnings from duplicate hooks")

			hooks := plugin.NewHookRegistry()
			registry := plugin.NewRegistry(testdataDir())
			plugin.RegisterRuntime(registry, rt, "test-plugin", hooks)

			Expect(hooks.Warnings()).NotTo(BeEmpty(),
				"registerRuntime must forward QuickJS EvalWarnings to HookRegistry.Warnings — "+
					"the EvalWarner interface check in registerRuntime must detect that "+
					"QuickJSRuntime implements EvalWarnings() and copy the warnings (issue #558)")
			Expect(hooks.Warnings()[0]).To(ContainSubstring("duplicate"),
				"forwarded warning must preserve the original duplicate warning content (issue #558)")
			Expect(hooks.Warnings()[0]).To(ContainSubstring("test-plugin"),
				"forwarded warning must include the plugin name for debugging (issue #558)")
		})

		It("duplicate detection fires on __registerHook 1-arg fallback path", func() {
			rt := plugin.NewQuickJSRuntime()
			DeferCleanup(rt.Close)
			Expect(rt.Init()).To(Succeed())

			err := rt.EvalFile(filepath.Join(testdataDir(), "single-files", "duplicate-hook-no-scope.js"))
			Expect(err).NotTo(HaveOccurred(),
				"EvalFile must succeed for 1-arg __registerHook calls — "+
					"duplicate registration is a warning, not an error (issue #558)")

			warnings := rt.EvalWarnings()
			Expect(warnings).NotTo(BeEmpty(),
				"EvalWarnings must detect duplicates even when __registerHook is called "+
					"with a single argument (no priority, no scope) — the 1-arg fallback path "+
					"must trigger the same duplicate detection as the 3-arg path (issue #558)")
			Expect(warnings[0]).To(ContainSubstring("duplicate"),
				"warning must mention 'duplicate' on the 1-arg path (issue #558)")
			Expect(warnings[0]).To(ContainSubstring("onContentTransformed"),
				"warning must include the hook name on the 1-arg path (issue #558)")
		})
	})

	// ── Omitted pages scope defaults (issue #977) ────────────────────
	// parseScopeMap defaults omitted/nil pages to PagesScopeAll, but
	// the documented contract is pages: false (PagesScopeNone). This
	// causes two compounding defects:
	// 1. Every hook registered with {} or {data: [...]} on a pageless
	//    event triggers a spurious ValidateScope warning on every build.
	// 2. Batch hooks silently serialize all pages across the bridge
	//    when the plugin never requested pages — the exact cost the
	//    docs say the default avoids.

	Describe("Omitted pages scope defaults (issue #977)", func() {

		It("parseScopeMap with empty map defaults pages to PagesScopeNone without Explicit", func() {
			scope, err := plugin.ParseScopeMap(map[string]interface{}{})
			Expect(err).NotTo(HaveOccurred())
			Expect(scope.Pages.Mode).To(Equal(plugin.PagesScopeNone),
				"empty scope map {} must default pages to PagesScopeNone — "+
					"omitting a scope option must not opt into maximum serialization (issue #977)")
			Expect(scope.Pages.Explicit).To(BeFalse(),
				"empty scope {} must not mark pages as Explicit — "+
					"per-page hooks should still fire for hooks that did not explicitly "+
					"opt out of pages (issue #1054)")
		})

		It("parseScopeJSON with empty object defaults pages to PagesScopeNone", func() {
			scope, err := plugin.ParseScopeJSON(`{}`)
			Expect(err).NotTo(HaveOccurred())
			Expect(scope.Pages.Mode).To(Equal(plugin.PagesScopeNone),
				"empty scope JSON {} must default pages to PagesScopeNone (issue #977)")
		})

		It("empty scope on each pageless hook passes ValidateScope without error", func() {
			// This is the core bug: {} on a pageless hook produces a
			// warning because nil → All → ValidateScope error. With the
			// fix (nil → None), ValidateScope returns nil for all pageless hooks.
			scope, err := plugin.ParseScopeMap(map[string]interface{}{})
			Expect(err).NotTo(HaveOccurred())

			pagelessHooks := []plugin.HookName{
				plugin.OnConfig,
				plugin.OnBeforeValidation,
				plugin.OnAfterValidation,
				plugin.OnDataFetched,
				plugin.OnAssetProcess,
				plugin.OnBuildComplete,
				plugin.OnDevServerStart,
				plugin.OnFileChanged,
			}
			for _, event := range pagelessHooks {
				err := plugin.ValidateScope(event, *scope)
				Expect(err).NotTo(HaveOccurred(),
					"empty scope {} on pageless hook %s must not produce a validation error — "+
						"omitted pages defaults to PagesScopeNone which is valid on all hooks (issue #977)", event)
			}
		})

		It("data-only scope on each pageless hook passes ValidateScope without error", func() {
			// Mirrors the documented pattern: {data: ["elements"]} on onDataFetched.
			// Must not warn — the data key does not set pages mode.
			scope, err := plugin.ParseScopeMap(map[string]interface{}{
				"data": []interface{}{"elements"},
			})
			Expect(err).NotTo(HaveOccurred())

			pagelessHooks := []plugin.HookName{
				plugin.OnConfig,
				plugin.OnBeforeValidation,
				plugin.OnAfterValidation,
				plugin.OnDataFetched,
				plugin.OnAssetProcess,
				plugin.OnBuildComplete,
				plugin.OnDevServerStart,
				plugin.OnFileChanged,
			}
			for _, event := range pagelessHooks {
				err := plugin.ValidateScope(event, *scope)
				Expect(err).NotTo(HaveOccurred(),
					"data-only scope on pageless hook %s must not produce a validation error — "+
						"the data key does not set pages mode (issue #977)", event)
			}
		})

		It("explicit pages: true on pageless hook still produces a validation error", func() {
			// Disambiguation: the fix changes the *default* for omitted pages,
			// not the behavior of explicit pages: true. A plugin that explicitly
			// requests pages on a pageless hook is still invalid.
			scope, err := plugin.ParseScopeMap(map[string]interface{}{
				"pages": true,
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(scope.Pages.Mode).To(Equal(plugin.PagesScopeAll),
				"explicit pages: true must still produce PagesScopeAll")
			Expect(scope.Pages.Explicit).To(BeTrue(),
				"explicit pages: true must set Explicit = true (issue #1054)")

			err = plugin.ValidateScope(plugin.OnConfig, *scope)
			Expect(err).To(HaveOccurred(),
				"explicit pages: true on pageless hook must still produce a validation error — "+
					"only the omitted/nil default changes, not explicit declarations (issue #977)")
			Expect(err.Error()).To(ContainSubstring("pages"),
				"validation error for explicit pages: true must mention pages (issue #977)")
		})

		It("omitted pages on page-aware batch hook defaults to PagesScopeNone", func() {
			// The default applies universally, not just to pageless hooks.
			// A plugin on onContentLoaded with {data: ["elements"]} must
			// not receive pages unless it explicitly says pages: true.
			scope, err := plugin.ParseScopeMap(map[string]interface{}{
				"data": []interface{}{"elements"},
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(scope.Pages.Mode).To(Equal(plugin.PagesScopeNone),
				"omitted pages on page-aware hook must default to PagesScopeNone — "+
					"plugins that want pages must explicitly declare pages: true (issue #977)")

			// PagesScopeNone is valid on page-aware hooks too — it just
			// means the plugin doesn't need pages for this hook.
			err = plugin.ValidateScope(plugin.OnContentLoaded, *scope)
			Expect(err).NotTo(HaveOccurred(),
				"PagesScopeNone must be valid on page-aware hooks (issue #977)")
		})
	})
})
