package pipeline_test

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/zeroedin/alloy/internal/config"
	"github.com/zeroedin/alloy/internal/pipeline"
)

// ── site.pages is built only when a template reads it (issue #1360) ────
// For every page it rendered, Alloy copied every page into a new list for
// site.pages, once per layout in the chain, whether or not the template
// read it. The work per page grew with the number of pages, so a build grew
// with the square of the page count: about 83% of a 1,000-page build.
//
// The rule:
//  1. A render that doesn't read site.pages does no per-page work for it.
//  2. When templates do read it, the list is built at most once per render
//     pass (content, layouts, extra formats, tag pages), not once per page.
//  3. The list is built again after any step that can change a page, such
//     as the plugin hooks that run between passes.
//  4. Everything a template can do with site and site.pages gives the same
//     output as before.
//
// See PLAN.md → "site.pages is built only when a template reads it
// (issue #1360)".

var _ = Describe("site.pages is built only when a template reads it (issue #1360)", func() {

	// ── Cost ────────────────────────────────────────────────────────
	// These count memory allocations, not time, so they give the same
	// answer on every machine. A site four times the size may allocate
	// about four times as much. Before the fix it allocated about eleven
	// times as much, because each page's render allocated a copy of every
	// page.

	buildAllocations := func(pages int, layout string) float64 {
		GinkgoHelper()
		files := map[string]string{"layouts/default.liquid": layout}
		for i := 0; i < pages; i++ {
			files[fmt.Sprintf("content/p%04d.md", i)] =
				fmt.Sprintf("---\ntitle: Page %d\nlayout: default\n---\nBody %d", i, i)
		}
		return testing.AllocsPerRun(2, func() {
			result, err := pipeline.BuildWithContent(&config.Config{
				Title:   "Lazy Site Pages",
				BaseURL: "https://example.com",
				Build:   config.BuildConfig{Output: "_site"},
			}, files)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.PageCount).To(Equal(pages), "sanity: every page is built")
		})
	}

	const (
		smallSite = 100
		largeSite = 400
		// largeSite is 4 times smallSite, so 4 is exact linear growth.
		// 5 leaves room for noise and is far below the old ratio of 11.
		maxGrowth = 5.0
	)

	Describe("the cost of a build grows in step with the number of pages", func() {
		It("when no template reads site.pages", func() {
			layout := "<h1>{{ page.title }}</h1>{{ content }}"
			small := buildAllocations(smallSite, layout)
			large := buildAllocations(largeSite, layout)

			Expect(large/small).To(BeNumerically("<", maxGrowth),
				"a site 4 times the size must not allocate much more than 4 times "+
					"as much (%d pages: %.0f allocations, %d pages: %.0f). A higher "+
					"ratio means each render still does work for every page, such "+
					"as building site.pages when no template reads it",
				smallSite, small, largeSite, large)
		})

		It("when every page's layout reads site.pages", func() {
			layout := "<h1>{{ page.title }}</h1>{{ site.pages | size }}{{ content }}"
			small := buildAllocations(smallSite, layout)
			large := buildAllocations(largeSite, layout)

			Expect(large/small).To(BeNumerically("<", maxGrowth),
				"a site 4 times the size must not allocate much more than 4 times "+
					"as much (%d pages: %.0f allocations, %d pages: %.0f). A higher "+
					"ratio means site.pages is built again for every page that "+
					"reads it, not once per render pass",
				smallSite, small, largeSite, large)
		})
	})

	// ── Guards: templates see what they saw before ──────────────────
	// These pass before the fix and must keep passing after it.

	Describe("templates see the same site and site.pages (guards)", func() {
		var root string

		write := func(rel, body string) {
			GinkgoHelper()
			path := filepath.Join(root, rel)
			Expect(os.MkdirAll(filepath.Dir(path), 0755)).To(Succeed())
			Expect(os.WriteFile(path, []byte(body), 0644)).To(Succeed())
		}

		newConfig := func() *config.Config {
			return &config.Config{
				Title:       "Probe",
				BaseURL:     "https://example.com",
				ProjectRoot: root,
				Build:       config.BuildConfig{Output: filepath.Join(root, "_site")},
				Structure: config.StructureConfig{
					Content: "content", Layouts: "layouts", Data: "data", Plugins: "plugins",
				},
				Collections: map[string]*config.CollectionConfig{"blog": {}},
				Taxonomies:  map[string]*config.TaxonomyConfig{"tags": {}},
			}
		}

		build := func(cfg *config.Config) *pipeline.BuildResult {
			GinkgoHelper()
			result, err := pipeline.Build(cfg, pipeline.BuildOptions{CaptureRenderedContent: true})
			Expect(err).NotTo(HaveOccurred())
			return result
		}

		// Three pages: the home page uses the probe layout, and two blog
		// posts. site.pages lists pages in path order: blog/a, blog/b, index.
		BeforeEach(func() {
			root = GinkgoT().TempDir()
			write("layouts/default.liquid", "{{ content }}")
			write("layouts/tags.liquid", "{{ taxonomy.term }}")
			write("content/index.md", "---\ntitle: Home\nlayout: probe\nweight: 3\n---\nhome")
			write("content/blog/a.md", "---\ntitle: A\nkind: doc\nweight: 2\ntags: [x]\n---\nA")
			write("content/blog/b.md", "---\ntitle: B\nkind: doc\nweight: 1\n---\nB")
		})

		It("reads site.pages in loops, lookups and filters", func() {
			write("layouts/probe.liquid",
				"LOOP[{% for p in site.pages %}{{ p.title }},{% endfor %}] "+
					"SIZE[{{ site.pages.size }}|{{ site.pages | size }}] "+
					"FIRST[{{ site.pages.first.title }}] LAST[{{ site.pages.last.title }}] "+
					"INDEX[{{ site.pages[1].title }}] URL[{{ site.pages.first.url }}] "+
					"ASSIGN[{% assign all = site.pages %}{{ all | size }}] "+
					"WHERE[{{ site.pages | where: \"kind\", \"doc\" | size }}] "+
					"SORT[{% assign sorted = site.pages | sort: \"weight\" %}{% for p in sorted %}{{ p.title }},{% endfor %}] "+
					"MAP[{{ site.pages | map: \"title\" | join: \"+\" }}]")

			out := build(newConfig()).RenderedContent["index.md"]
			Expect(out).To(ContainSubstring("LOOP[A,B,Home,]"))
			Expect(out).To(ContainSubstring("SIZE[3|3]"))
			Expect(out).To(ContainSubstring("FIRST[A] LAST[Home]"))
			Expect(out).To(ContainSubstring("INDEX[B] URL[/blog/a/]"))
			Expect(out).To(ContainSubstring("ASSIGN[3]"))
			Expect(out).To(ContainSubstring("WHERE[2]"))
			Expect(out).To(ContainSubstring("SORT[B,A,Home,]"))
			Expect(out).To(ContainSubstring("MAP[A+B+Home]"))
		})

		It("reads the other site keys", func() {
			write("data/nav.yaml", "home: /\n")
			write("layouts/probe.liquid",
				"TITLE[{{ site.title }}] BASE[{{ site.baseURL }}] DATA[{{ site.data.nav.home }}] "+
					"COLL[{{ site.collections.blog | size }}] LANG[{{ site.language }}] NONE[{{ site.nope }}]")

			out := build(newConfig()).RenderedContent["index.md"]
			Expect(out).To(ContainSubstring("TITLE[Probe] BASE[https://example.com] DATA[/]"))
			Expect(out).To(ContainSubstring("COLL[2]"))
			Expect(out).To(ContainSubstring("LANG[] NONE[]"),
				"a key that isn't set prints nothing")
		})

		It("passes site whole to a filter and loops over its keys", func() {
			write("layouts/probe.liquid",
				"JSON[{{ site | json }}] KEYS[{% for pair in site %}{{ pair[0] }};{% endfor %}]")

			out := build(newConfig()).RenderedContent["index.md"]
			Expect(out).To(ContainSubstring(`"baseURL":"https://example.com"`))
			Expect(out).To(ContainSubstring(`"title":"Probe"`))
			Expect(out).To(MatchRegexp(`"pages":\[.*"title":"A".*"title":"B".*"title":"Home"`),
				"site | json must still include the list of pages")
			Expect(out).To(ContainSubstring("KEYS[baseURL;collections;data;pages;title;]"),
				"looping over site must still give its keys, pages among them")
		})

		It("reads site.pages in a tag page and in another output format", func() {
			write("layouts/tags.liquid", "{% if taxonomy.term %}TAG[{{ site.pages | size }}]{% endif %}")
			write("layouts/probe.liquid", "{{ content }}")
			write("layouts/probe.json.liquid", `{"pages": {{ site.pages | size }}}`)
			write("content/index.md", "---\ntitle: Home\nlayout: probe\noutputs: [html, json]\n---\nhome")

			cfg := newConfig()
			result := build(cfg)
			Expect(result.RenderedContent["/tags/x/"]).To(ContainSubstring("TAG[3]"))
			Expect(result.FormatContent["index.md"]["json"]).To(Equal(`{"pages": 3}`))
		})

		// Rule 3. The page body renders before onContentLoaded runs, and the
		// layout renders after it. A list kept from the first pass would
		// miss what the hook added.
		It("shows front matter a hook added, in layouts but not in page bodies", func() {
			write("plugins/badge.js", `export default function(alloy) {
  alloy.hook('onContentLoaded', { pages: true, pageFields: ["*"] }, function(pages) {
    for (var i = 0; i < pages.length; i++) { pages[i].frontMatter.badge = 'hooked'; }
    return pages;
  });
}`)
			write("layouts/probe.liquid",
				"LAYOUT[{% for p in site.pages %}{{ p.badge }}|{% endfor %}] {{ content }}")
			write("content/index.md",
				"---\ntitle: Home\nlayout: probe\n---\nBODY[{% for p in site.pages %}{{ p.badge }}|{% endfor %}]")

			out := build(newConfig()).RenderedContent["index.md"]
			Expect(out).To(ContainSubstring("LAYOUT[hooked|hooked|hooked|]"),
				"a layout renders after onContentLoaded, so site.pages must "+
					"show the field the hook added to every page")
			Expect(out).To(ContainSubstring("BODY[|||]"),
				"a page body renders before onContentLoaded, as before")
		})

		It("lists every language's pages in a multilingual build", func() {
			write("layouts/default.liquid", "LIST[{% for p in site.pages %}{{ p.title }},{% endfor %}]")
			Expect(os.RemoveAll(filepath.Join(root, "content"))).To(Succeed())
			write("content/en/index.md", "---\ntitle: Home\n---\nhome")
			write("content/es/index.md", "---\ntitle: Inicio\n---\ninicio")
			cfg := newConfig()
			cfg.Collections = nil
			cfg.Taxonomies = nil
			cfg.Languages = map[string]*config.LanguageConfig{
				"en": {Root: true, Weight: 1},
				"es": {Weight: 2},
			}

			result := build(cfg)
			Expect(result.RenderedContent["en/index.md"]).To(ContainSubstring("LIST[Home,Inicio,]"))
			Expect(result.RenderedContent["es/index.md"]).To(ContainSubstring("LIST[Home,Inicio,]"))
		})

		It("lists the current pages in an incremental rebuild", func() {
			write("layouts/probe.liquid", "LIST[{% for p in site.pages %}{{ p.title }},{% endfor %}] {{ content }}")
			cfg := newConfig()
			config.ApplyDefaults(cfg)
			registry, hooks, _ := pipeline.DiscoverPlugins(cfg)
			DeferCleanup(registry.Close)
			state, err := pipeline.InitPipelineState(cfg, registry, hooks)
			Expect(err).NotTo(HaveOccurred())
			first := build(cfg)
			Expect(first.RenderedContent["index.md"]).To(ContainSubstring("LIST[A,B,Home,]"))

			// A body-only edit re-renders just the home page.
			write("content/index.md", "---\ntitle: Home\nlayout: probe\nweight: 3\n---\nhome edited")
			second, err := pipeline.BuildIncremental(cfg, nil, first.Cache, []string{"content/index.md"},
				pipeline.BuildOptions{PipelineState: state, CaptureRenderedContent: true})
			Expect(err).NotTo(HaveOccurred())
			Expect(second.RenderedContent["index.md"]).To(ContainSubstring("LIST[A,B,Home,]"))
			Expect(second.RenderedContent["index.md"]).To(ContainSubstring("home edited"))
		})
	})
})
