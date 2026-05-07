package plugin_test

import (
	"bytes"
	"context"
	"log"
	"os"

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
			batchFn := func(_ context.Context, ps []interface{}) ([]interface{}, error) {
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

	// ── Duplicate hook scope clobber (issue #544) ──────────────────
	// NodeRuntime.hookScopes is a map keyed by hook name. If a plugin
	// registers the same hook twice with different scopes, the last
	// registration silently overwrites the first. The fix must log a
	// warning so plugin authors get feedback.

	Describe("Duplicate hook scope clobber (issue #544)", func() {
		It("EvalFile must warn when overwriting hookScopes entry (issue #544)", func() {
			var logBuf bytes.Buffer
			log.SetOutput(&logBuf)
			defer log.SetOutput(os.Stderr)

			scopeA := &plugin.HookScope{
				Data:  []string{"elements"},
				Pages: plugin.PagesScope{Mode: plugin.PagesScopeGlob, Glob: "/blog/**"},
			}
			scopeB := &plugin.HookScope{
				Data:  []string{"tokens"},
				Pages: plugin.PagesScope{Mode: plugin.PagesScopeAll},
			}

			// Simulate what EvalFile does: set hookScopes twice for the same name.
			// After the fix, the second assignment must emit a warning.
			rt := plugin.NewNodeRuntimeWithHooks(
				[]string{"onContentTransformed", "onContentTransformed"},
				map[string]*plugin.HookScope{"onContentTransformed": scopeB},
				nil,
			)
			regs := rt.RegisteredHookDetails()
			Expect(regs).To(HaveLen(2),
				"duplicate hook names must produce two registrations (issue #544)")

			// The fix must add a warning when hookScopes[name] is overwritten.
			// Currently no warning is emitted — this assertion drives the fix.
			_ = scopeA
			Expect(logBuf.String()).To(ContainSubstring("duplicate"),
				"EvalFile must log a warning containing 'duplicate' when a hook scope "+
					"is overwritten by a second registration of the same hook name (issue #544)")
		})
	})
})
