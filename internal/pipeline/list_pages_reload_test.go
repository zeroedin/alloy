package pipeline_test

import (
	"fmt"
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/zeroedin/alloy/internal/cache"
	"github.com/zeroedin/alloy/internal/config"
	"github.com/zeroedin/alloy/internal/pipeline"
)

// ── alloy dev keeps list and tag pages current (issue #1320) ───────────
// An incremental rebuild used to re-render only the pages that changed. A
// page that lists other pages (through collections or a taxonomy) kept the
// old list, and a tag page was never regenerated, until a full rebuild or a
// restart. The rule: on a site with a collection or a taxonomy, when a page's
// front matter changes, or a page is added or removed, the rebuild re-renders
// every page and regenerates the taxonomy pages, as a full build does. A site
// with neither, and a body-only edit, stay incremental.
//
// These tests drive BuildIncremental the way cmd/dev.go does: one
// PipelineState, the previous build's cache, and the changed paths.
//
// See PLAN.md → "List and tag pages stay current (issue #1320)".

var _ = Describe("alloy dev keeps list and tag pages current (issue #1320)", func() {
	type site struct {
		root  string
		cfg   *config.Config
		state *pipeline.PipelineState
		lists bool // the home page lists the blog collection
	}

	write := func(s site, rel, body string) {
		GinkgoHelper()
		path := filepath.Join(s.root, rel)
		Expect(os.MkdirAll(filepath.Dir(path), 0755)).To(Succeed())
		Expect(os.WriteFile(path, []byte(body), 0644)).To(Succeed())
	}

	post := func(title, tag, body string) string {
		return "---\ntitle: " + title + "\ndate: 2026-01-01\ntags: [\"" + tag + "\"]\n---\n" + body
	}

	// The home page lists collections.blog. Tag pages list their pages.
	// setup can change the files or config before the pipeline starts.
	newSite := func(setup ...func(s *site)) site {
		GinkgoHelper()
		root := GinkgoT().TempDir()
		s := site{root: root, lists: true}
		write(s, "layouts/default.liquid", "{{ content }}")
		write(s, "layouts/tags.liquid",
			"{% if taxonomy.term %}TAG {{ taxonomy.term }}:{% for p in taxonomy.pages %}[{{ p.title }}]{% endfor %}{% endif %}")
		write(s, "content/index.md",
			"---\ntitle: Home\n---\nLIST:{% for p in collections.blog %}[{{ p.title }}]{% endfor %}")
		write(s, "content/blog/one.md", post("One", "a", "first"))
		write(s, "content/blog/two.md", post("Two", "a", "second"))
		s.cfg = &config.Config{
			Title:       "List Pages",
			BaseURL:     "https://example.com",
			ProjectRoot: root,
			Build:       config.BuildConfig{Output: filepath.Join(root, "_site")},
			Structure:   config.StructureConfig{Content: "content", Layouts: "layouts"},
			Collections: map[string]*config.CollectionConfig{"blog": {}},
			Taxonomies:  map[string]*config.TaxonomyConfig{"tags": {}},
		}
		for _, f := range setup {
			f(&s)
		}
		config.ApplyDefaults(s.cfg)
		registry, hooks, _ := pipeline.DiscoverPlugins(s.cfg)
		DeferCleanup(registry.Close)
		state, err := pipeline.InitPipelineState(s.cfg, registry, hooks)
		Expect(err).NotTo(HaveOccurred())
		s.state = state
		return s
	}

	// alloy dev starts with a full Build, then rebuilds incrementally.
	firstBuild := func(s site) *pipeline.BuildResult {
		GinkgoHelper()
		result, err := pipeline.Build(s.cfg, pipeline.BuildOptions{CaptureRenderedContent: true})
		Expect(err).NotTo(HaveOccurred())
		if s.lists {
			Expect(result.RenderedContent["index.md"]).To(ContainSubstring("[One]"),
				"sanity: the home page lists the blog collection")
		}
		if s.cfg.Taxonomies != nil {
			Expect(result.RenderedContent["/tags/a/"]).To(ContainSubstring("TAG a:"),
				"sanity: the first build generates the tag page")
		}
		return result
	}

	rebuild := func(s site, prev *pipeline.BuildResult, changed ...string) *pipeline.BuildResult {
		GinkgoHelper()
		result, err := pipeline.BuildIncremental(s.cfg, nil, prev.Cache, changed, pipeline.BuildOptions{
			PipelineState:          s.state,
			CaptureRenderedContent: true,
		})
		Expect(err).NotTo(HaveOccurred())
		return result
	}

	page := func(s site, rel string) string {
		GinkgoHelper()
		b, err := os.ReadFile(filepath.Join(s.root, "_site", rel))
		Expect(err).NotTo(HaveOccurred(), "%s must exist in the output", rel)
		return string(b)
	}

	It("shows an edited title on the list page", func() {
		s := newSite()
		first := firstBuild(s)

		write(s, "content/blog/one.md", post("One edited", "a", "first"))
		result := rebuild(s, first, "content/blog/one.md")

		Expect(result.PagesRendered).To(ContainElement("index.md"),
			"a front matter change must re-render the pages that list it")
		Expect(page(s, "index.html")).To(ContainSubstring("[One edited]"))
	})

	It("adds a new page to the list", func() {
		s := newSite()
		first := firstBuild(s)

		write(s, "content/blog/three.md", post("Three", "a", "third"))
		rebuild(s, first, "content/blog/three.md")

		Expect(page(s, "index.html")).To(ContainSubstring("[Three]"),
			"an added page must appear on the list without a restart")
	})

	It("drops a deleted page from the list", func() {
		s := newSite()
		first := firstBuild(s)

		Expect(os.Remove(filepath.Join(s.root, "content/blog/two.md"))).To(Succeed())
		rebuild(s, first, "content/blog/two.md")

		Expect(page(s, "index.html")).NotTo(ContainSubstring("[Two]"),
			"a deleted page must drop off the list without a restart")
	})

	// HTML is a content format by default, so its pages are in the
	// collection too.
	It("drops a deleted HTML page from the list", func() {
		s := newSite(func(s *site) {
			write(*s, "content/blog/three.html", post("Three", "a", "<p>third</p>"))
		})
		first := firstBuild(s)
		Expect(page(s, "index.html")).To(ContainSubstring("[Three]"), "sanity: the HTML page is listed")

		Expect(os.Remove(filepath.Join(s.root, "content/blog/three.html"))).To(Succeed())
		rebuild(s, first, "content/blog/three.html")

		Expect(page(s, "index.html")).NotTo(ContainSubstring("[Three]"),
			"a deleted HTML page must drop off the list without a restart")
	})

	// The rebuild after a delete must forget the page, or a later rebuild
	// that brings back the same file sees nothing new.
	It("lists a page again when it is deleted and then recreated", func() {
		s := newSite()
		first := firstBuild(s)

		Expect(os.Remove(filepath.Join(s.root, "content/blog/two.md"))).To(Succeed())
		deleted := rebuild(s, first, "content/blog/two.md")
		Expect(page(s, "index.html")).NotTo(ContainSubstring("[Two]"), "sanity: the deleted page is dropped")

		write(s, "content/blog/two.md", post("Two", "a", "second"))
		rebuild(s, deleted, "content/blog/two.md")

		Expect(page(s, "index.html")).To(ContainSubstring("[Two]"),
			"a page recreated with the same content is a new page and must appear on the list again")
	})

	It("moves a page to its new tag page", func() {
		s := newSite()
		first := firstBuild(s)

		// Two posts use tag a, so tags/a still has a page after One moves.
		write(s, "content/blog/one.md", post("One", "b", "first"))
		rebuild(s, first, "content/blog/one.md")

		Expect(page(s, "tags/a/index.html")).NotTo(ContainSubstring("[One]"),
			"the old tag page must no longer list the page")
		Expect(page(s, "tags/b/index.html")).To(ContainSubstring("[One]"),
			"the new tag page must list the page")
	})

	It("generates a tag page for a new tag", func() {
		s := newSite()
		first := firstBuild(s)

		write(s, "content/blog/two.md", post("Two", "brand-new", "second"))
		rebuild(s, first, "content/blog/two.md")

		Expect(page(s, "tags/brand-new/index.html")).To(ContainSubstring("TAG brand-new:[Two]"),
			"a tag used for the first time must get its page without a restart")
	})

	// A tag page whose last page leaves is deleted, as a full build (which
	// empties the output folder first) would leave it out.
	It("deletes a tag page once no page has that tag", func() {
		s := newSite(func(s *site) {
			write(*s, "content/blog/two.md", post("Two", "z", "second"))
		})
		first := firstBuild(s)
		Expect(page(s, "tags/z/index.html")).To(ContainSubstring("TAG z:[Two]"), "sanity: the tag page exists")

		write(s, "content/blog/two.md", post("Two", "a", "second"))
		rebuild(s, first, "content/blog/two.md")

		_, err := os.Stat(filepath.Join(s.root, "_site", "tags/z/index.html"))
		Expect(os.IsNotExist(err)).To(BeTrue(),
			"no page has tag z any more, so its tag page must be deleted, not left listing a page that moved")
		Expect(page(s, "tags/a/index.html")).To(ContainSubstring("[Two]"))
	})

	// A plugin can build a virtual page from other pages' front matter, so
	// when the rule fires, onPagesReady virtual pages re-render too.
	It("re-renders a virtual page that lists the collection", func() {
		s := newSite(func(s *site) {
			write(*s, "plugins/virtual.js", `export default function(alloy) {
  alloy.hook('onPagesReady', { pages: false }, function() {
    return { addPages: [{
      path: '_virtual/v.html',
      url: '/v/',
      dependencies: [],
      frontMatter: { title: 'V', layout: 'default', markdown: false },
      content: 'V:{% for p in collections.blog %}[{{ p.title }}]{% endfor %}'
    }] };
  });
}`)
		})
		first := firstBuild(s)
		Expect(page(s, "v/index.html")).To(ContainSubstring("[One]"), "sanity: the virtual page lists the collection")

		write(s, "content/blog/one.md", post("One edited", "a", "first"))
		rebuild(s, first, "content/blog/one.md")

		Expect(page(s, "v/index.html")).To(ContainSubstring("[One edited]"),
			"a virtual page that lists the collection must show the new title")
	})

	// Guard: a draft left out of the build (alloy dev --no-drafts) has no
	// output, but it is still a page Alloy has seen. It must not count as
	// new on every rebuild.
	It("keeps a body-only edit incremental when a draft is left out", func() {
		s := newSite(func(s *site) {
			write(*s, "content/blog/draft.md",
				"---\ntitle: Draft\ndate: 2026-01-01\ndraft: true\ntags: [\"a\"]\n---\ndraft")
			s.cfg.IncludeDrafts = false
		})
		first := firstBuild(s)
		Expect(page(s, "index.html")).NotTo(ContainSubstring("[Draft]"), "sanity: the draft is left out")

		write(s, "content/blog/one.md", post("One", "a", "first, edited body"))
		result := rebuild(s, first, "content/blog/one.md")

		Expect(result.PagesRendered).To(ContainElement("blog/one.md"))
		Expect(result.PagesRendered).NotTo(ContainElement("index.md"),
			"a draft that is left out must not make every rebuild re-render the site")
	})

	// Guard: the rule must not make every edit a full re-render.
	It("keeps a body-only edit incremental", func() {
		s := newSite()
		first := firstBuild(s)

		write(s, "content/blog/one.md", post("One", "a", "first, edited body"))
		result := rebuild(s, first, "content/blog/one.md")

		Expect(result.PagesRendered).To(ContainElement("blog/one.md"))
		Expect(result.PagesRendered).NotTo(ContainElement("index.md"),
			"an edit that leaves the front matter unchanged must not re-render other pages")
	})

	It("counts a collection that comes from a date permalink", func() {
		// No collections or taxonomies in config. The blog section is a
		// collection because its permalink has date tokens.
		s := newSite(func(s *site) {
			s.cfg.Collections = nil
			s.cfg.Taxonomies = nil
			write(*s, "content/blog/_data.yaml", "permalink: \"/:year/:month/:slug/\"\n")
		})
		first := firstBuild(s)

		write(s, "content/blog/one.md", post("One edited", "a", "first"))
		result := rebuild(s, first, "content/blog/one.md")

		Expect(result.PagesRendered).To(ContainElement("index.md"),
			"a date-permalink section is a collection, so the list page must re-render")
		Expect(page(s, "index.html")).To(ContainSubstring("[One edited]"))
	})

	// Guard: with no collection and no taxonomy, no page can list another,
	// so a front matter change re-renders only the changed page.
	It("keeps a front matter edit incremental on a site with no collection or taxonomy", func() {
		s := newSite(func(s *site) {
			s.cfg.Collections = nil
			s.cfg.Taxonomies = nil
			s.lists = false
		})
		first := firstBuild(s)

		write(s, "content/blog/one.md", post("One edited", "a", "first"))
		result := rebuild(s, first, "content/blog/one.md")

		Expect(result.PagesRendered).To(ContainElement("blog/one.md"))
		Expect(result.PagesRendered).NotTo(ContainElement("index.md"),
			"with no collection or taxonomy, other pages must not re-render")
		Expect(result.PagesRendered).NotTo(ContainElement("blog/two.md"),
			"with no collection or taxonomy, other pages must not re-render")
	})

	// A cache written before front matter hashes existed has only content
	// hashes. Its front matter can't be compared, so it counts as changed:
	// one full re-render, then rebuilds are incremental again.
	It("re-renders list pages once from a cache with no front matter hashes", func() {
		s := newSite()
		firstBuild(s)

		// The old cache format: a content hash per page, nothing else.
		old := cache.New()
		for _, rel := range []string{"index.md", "blog/one.md", "blog/two.md"} {
			b, err := os.ReadFile(filepath.Join(s.root, "content", rel))
			Expect(err).NotTo(HaveOccurred())
			old.SetHash(rel, cache.HashContent(b))
		}

		// A body-only edit: only the missing front matter hash can make
		// this rebuild re-render the list page.
		write(s, "content/blog/one.md", post("One", "a", "first, edited body"))
		upgraded := rebuild(s, &pipeline.BuildResult{Cache: old}, "content/blog/one.md")
		Expect(upgraded.PagesRendered).To(ContainElement("index.md"),
			"without a previous front matter hash, the change can't be ruled out, so list pages must re-render")

		// The re-render wrote front matter hashes, so the next body-only
		// edit is incremental again.
		write(s, "content/blog/two.md", post("Two", "a", "second, edited body"))
		next := rebuild(s, upgraded, "content/blog/two.md")
		Expect(next.PagesRendered).NotTo(ContainElement("index.md"),
			"after one full re-render, a body-only edit must be incremental again")
	})

	// ── Tag pages in other cases (issue #1348) ───────────────────────────
	// Each case below was found in review of the #1320 code and checked
	// only by hand. These tests keep it fixed.

	// A tag page is regenerated when a broken _data.yaml is fixed, even if
	// no front matter changed in that rebuild: the pages lost their folder
	// data while it was broken, so the tag pages built then were wrong.
	It("regenerates tag pages when a broken _data.yaml is fixed", func() {
		s := newSite(func(s *site) {
			// The posts get their tag from the folder, not front matter.
			write(*s, "content/blog/_data.yaml", "tags: [\"a\"]\n")
			write(*s, "content/blog/one.md", "---\ntitle: One\ndate: 2026-01-01\n---\nfirst")
			write(*s, "content/blog/two.md", "---\ntitle: Two\ndate: 2026-01-01\n---\nsecond")
		})
		first := firstBuild(s)

		// One rebuild breaks the folder data and edits a title.
		write(s, "content/blog/_data.yaml", "tags: [\"a\"\n")
		write(s, "content/blog/one.md", "---\ntitle: One edited\ndate: 2026-01-01\n---\nfirst")
		broken := rebuild(s, first, "content/blog/_data.yaml", "content/blog/one.md")

		// The next rebuild only fixes the folder data.
		write(s, "content/blog/_data.yaml", "tags: [\"a\"]\n")
		rebuild(s, broken, "content/blog/_data.yaml")

		Expect(page(s, "tags/a/index.html")).To(ContainSubstring("[One edited]"),
			"once the folder data loads again, the tag page must be rebuilt with the pages' current titles")
	})

	// A content page on a tag page's URL is a conflict, as in a full build.
	It("reports a page whose URL is a tag page's URL", func() {
		s := newSite()
		first := firstBuild(s)

		write(s, "content/clash.md", "---\ntitle: Clash\npermalink: /tags/a/\n---\nclash")
		result := rebuild(s, first, "content/clash.md")

		Expect(fmt.Sprint(result.Errors)).To(ContainSubstring("output path conflict"),
			"the page and the tag page write the same path, so the rebuild must report it")
	})

	// With no previous cache, every page renders, so the tag pages must be
	// written too.
	It("writes tag pages when there is no previous cache", func() {
		s := newSite()

		_, err := pipeline.BuildIncremental(s.cfg, nil, nil, nil, pipeline.BuildOptions{PipelineState: s.state})
		Expect(err).NotTo(HaveOccurred())

		Expect(page(s, "tags/a/index.html")).To(ContainSubstring("TAG a:"),
			"a rebuild with no cache renders everything, as a full build does, tag pages included")
	})

	// newLangSite builds a site with English at the root and Spanish under
	// /es/. With withSpanish false, Spanish is configured but has no
	// content folder.
	newLangSite := func(withSpanish bool) site {
		GinkgoHelper()
		root := GinkgoT().TempDir()
		s := site{root: root}
		write(s, "layouts/default.liquid", "{{ content }}")
		write(s, "layouts/tags.liquid",
			"{% if taxonomy.term %}TAG {{ taxonomy.term }}:{% for p in taxonomy.pages %}[{{ p.title }}]{% endfor %}{% endif %}")
		write(s, "content/en/index.md", "---\ntitle: Home\n---\nhome")
		write(s, "content/en/blog/one.md", post("One", "a", "first"))
		if withSpanish {
			write(s, "content/es/blog/uno.md", post("Uno", "a", "primero"))
		}
		s.cfg = &config.Config{
			Title:       "Languages",
			BaseURL:     "https://example.com",
			ProjectRoot: root,
			Build:       config.BuildConfig{Output: filepath.Join(root, "_site")},
			Structure:   config.StructureConfig{Content: "content", Layouts: "layouts"},
			Taxonomies:  map[string]*config.TaxonomyConfig{"tags": {}},
			Languages: map[string]*config.LanguageConfig{
				"en": {Root: true, Weight: 1},
				"es": {Weight: 2},
			},
		}
		config.ApplyDefaults(s.cfg)
		registry, hooks, _ := pipeline.DiscoverPlugins(s.cfg)
		DeferCleanup(registry.Close)
		state, err := pipeline.InitPipelineState(s.cfg, registry, hooks)
		Expect(err).NotTo(HaveOccurred())
		s.state = state
		return s
	}

	langBuild := func(s site) *pipeline.BuildResult {
		GinkgoHelper()
		result, err := pipeline.Build(s.cfg)
		Expect(err).NotTo(HaveOccurred())
		return result
	}

	// Each language has its own tag pages, as in a full build.
	It("keeps each language's tag pages separate", func() {
		s := newLangSite(true)
		first := langBuild(s)
		Expect(page(s, "es/tags/a/index.html")).To(ContainSubstring("[Uno]"), "sanity: Spanish has its own tag page")

		write(s, "content/es/blog/uno.md", post("Uno editado", "a", "primero"))
		rebuild(s, first, "content/es/blog/uno.md")

		Expect(page(s, "es/tags/a/index.html")).To(ContainSubstring("[Uno editado]"),
			"the Spanish tag page must show the edited Spanish page")
		Expect(page(s, "tags/a/index.html")).To(ContainSubstring("[One]"))
		Expect(page(s, "tags/a/index.html")).NotTo(ContainSubstring("Uno"),
			"the English tag page must list only English pages")
	})

	// A language with no content folder gets no tag pages, as in a full
	// build.
	It("writes no tag pages for a language with no content folder", func() {
		s := newLangSite(false)
		first := langBuild(s)
		Expect(filepath.Join(s.root, "_site", "es", "tags")).NotTo(BeADirectory(),
			"sanity: a full build writes no Spanish tag pages")

		write(s, "content/en/blog/one.md", post("One edited", "a", "first"))
		rebuild(s, first, "content/en/blog/one.md")

		Expect(page(s, "tags/a/index.html")).To(ContainSubstring("[One edited]"))
		Expect(filepath.Join(s.root, "_site", "es", "tags")).NotTo(BeADirectory(),
			"Spanish has no content folder, so the rebuild must not write Spanish tag pages")
	})
})
