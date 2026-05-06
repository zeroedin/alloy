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
				"plugins that declare pages: false opt out of receiving "+
				"any page data in the hook payload (issue #539)")
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
