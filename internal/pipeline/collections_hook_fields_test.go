package pipeline_test

import (
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/zeroedin/alloy/internal/config"
	"github.com/zeroedin/alloy/internal/pipeline"
	"github.com/zeroedin/alloy/internal/plugin"
)

// ── Hook fields show in collections (issue #1385) ───────────────────────
// In a layout, every list of pages shows each page with the fields
// onContentLoaded and onDataCascadeReady add, as site.pages and
// taxonomy.pages already do:
//   - collections.<name>;
//   - taxonomies.<name>.<term>;
//   - pagination.items on a page that paginates a collection.
//
// Which pages a list holds, and their order, are still decided before the
// hooks, from front matter after onPagesReady. A hook that changes tags or
// date changes what a page shows, not where it is.
//
// Liquid in a page's own content is rendered before the hooks, so no list
// shows their fields there.
//
// Before, these three lists held copies of each page made before the hooks,
// so a field a hook added was empty in them.
//
// See PLAN.md → "Hook fields show in collections".
var _ = Describe("Hook fields show in collections (issue #1385)", func() {
	const node = "export const runtime = \"node\";\n"

	// each prints, for every page in a list, its title and the fields the
	// two hooks add: A=LA,DA; when both hooks reached page A.
	each := func(list string) string {
		return "{% for p in " + list + " %}{{ p.title }}={{ p.loaded }},{{ p.cascaded }};{% endfor %}"
	}

	// The hooks add a field to every page, from the page's own title, so a
	// page that shows another page's fields fails too. onContentLoaded also
	// moves post C to the newest date and adds the tag y: neither may move
	// C in any list.
	const hooks = "export default function(alloy) {\n" +
		"  alloy.hook('onContentLoaded', { pages: true }, (pages) => pages.map(p => {\n" +
		"    const fm = { ...p.frontMatter, loaded: 'L' + p.frontMatter.title };\n" +
		"    if (p.frontMatter.title === 'C') { fm.date = '2026-02-01'; fm.tags = ['x', 'y']; }\n" +
		"    return { ...p, frontMatter: fm };\n" +
		"  }));\n" +
		"  alloy.hook('onDataCascadeReady', { pages: true }, (pages) =>\n" +
		"    pages.map(p => ({ ...p, data: { cascaded: 'D' + p.data.title } })));\n" +
		"}\n"

	// Three posts in a blog collection, newest first: A, B, C. File names
	// sort the same way, so the order holds whether or not the collection is
	// sorted by date. All are tagged x. /list/ paginates the collection two
	// to a page.
	files := func(hookSrc string) map[string]string {
		return map[string]string{
			"content/index.md": "---\ntitle: Home\nlayout: home\n---\n" +
				"BODY[" + each("collections.blog") + "]",
			"content/list.md":         "---\ntitle: List\nlayout: list\npagination:\n  data: collections.blog\n  perPage: 2\n---\nlist",
			"content/blog/_data.yaml": "permalink: \"/blog/:year/:slug/\"",
			"content/blog/a.md":       "---\ntitle: A\ndate: 2026-01-03\ntags: [x]\nlayout: post\n---\na",
			"content/blog/b.md":       "---\ntitle: B\ndate: 2026-01-02\ntags: [x]\nlayout: post\n---\nb",
			"content/blog/c.md":       "---\ntitle: C\ndate: 2026-01-01\ntags: [x]\nlayout: post\n---\nc",
			"layouts/home.liquid": "COLL[" + each("collections.blog") + "] " +
				"TAXG[" + each("taxonomies.tags.x") + "] " +
				"TAGY[" + each("taxonomies.tags.y") + "] " +
				"SITE[{% for p in site.pages %}{% if p.layout == 'post' %}{{ p.title }}={{ p.loaded }},{{ p.cascaded }};{% endif %}{% endfor %}] " +
				"{{ content }}",
			"layouts/list.liquid":            "PAG[" + each("pagination.items") + "]",
			"layouts/post.liquid":            "{{ content }}",
			"layouts/taxonomies/tags.liquid": "TAXP[" + each("taxonomy.pages") + "]",
			"plugins/hooks.js":               hookSrc,
		}
	}

	newConfig := func() *config.Config {
		return &config.Config{
			Title:      "Collections Hook Fields",
			BaseURL:    "https://example.com",
			Build:      config.BuildConfig{Output: "_site"},
			Taxonomies: map[string]*config.TaxonomyConfig{"tags": {}},
		}
	}

	for _, runtime := range []string{"QuickJS", "Node"} {
		rt := runtime
		Describe("in a full build on "+rt, Ordered, ContinueOnFailure, func() {
			var out map[string]string
			BeforeAll(func() {
				src := hooks
				if rt == "Node" {
					if plugin.CheckNodeAvailable() != nil {
						Skip("node is not installed")
					}
					src = node + hooks
				}
				result, err := pipeline.BuildWithContent(newConfig(), files(src), pipeline.BuildOptions{CaptureRenderedContent: true})
				Expect(err).NotTo(HaveOccurred())
				out = result.RenderedContent
			})

			// Guards: the lists that show hook fields today.
			It("shows hook fields in site.pages (guard)", func() {
				Expect(out["index.md"]).To(ContainSubstring("C=LC,DC;"))
				Expect(out["index.md"]).To(ContainSubstring("B=LB,DB;"))
				Expect(out["index.md"]).To(ContainSubstring("A=LA,DA;"))
			})
			It("shows hook fields in taxonomy.pages on a tag page (guard)", func() {
				Expect(out["/tags/x/"]).To(ContainSubstring("TAXP[A=LA,DA;B=LB,DB;C=LC,DC;]"))
			})

			It("shows hook fields in collections, in the collection's order", func() {
				Expect(out["index.md"]).To(ContainSubstring("COLL[A=LA,DA;B=LB,DB;C=LC,DC;]"))
			})
			It("shows hook fields in taxonomies.<name>.<term>", func() {
				Expect(out["index.md"]).To(ContainSubstring("TAXG[A=LA,DA;B=LB,DB;C=LC,DC;]"))
			})
			It("shows hook fields in pagination.items over a collection, on every page", func() {
				Expect(out["/list/"]).To(ContainSubstring("PAG[A=LA,DA;B=LB,DB;]"))
				Expect(out["/list/page/2/"]).To(ContainSubstring("PAG[C=LC,DC;]"))
			})

			// Guards: a hook's new date and tag change neither membership
			// nor order. These hold today and must keep holding.
			It("keeps a page in its place when a hook changes its date (guard)", func() {
				Expect(out["index.md"]).To(MatchRegexp(`COLL\[A=[^;]*;B=[^;]*;C=[^;]*;\]`))
				Expect(out["/list/"]).To(MatchRegexp(`PAG\[A=[^;]*;B=[^;]*;\]`))
				Expect(out["/list/page/2/"]).To(MatchRegexp(`PAG\[C=[^;]*;\]`))
			})
			It("doesn't add a page to a term when a hook adds a tag (guard)", func() {
				Expect(out["index.md"]).To(ContainSubstring("TAGY[]"))
				Expect(out).NotTo(HaveKey("/tags/y/"))
			})

			// Guard: Liquid in a page's own content renders before the hooks.
			It("shows no hook fields in a page's own content (guard)", func() {
				Expect(out["index.md"]).To(ContainSubstring("BODY[A=,;B=,;C=,;]"))
			})
		})
	}

	// alloy dev rebuilds the lists and runs the hooks on every page (issue
	// #1377). The rebuild edits the home page so it renders again.
	Describe("in an alloy dev rebuild", Ordered, ContinueOnFailure, func() {
		var full, rebuilt string
		BeforeAll(func() {
			root := GinkgoT().TempDir()
			write := func(rel, body string) {
				GinkgoHelper()
				p := filepath.Join(root, rel)
				Expect(os.MkdirAll(filepath.Dir(p), 0o755)).To(Succeed())
				Expect(os.WriteFile(p, []byte(body), 0o644)).To(Succeed())
			}
			read := func() string {
				GinkgoHelper()
				b, err := os.ReadFile(filepath.Join(root, "_site", "index.html"))
				Expect(err).NotTo(HaveOccurred())
				return string(b)
			}
			for rel, body := range files(hooks) {
				write(rel, body)
			}
			cfg := newConfig()
			cfg.ProjectRoot = root
			cfg.Build.Output = filepath.Join(root, "_site")
			cfg.Structure = config.StructureConfig{Content: "content", Layouts: "layouts", Data: "data"}
			config.ApplyDefaults(cfg)
			registry, hookRegistry, _ := pipeline.DiscoverPlugins(cfg)
			DeferCleanup(registry.Close)
			s, err := pipeline.InitPipelineState(cfg, registry, hookRegistry)
			Expect(err).NotTo(HaveOccurred())
			first, err := pipeline.Build(cfg, pipeline.BuildOptions{PipelineState: s})
			Expect(err).NotTo(HaveOccurred())
			full = read()

			write("content/index.md", "---\ntitle: Home\nlayout: home\n---\nagain")
			_, err = pipeline.BuildIncremental(cfg, nil, first.Cache, []string{"content/index.md"},
				pipeline.BuildOptions{PipelineState: s})
			Expect(err).NotTo(HaveOccurred())
			rebuilt = read()
		})

		It("shows hook fields in collections in the full build", func() {
			Expect(full).To(ContainSubstring("COLL[A=LA,DA;B=LB,DB;C=LC,DC;]"))
		})
		It("shows hook fields in collections after the rebuild", func() {
			Expect(rebuilt).To(ContainSubstring("<p>again</p>"))
			Expect(rebuilt).To(ContainSubstring("COLL[A=LA,DA;B=LB,DB;C=LC,DC;]"))
		})
		It("shows hook fields in taxonomies.<name>.<term> after the rebuild", func() {
			Expect(rebuilt).To(ContainSubstring("TAXG[A=LA,DA;B=LB,DB;C=LC,DC;]"))
		})
	})
})
