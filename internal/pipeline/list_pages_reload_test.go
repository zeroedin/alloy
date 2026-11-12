package pipeline_test

import (
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/zeroedin/alloy/internal/config"
	"github.com/zeroedin/alloy/internal/pipeline"
)

// ── alloy dev keeps list and tag pages current (issue #1320) ───────────
// An incremental rebuild used to re-render only the pages that changed. A
// page that lists other pages (through collections or a taxonomy) kept the
// old list, and a tag page was never regenerated, until a full rebuild or a
// restart. The rule: when a page's front matter changes, or a page is added
// or removed, the rebuild re-renders every page and regenerates the
// taxonomy pages, as a full build does. A body-only edit stays incremental.
//
// These tests drive BuildIncremental the way cmd/dev.go does: one
// PipelineState, the previous build's cache, and the changed paths.
//
// See PLAN.md → "alloy dev keeps list and tag pages current (issue #1320)".

var _ = Describe("alloy dev keeps list and tag pages current (issue #1320)", func() {
	type site struct {
		root  string
		cfg   *config.Config
		state *pipeline.PipelineState
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
	newSite := func() site {
		GinkgoHelper()
		root := GinkgoT().TempDir()
		s := site{root: root}
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
		Expect(result.RenderedContent["index.md"]).To(ContainSubstring("[One]"),
			"sanity: the home page lists the blog collection")
		Expect(result.RenderedContent["/tags/a/"]).To(ContainSubstring("TAG a:"),
			"sanity: the first build generates the tag page")
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
})
