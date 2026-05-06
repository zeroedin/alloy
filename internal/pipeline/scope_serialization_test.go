package pipeline_test

import (
	"sort"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/zeroedin/alloy/internal/content"
	"github.com/zeroedin/alloy/internal/pipeline"
	"github.com/zeroedin/alloy/internal/plugin"
)

var _ = Describe("computeUnionScope (issue #539)", func() {

	It("returns nil for empty scopes slice", func() {
		result := pipeline.ComputeUnionScope([]*plugin.HookScope{})
		Expect(result).To(BeNil(),
			"an empty scopes slice means no hooks registered — "+
				"must return nil so the caller serializes the full payload (issue #539)")
	})

	It("returns nil when any scope is nil (unscoped hook)", func() {
		scoped := &plugin.HookScope{Data: []string{"elements"}}
		result := pipeline.ComputeUnionScope([]*plugin.HookScope{scoped, nil})
		Expect(result).To(BeNil(),
			"a nil scope means an unscoped hook that needs everything — "+
				"the union must collapse to nil (full payload) (issue #539)")
	})

	It("single scope passes through unchanged", func() {
		scope := &plugin.HookScope{
			Data:       []string{"elements"},
			PageFields: []string{"frontMatter"},
			Pages:      plugin.PagesScope{Mode: plugin.PagesScopeGlob, Glob: "/blog/**"},
		}
		result := pipeline.ComputeUnionScope([]*plugin.HookScope{scope})
		Expect(result).NotTo(BeNil())
		Expect(result.Data).To(Equal([]string{"elements"}))
		Expect(result.PageFields).To(Equal([]string{"frontMatter"}))
		Expect(result.Pages.Mode).To(Equal(plugin.PagesScopeGlob))
		Expect(result.Pages.Glob).To(Equal("/blog/**"),
			"a single scope must pass through without modification (issue #539)")
	})

	It("glob + taxonomy → PagesScopeAll", func() {
		glob := &plugin.HookScope{Pages: plugin.PagesScope{Mode: plugin.PagesScopeGlob, Glob: "/blog/**"}}
		taxonomy := &plugin.HookScope{Pages: plugin.PagesScope{
			Mode:       plugin.PagesScopeTaxonomy,
			Taxonomies: map[string][]string{"tags": {"go"}},
		}}
		result := pipeline.ComputeUnionScope([]*plugin.HookScope{glob, taxonomy})
		Expect(result).NotTo(BeNil())
		Expect(result.Pages.Mode).To(Equal(plugin.PagesScopeAll),
			"mixing glob and taxonomy modes forces the union to PagesScopeAll — "+
				"the pipeline cannot efficiently intersect these filter types (issue #539)")
	})

	It("identical globs stay as glob", func() {
		s1 := &plugin.HookScope{Pages: plugin.PagesScope{Mode: plugin.PagesScopeGlob, Glob: "/docs/**"}}
		s2 := &plugin.HookScope{Pages: plugin.PagesScope{Mode: plugin.PagesScopeGlob, Glob: "/docs/**"}}
		result := pipeline.ComputeUnionScope([]*plugin.HookScope{s1, s2})
		Expect(result).NotTo(BeNil())
		Expect(result.Pages.Mode).To(Equal(plugin.PagesScopeGlob))
		Expect(result.Pages.Glob).To(Equal("/docs/**"),
			"when all scopes use the same glob, the union preserves it (issue #539)")
	})

	It("different globs → PagesScopeAll", func() {
		s1 := &plugin.HookScope{Pages: plugin.PagesScope{Mode: plugin.PagesScopeGlob, Glob: "/blog/**"}}
		s2 := &plugin.HookScope{Pages: plugin.PagesScope{Mode: plugin.PagesScopeGlob, Glob: "/docs/**"}}
		result := pipeline.ComputeUnionScope([]*plugin.HookScope{s1, s2})
		Expect(result).NotTo(BeNil())
		Expect(result.Pages.Mode).To(Equal(plugin.PagesScopeAll),
			"different globs cannot be merged into a single pattern — "+
				"the union must fall back to PagesScopeAll (issue #539)")
	})

	It("PagesScopeAll dominates other modes", func() {
		all := &plugin.HookScope{Pages: plugin.PagesScope{Mode: plugin.PagesScopeAll}}
		glob := &plugin.HookScope{Pages: plugin.PagesScope{Mode: plugin.PagesScopeGlob, Glob: "/blog/**"}}
		result := pipeline.ComputeUnionScope([]*plugin.HookScope{glob, all})
		Expect(result).NotTo(BeNil())
		Expect(result.Pages.Mode).To(Equal(plugin.PagesScopeAll),
			"PagesScopeAll absorbs any narrower filter — "+
				"one hook needs all pages, so the union must include all (issue #539)")
	})

	It("field union: explicit fields from multiple scopes are merged", func() {
		s1 := &plugin.HookScope{PageFields: []string{"frontMatter"}}
		s2 := &plugin.HookScope{PageFields: []string{"html", "content"}}
		result := pipeline.ComputeUnionScope([]*plugin.HookScope{s1, s2})
		Expect(result).NotTo(BeNil())
		fields := make([]string, len(result.PageFields))
		copy(fields, result.PageFields)
		sort.Strings(fields)
		Expect(fields).To(Equal([]string{"content", "frontMatter", "html"}),
			"the union of explicit page fields from multiple scopes must include all "+
				"unique field names (issue #539)")
	})

	It("field union: nil PageFields (any scope) → nil (all fields)", func() {
		s1 := &plugin.HookScope{PageFields: []string{"frontMatter"}}
		s2 := &plugin.HookScope{PageFields: nil}
		result := pipeline.ComputeUnionScope([]*plugin.HookScope{s1, s2})
		Expect(result).NotTo(BeNil())
		Expect(result.PageFields).To(BeNil(),
			"nil PageFields means 'all fields' — if any scope has nil, "+
				"the union must be nil to include everything (issue #539)")
	})

	It("field union: [\"*\"] in any scope → nil (all fields)", func() {
		s1 := &plugin.HookScope{PageFields: []string{"frontMatter"}}
		s2 := &plugin.HookScope{PageFields: []string{"*"}}
		result := pipeline.ComputeUnionScope([]*plugin.HookScope{s1, s2})
		Expect(result).NotTo(BeNil())
		Expect(result.PageFields).To(BeNil(),
			"PageFields: [\"*\"] is equivalent to nil (all fields) — "+
				"the union must collapse to nil (issue #539)")
	})

	It("data union: explicit keys from multiple scopes are merged", func() {
		s1 := &plugin.HookScope{Data: []string{"elements"}}
		s2 := &plugin.HookScope{Data: []string{"tokens", "elements"}}
		result := pipeline.ComputeUnionScope([]*plugin.HookScope{s1, s2})
		Expect(result).NotTo(BeNil())
		data := make([]string, len(result.Data))
		copy(data, result.Data)
		sort.Strings(data)
		Expect(data).To(Equal([]string{"elements", "tokens"}),
			"data key union must include all unique keys from all scopes (issue #539)")
	})

	It("data union: [\"*\"] in any scope → [\"*\"]", func() {
		s1 := &plugin.HookScope{Data: []string{"elements"}}
		s2 := &plugin.HookScope{Data: []string{"*"}}
		result := pipeline.ComputeUnionScope([]*plugin.HookScope{s1, s2})
		Expect(result).NotTo(BeNil())
		Expect(result.Data).To(Equal([]string{"*"}),
			"Data: [\"*\"] means 'all siteData' — if any scope has \"*\", "+
				"the union must be [\"*\"] (issue #539)")
	})

	It("data union: all scopes with nil Data → nil", func() {
		s1 := &plugin.HookScope{Data: nil}
		s2 := &plugin.HookScope{Data: nil}
		result := pipeline.ComputeUnionScope([]*plugin.HookScope{s1, s2})
		Expect(result).NotTo(BeNil())
		Expect(result.Data).To(BeNil(),
			"when no scope requests siteData, the union must have nil Data (issue #539)")
	})
})

var _ = Describe("matchPageGlob (issue #539)", func() {

	It("double-star matches any suffix after prefix", func() {
		Expect(pipeline.MatchPageGlob("/blog/**", "/blog/2024/post")).To(BeTrue(),
			"\"**\" pattern must match any path starting with the prefix (issue #539)")
		Expect(pipeline.MatchPageGlob("/blog/**", "/docs/intro")).To(BeFalse(),
			"\"**\" pattern must not match paths that don't start with the prefix (issue #539)")
	})

	It("simple glob via path.Match", func() {
		Expect(pipeline.MatchPageGlob("/blog/*", "/blog/post")).To(BeTrue(),
			"single-star glob must match via path.Match semantics (issue #539)")
		Expect(pipeline.MatchPageGlob("/blog/*", "/blog/sub/post")).To(BeFalse(),
			"single-star glob must not match nested paths (issue #539)")
	})

	It("empty pattern matches nothing", func() {
		Expect(pipeline.MatchPageGlob("", "/any/path")).To(BeFalse(),
			"an empty glob pattern must not match any page (issue #539)")
	})

	It("exact path match", func() {
		Expect(pipeline.MatchPageGlob("/about", "/about")).To(BeTrue(),
			"an exact path string must match the identical URL (issue #539)")
		Expect(pipeline.MatchPageGlob("/about", "/contact")).To(BeFalse(),
			"an exact path string must not match a different URL (issue #539)")
	})
})

var _ = Describe("serializePagesForHook (issue #539)", func() {

	makePages := func() []*content.Page {
		return []*content.Page{
			{
				RelPath:      "content/blog/post.md",
				URL:          "/blog/post",
				FrontMatter:  map[string]interface{}{"title": "Blog Post"},
				Content:      []byte("raw content"),
				RenderedBody: []byte("<p>rendered</p>"),
			},
			{
				RelPath:      "content/docs/intro.md",
				URL:          "/docs/intro",
				FrontMatter:  map[string]interface{}{"title": "Intro"},
				Content:      []byte("doc content"),
				RenderedBody: []byte("<p>doc rendered</p>"),
			},
		}
	}

	It("PagesScopeNone returns nil", func() {
		scope := &plugin.HookScope{Pages: plugin.PagesScope{Mode: plugin.PagesScopeNone}}
		result := pipeline.SerializePagesForHook(makePages(), scope)
		Expect(result).To(BeNil(),
			"PagesScopeNone must produce nil — the plugin opted out of pages (issue #539)")
	})

	It("nil scope serializes all fields (backward compat)", func() {
		result := pipeline.SerializePagesForHook(makePages(), nil)
		Expect(result).To(HaveLen(2))
		Expect(result[0].Path).To(Equal("content/blog/post.md"))
		Expect(result[0].URL).To(Equal("/blog/post"))
		Expect(result[0].FrontMatter).NotTo(BeNil(),
			"nil scope must include frontMatter for backward compatibility (issue #539)")
		Expect(result[0].Content).To(Equal("raw content"),
			"nil scope must include content for backward compatibility (issue #539)")
		Expect(result[0].HTML).To(Equal("<p>rendered</p>"),
			"nil scope must include html for backward compatibility (issue #539)")
	})

	It("pageFields: [\"frontMatter\"] omits html and content", func() {
		scope := &plugin.HookScope{
			Pages:      plugin.PagesScope{Mode: plugin.PagesScopeAll},
			PageFields: []string{"frontMatter"},
		}
		result := pipeline.SerializePagesForHook(makePages(), scope)
		Expect(result).To(HaveLen(2))
		Expect(result[0].Path).To(Equal("content/blog/post.md"),
			"path is always included regardless of pageFields (issue #539)")
		Expect(result[0].URL).To(Equal("/blog/post"),
			"url is always included regardless of pageFields (issue #539)")
		Expect(result[0].FrontMatter).NotTo(BeNil(),
			"frontMatter must be included when listed in pageFields (issue #539)")
		Expect(result[0].Content).To(BeEmpty(),
			"content must be omitted when not in pageFields (issue #539)")
		Expect(result[0].HTML).To(BeEmpty(),
			"html must be omitted when not in pageFields (issue #539)")
	})

	It("pageFields: [\"html\"] omits frontMatter and content", func() {
		scope := &plugin.HookScope{
			Pages:      plugin.PagesScope{Mode: plugin.PagesScopeAll},
			PageFields: []string{"html"},
		}
		result := pipeline.SerializePagesForHook(makePages(), scope)
		Expect(result).To(HaveLen(2))
		Expect(result[0].HTML).To(Equal("<p>rendered</p>"),
			"html must be included when listed in pageFields (issue #539)")
		Expect(result[0].FrontMatter).To(BeNil(),
			"frontMatter must be omitted when not in pageFields (issue #539)")
		Expect(result[0].Content).To(BeEmpty(),
			"content must be omitted when not in pageFields (issue #539)")
	})

	It("glob filter excludes non-matching pages", func() {
		scope := &plugin.HookScope{
			Pages: plugin.PagesScope{Mode: plugin.PagesScopeGlob, Glob: "/blog/**"},
		}
		result := pipeline.SerializePagesForHook(makePages(), scope)
		Expect(result).To(HaveLen(1),
			"glob /blog/** must filter out pages outside /blog/ (issue #539)")
		Expect(result[0].URL).To(Equal("/blog/post"))
	})

	It("PagesScopeAll includes all pages", func() {
		scope := &plugin.HookScope{
			Pages: plugin.PagesScope{Mode: plugin.PagesScopeAll},
		}
		result := pipeline.SerializePagesForHook(makePages(), scope)
		Expect(result).To(HaveLen(2),
			"PagesScopeAll must include every page (issue #539)")
	})
})
