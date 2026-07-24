package plugin_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/zeroedin/alloy/internal/plugin"
)

var _ = Describe("ParseFileChangedResult (issue #1100)", func() {

	// ── onFileChanged return value processing ──────────────────────
	//
	// Plugins that transform page output based on external files have no
	// way to tell Alloy about those dependencies. The onFileChanged hook
	// changes from read-only to actionable: plugins can return
	// { invalidateByDependency: [...], restart: bool } to trigger
	// targeted incremental rebuilds and Node bridge restarts.
	//
	// ParseFileChangedResult extracts this structured data from the
	// hook's return value. It must be resilient to nil, non-map,
	// malformed, and partial return values.

	Describe("invalidateByDependency extraction", func() {
		It("extracts dependency paths from the return value", func() {
			result := map[string]interface{}{
				"invalidateByDependency": []interface{}{
					"elements/rh-card/rh-card.js",
					"elements/rh-icon/rh-icon.js",
				},
			}

			parsed := plugin.ParseFileChangedResult(result)
			Expect(parsed).NotTo(BeNil(),
				"ParseFileChangedResult must return a non-nil result when "+
					"the return value contains invalidateByDependency")
			Expect(parsed.InvalidateByDependency).To(ConsistOf(
				"elements/rh-card/rh-card.js",
				"elements/rh-icon/rh-icon.js",
			), "InvalidateByDependency must contain the exact dependency "+
				"paths returned by the plugin — these paths are used to "+
				"look up the reverse index and determine which pages to "+
				"rebuild (issue #1100)")
		})

		It("returns nil for unknown dependency paths", func() {
			// This tests that ParseFileChangedResult succeeds even when
			// the dependency paths don't match any tracked pages — the
			// lookup is the caller's responsibility, not the parser's.
			result := map[string]interface{}{
				"invalidateByDependency": []interface{}{
					"nonexistent/file.js",
				},
			}

			parsed := plugin.ParseFileChangedResult(result)
			Expect(parsed).NotTo(BeNil())
			Expect(parsed.InvalidateByDependency).To(ConsistOf("nonexistent/file.js"),
				"ParseFileChangedResult must extract paths as-is — validation "+
					"against the dependency cache happens in the dev server, "+
					"not in the parser (issue #1100)")
		})

		It("handles empty invalidateByDependency array", func() {
			result := map[string]interface{}{
				"invalidateByDependency": []interface{}{},
			}

			parsed := plugin.ParseFileChangedResult(result)
			Expect(parsed).NotTo(BeNil(),
				"empty invalidateByDependency array is valid (plugin examined "+
					"events but found no matching dependencies)")
			Expect(parsed.InvalidateByDependency).To(BeEmpty(),
				"InvalidateByDependency must be empty (not nil) when the "+
					"plugin returned an empty array")
		})

		It("filters out non-string entries with warning", func() {
			result := map[string]interface{}{
				"invalidateByDependency": []interface{}{
					"elements/rh-card/rh-card.js",
					42,
					true,
					"elements/rh-icon/rh-icon.js",
				},
			}

			parsed := plugin.ParseFileChangedResult(result)
			Expect(parsed).NotTo(BeNil())
			Expect(parsed.InvalidateByDependency).To(ConsistOf(
				"elements/rh-card/rh-card.js",
				"elements/rh-icon/rh-icon.js",
			), "non-string entries in invalidateByDependency must be "+
				"filtered out — only string paths are valid dependency "+
				"identifiers. The parser must not panic or reject the "+
				"entire array (issue #1100)")
			Expect(parsed.Warnings).To(ContainElement(
				ContainSubstring("non-string"),
			), "filtered non-string entries must produce a warning so "+
				"plugin authors can diagnose the issue")
		})
	})

	Describe("restart extraction", func() {
		It("extracts restart: true", func() {
			result := map[string]interface{}{
				"restart": true,
			}

			parsed := plugin.ParseFileChangedResult(result)
			Expect(parsed).NotTo(BeNil())
			Expect(parsed.Restart).To(BeTrue(),
				"restart: true tells Alloy to restart the Node bridge "+
					"subprocess before running the incremental rebuild, so "+
					"plugins re-import fresh module code instead of using "+
					"stale ESM cache (issue #1100)")
		})

		It("extracts restart: false", func() {
			result := map[string]interface{}{
				"restart": false,
			}

			parsed := plugin.ParseFileChangedResult(result)
			Expect(parsed).NotTo(BeNil())
			Expect(parsed.Restart).To(BeFalse(),
				"restart: false is an explicit opt-out — plugins that read "+
					"dependency files via fs.readFile() do not need a bridge "+
					"restart (issue #1100)")
		})

		It("defaults restart to false when omitted", func() {
			result := map[string]interface{}{
				"invalidateByDependency": []interface{}{"a.js"},
			}

			parsed := plugin.ParseFileChangedResult(result)
			Expect(parsed).NotTo(BeNil())
			Expect(parsed.Restart).To(BeFalse(),
				"restart must default to false when not present in the "+
					"return value — most dependency changes don't require "+
					"a Node bridge restart (issue #1100)")
		})
	})

	Describe("combined fields", func() {
		It("extracts both invalidateByDependency and restart", func() {
			result := map[string]interface{}{
				"invalidateByDependency": []interface{}{
					"elements/rh-icon/rh-icon.js",
				},
				"restart": true,
			}

			parsed := plugin.ParseFileChangedResult(result)
			Expect(parsed).NotTo(BeNil())
			Expect(parsed.InvalidateByDependency).To(ConsistOf(
				"elements/rh-icon/rh-icon.js",
			), "invalidateByDependency must be extracted alongside restart")
			Expect(parsed.Restart).To(BeTrue(),
				"restart must be extracted alongside invalidateByDependency — "+
					"the SSR use case needs both: invalidate pages that used "+
					"the changed component AND restart the Node bridge to "+
					"clear the ESM module cache (issue #1100)")
		})
	})

	Describe("nil and non-map inputs", func() {
		It("returns nil for nil input", func() {
			parsed := plugin.ParseFileChangedResult(nil)
			Expect(parsed).To(BeNil(),
				"nil result means the plugin returned nothing (read-only "+
					"observation) — no action needed (issue #1100)")
		})

		It("returns nil for string input", func() {
			parsed := plugin.ParseFileChangedResult("some string")
			Expect(parsed).To(BeNil(),
				"string return value is not a valid onFileChanged result — "+
					"treat as read-only observation for forward compatibility "+
					"(issue #1100)")
		})

		It("returns nil for boolean input", func() {
			parsed := plugin.ParseFileChangedResult(true)
			Expect(parsed).To(BeNil(),
				"boolean return value is not valid — treat as no-op")
		})

		It("returns nil for map without recognized keys", func() {
			result := map[string]interface{}{
				"unrecognized": "value",
				"another":      42,
			}

			parsed := plugin.ParseFileChangedResult(result)
			Expect(parsed).To(BeNil(),
				"a map without invalidateByDependency or restart is not "+
					"actionable — return nil for forward compatibility so "+
					"new return fields from future spec revisions don't "+
					"cause errors (issue #1100)")
		})

		It("ignores unknown keys alongside recognized keys", func() {
			result := map[string]interface{}{
				"invalidateByDependency": []interface{}{"a.js"},
				"restart":               true,
				"futureField":           "value",
			}

			parsed := plugin.ParseFileChangedResult(result)
			Expect(parsed).NotTo(BeNil(),
				"unknown keys alongside recognized keys must not prevent "+
					"extraction of recognized fields")
			Expect(parsed.InvalidateByDependency).To(ConsistOf("a.js"))
			Expect(parsed.Restart).To(BeTrue())
		})
	})

	Describe("malformed invalidateByDependency", func() {
		It("treats non-array invalidateByDependency as no-op with warning", func() {
			result := map[string]interface{}{
				"invalidateByDependency": "not-an-array",
			}

			parsed := plugin.ParseFileChangedResult(result)
			// Non-array invalidateByDependency is a plugin bug, but the
			// parser should be lenient — log a warning, don't crash.
			// parsed may be nil (if restart is also absent) or non-nil
			// with nil InvalidateByDependency.
			if parsed != nil {
				Expect(parsed.InvalidateByDependency).To(BeNil(),
					"non-array invalidateByDependency must not produce "+
						"dependency paths — the value is malformed")
				Expect(parsed.Warnings).To(ContainElement(
					ContainSubstring("invalidateByDependency"),
				), "malformed invalidateByDependency must produce a warning")
			}
		})

		It("treats numeric invalidateByDependency as no-op with warning", func() {
			result := map[string]interface{}{
				"invalidateByDependency": 42,
				"restart":               true,
			}

			parsed := plugin.ParseFileChangedResult(result)
			Expect(parsed).NotTo(BeNil(),
				"restart: true makes the result actionable even when "+
					"invalidateByDependency is malformed")
			Expect(parsed.InvalidateByDependency).To(BeNil(),
				"numeric invalidateByDependency must not produce paths")
			Expect(parsed.Restart).To(BeTrue(),
				"restart extraction must succeed independently of "+
					"invalidateByDependency extraction")
			Expect(parsed.Warnings).To(ContainElement(
				ContainSubstring("invalidateByDependency"),
			), "malformed invalidateByDependency must produce a warning")
		})
	})
})
