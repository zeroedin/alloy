package plugin_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/zeroedin/alloy/internal/plugin"
)

var _ = Describe("parseScopeJSON (issue #539)", func() {

	It("pages: false → PagesScopeNone", func() {
		scope, err := plugin.ParseScopeJSON(`{"pages": false}`)
		Expect(err).NotTo(HaveOccurred())
		Expect(scope.Pages.Mode).To(Equal(plugin.PagesScopeNone),
			"pages: false must produce PagesScopeNone — "+
				"hooks that check Pages.Mode (e.g. onContentTransformed) "+
				"skip page dispatch entirely; onPagesReady is a special case "+
				"that always sends the pages array for injection (issue #539)")
	})

	It("pages: true → PagesScopeAll", func() {
		scope, err := plugin.ParseScopeJSON(`{"pages": true}`)
		Expect(err).NotTo(HaveOccurred())
		Expect(scope.Pages.Mode).To(Equal(plugin.PagesScopeAll),
			"pages: true must produce PagesScopeAll — "+
				"plugins that declare pages: true receive all pages (issue #539)")
	})

	It("pages: \"**\" → PagesScopeAll", func() {
		scope, err := plugin.ParseScopeJSON(`{"pages": "**"}`)
		Expect(err).NotTo(HaveOccurred())
		Expect(scope.Pages.Mode).To(Equal(plugin.PagesScopeAll),
			"pages: \"**\" is a shorthand for all pages — "+
				"must produce PagesScopeAll, not PagesScopeGlob (issue #539)")
	})

	It("pages: \"/blog/**\" → PagesScopeGlob with Glob field", func() {
		scope, err := plugin.ParseScopeJSON(`{"pages": "/blog/**"}`)
		Expect(err).NotTo(HaveOccurred())
		Expect(scope.Pages.Mode).To(Equal(plugin.PagesScopeGlob),
			"a glob string other than \"**\" must produce PagesScopeGlob (issue #539)")
		Expect(scope.Pages.Glob).To(Equal("/blog/**"),
			"the Glob field must store the original pattern string (issue #539)")
	})

	It("pages: { tags: [\"component\"] } → PagesScopeTaxonomy with Taxonomies map", func() {
		scope, err := plugin.ParseScopeJSON(`{"pages": {"tags": ["component"]}}`)
		Expect(err).NotTo(HaveOccurred())
		Expect(scope.Pages.Mode).To(Equal(plugin.PagesScopeTaxonomy),
			"a map value for pages must produce PagesScopeTaxonomy (issue #539)")
		Expect(scope.Pages.Taxonomies).To(HaveKeyWithValue("tags", []string{"component"}),
			"the Taxonomies map must contain the parsed taxonomy → terms mapping (issue #539)")
	})

	It("pages: null (omitted) → PagesScopeAll (backward compat)", func() {
		scope, err := plugin.ParseScopeJSON(`{"data": ["elements"]}`)
		Expect(err).NotTo(HaveOccurred())
		Expect(scope.Pages.Mode).To(Equal(plugin.PagesScopeAll),
			"when pages is omitted (null), default to PagesScopeAll for "+
				"backward compatibility — existing plugins that don't declare "+
				"a pages scope must continue to receive all pages (issue #539)")
	})

	It("preserves data and pageFields arrays", func() {
		scope, err := plugin.ParseScopeJSON(`{"data": ["elements", "tokens"], "pages": true, "pageFields": ["frontMatter", "url"]}`)
		Expect(err).NotTo(HaveOccurred())
		Expect(scope.Data).To(Equal([]string{"elements", "tokens"}),
			"data array must be preserved verbatim from the JSON input (issue #539)")
		Expect(scope.PageFields).To(Equal([]string{"frontMatter", "url"}),
			"pageFields array must be preserved verbatim from the JSON input (issue #539)")
	})

	It("returns error on invalid JSON", func() {
		_, err := plugin.ParseScopeJSON(`{not json}`)
		Expect(err).To(HaveOccurred(),
			"parseScopeJSON must return an error for malformed JSON input (issue #539)")
	})
})

// ── parseScopeMap (issue #545) ─────────────────────────────────
// parseScopeMap builds a HookScope directly from a Go map without
// JSON serialization. It must handle the same polymorphic pages
// field as parseScopeJSON. parseScopeJSON should delegate to
// parseScopeMap after unmarshaling.

var _ = Describe("parseScopeMap (issue #545)", func() {

	It("pages: false → PagesScopeNone", func() {
		scope, err := plugin.ParseScopeMap(map[string]interface{}{
			"pages": false,
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(scope.Pages.Mode).To(Equal(plugin.PagesScopeNone),
			"parseScopeMap must handle bool false the same as parseScopeJSON (issue #545)")
	})

	It("pages: true → PagesScopeAll", func() {
		scope, err := plugin.ParseScopeMap(map[string]interface{}{
			"pages": true,
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(scope.Pages.Mode).To(Equal(plugin.PagesScopeAll),
			"parseScopeMap must handle bool true the same as parseScopeJSON (issue #545)")
	})

	It("pages: \"**\" → PagesScopeAll", func() {
		scope, err := plugin.ParseScopeMap(map[string]interface{}{
			"pages": "**",
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(scope.Pages.Mode).To(Equal(plugin.PagesScopeAll),
			"parseScopeMap must treat \"**\" as PagesScopeAll shorthand (issue #545)")
	})

	It("pages: \"/blog/**\" → PagesScopeGlob with Glob field", func() {
		scope, err := plugin.ParseScopeMap(map[string]interface{}{
			"pages": "/blog/**",
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(scope.Pages.Mode).To(Equal(plugin.PagesScopeGlob),
			"parseScopeMap must produce PagesScopeGlob for non-** glob strings (issue #545)")
		Expect(scope.Pages.Glob).To(Equal("/blog/**"),
			"the Glob field must store the original pattern (issue #545)")
	})

	It("pages: map with taxonomy terms → PagesScopeTaxonomy", func() {
		scope, err := plugin.ParseScopeMap(map[string]interface{}{
			"pages": map[string]interface{}{
				"tags": []interface{}{"component", "design-system"},
			},
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(scope.Pages.Mode).To(Equal(plugin.PagesScopeTaxonomy),
			"parseScopeMap must handle map[string]interface{} as taxonomy scope (issue #545)")
		Expect(scope.Pages.Taxonomies).To(HaveKeyWithValue("tags", []string{"component", "design-system"}),
			"taxonomy terms must be extracted from []interface{} to []string (issue #545)")
	})

	It("pages: nil (omitted) → PagesScopeAll (backward compat)", func() {
		scope, err := plugin.ParseScopeMap(map[string]interface{}{
			"data": []interface{}{"elements"},
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(scope.Pages.Mode).To(Equal(plugin.PagesScopeAll),
			"omitted pages key must default to PagesScopeAll — "+
				"backward compatibility with existing plugins (issue #545)")
	})

	It("preserves data and pageFields arrays", func() {
		scope, err := plugin.ParseScopeMap(map[string]interface{}{
			"data":       []interface{}{"elements", "tokens"},
			"pages":      true,
			"pageFields": []interface{}{"frontMatter", "url"},
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(scope.Data).To(Equal([]string{"elements", "tokens"}),
			"data array must be converted from []interface{} to []string (issue #545)")
		Expect(scope.PageFields).To(Equal([]string{"frontMatter", "url"}),
			"pageFields array must be converted from []interface{} to []string (issue #545)")
	})

	It("parseScopeJSON delegates to parseScopeMap (issue #545)", func() {
		jsonScope, err := plugin.ParseScopeJSON(`{"data": ["elements"], "pages": {"tags": ["go"]}, "pageFields": ["frontMatter"]}`)
		Expect(err).NotTo(HaveOccurred())

		mapScope, err := plugin.ParseScopeMap(map[string]interface{}{
			"data":       []interface{}{"elements"},
			"pages":      map[string]interface{}{"tags": []interface{}{"go"}},
			"pageFields": []interface{}{"frontMatter"},
		})
		Expect(err).NotTo(HaveOccurred())

		Expect(mapScope.Data).To(Equal(jsonScope.Data),
			"parseScopeMap and parseScopeJSON must produce identical Data (issue #545)")
		Expect(mapScope.Pages.Mode).To(Equal(jsonScope.Pages.Mode),
			"parseScopeMap and parseScopeJSON must produce identical Pages.Mode (issue #545)")
		Expect(mapScope.Pages.Taxonomies).To(Equal(jsonScope.Pages.Taxonomies),
			"parseScopeMap and parseScopeJSON must produce identical Taxonomies (issue #545)")
		Expect(mapScope.PageFields).To(Equal(jsonScope.PageFields),
			"parseScopeMap and parseScopeJSON must produce identical PageFields (issue #545)")
	})
})
