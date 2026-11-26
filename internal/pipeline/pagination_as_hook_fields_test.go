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

// ── Pagination's as: name shows what content hooks add (issue #1426) ────
// On a page that paginates a collection, the name set with pagination.as
// (default "item") holds the same pages as pagination.items. In a layout it
// shows them with the fields onContentLoaded and onDataCascadeReady add, as
// pagination.items does after #1385:
//   - perPage above 1: the page's group of items;
//   - perPage 1: the one item.
//
// Unchanged: front matter filled in from the item (title, permalink) uses
// the item before the hooks; Liquid in the page's own content renders
// before the hooks; pagination over site.data is untouched.
//
// Before #1426, the as: name held a second copy of the items, made before
// the hooks, so a field a hook added was empty in it.
//
// See PLAN.md → "Pagination's as: name shows them too (issue #1426)".
var _ = Describe("Pagination's as: name shows what content hooks add (issue #1426)", func() {
	const node = "export const runtime = \"node\";\n"

	// each prints, for every page in a list, its title and the fields the
	// two hooks add: A=LA,DA; when both hooks reached page A.
	each := func(list string) string {
		return "{% for p in " + list + " %}{{ p.title }}={{ p.loaded }},{{ p.cascaded }};{% endfor %}"
	}

	const hooks = "export default function(alloy) {\n" +
		"  alloy.hook('onContentLoaded', { pages: true }, (pages) => pages.map(p =>\n" +
		"    ({ ...p, frontMatter: { ...p.frontMatter, loaded: 'L' + p.frontMatter.title } })));\n" +
		"  alloy.hook('onDataCascadeReady', { pages: true }, (pages) =>\n" +
		"    pages.map(p => ({ ...p, data: { cascaded: 'D' + p.data.title } })));\n" +
		"}\n"

	// Posts A, B, C in a blog collection, newest first, so path order and
	// date order agree.
	files := func(hookSrc string) map[string]string {
		return map[string]string{
			"content/blog/_data.yaml": "permalink: \"/blog/:year/:slug/\"",
			"content/blog/a.md":       "---\ntitle: A\ndate: 2026-01-03\n---\na",
			"content/blog/b.md":       "---\ntitle: B\ndate: 2026-01-02\n---\nb",
			"content/blog/c.md":       "---\ntitle: C\ndate: 2026-01-01\n---\nc",

			// A list page, two posts to a page, under the name posts. Its own
			// content loops over posts too.
			"content/list.md": "---\ntitle: List\nlayout: list\npagination:\n  data: collections.blog\n  perPage: 2\n  as: posts\n---\n" +
				"BODY[" + each("posts") + "]",
			"layouts/list.liquid": "AS[" + each("posts") + "] {{ content }}",

			// One page per post, under the name member. Its title and
			// permalink are filled in from the item.
			"content/people.md": "---\ntitle: \"Person {{ member.title }}-{{ member.loaded }}\"\nlayout: person\n" +
				"permalink: \"/people/{{ member.title | downcase }}/\"\npagination:\n  data: collections.blog\n  as: member\n---\nperson",
			"layouts/person.liquid": "ONE[{{ member.title }}={{ member.loaded }},{{ member.cascaded }}] TITLE[{{ page.title }}]",

			// No as: the name is item.
			"content/all.md":     "---\ntitle: All\nlayout: all\npagination:\n  data: collections.blog\n  perPage: 3\n---\nall",
			"layouts/all.liquid": "DEF[" + each("item") + "]",

			// Pagination over site.data: data items, which hooks never touch.
			"data/team.yaml":      "- name: Ann\n- name: Bob\n",
			"content/team.md":     "---\ntitle: Team\nlayout: team\npagination:\n  data: site.data.team\n  perPage: 2\n  as: people\n---\nteam",
			"layouts/team.liquid": "TEAM[{% for m in people %}{{ m.name }}={{ m.loaded }},{{ m.cascaded }};{% endfor %}]",

			"plugins/hooks.js": hookSrc,
		}
	}

	newConfig := func() *config.Config {
		return &config.Config{
			Title:   "Pagination As Hook Fields",
			BaseURL: "https://example.com",
			Build:   config.BuildConfig{Output: "_site"},
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

			It("shows hook fields in the as: name on a list page, on every page", func() {
				Expect(out["/list/"]).To(ContainSubstring("AS[A=LA,DA;B=LB,DB;]"))
				Expect(out["/list/page/2/"]).To(ContainSubstring("AS[C=LC,DC;]"))
			})
			It("shows hook fields in the as: name with perPage 1", func() {
				Expect(out["/people/a/"]).To(ContainSubstring("ONE[A=LA,DA]"))
				Expect(out["/people/c/"]).To(ContainSubstring("ONE[C=LC,DC]"))
			})
			It("shows hook fields under the default name item", func() {
				Expect(out["/all/"]).To(ContainSubstring("DEF[A=LA,DA;B=LB,DB;C=LC,DC;]"))
			})

			// Guards: these hold today and must keep holding.
			It("fills front matter from the item as it was before the hooks (guard)", func() {
				Expect(out).To(HaveKey("/people/a/"))
				Expect(out["/people/a/"]).To(ContainSubstring("TITLE[Person A-]"))
			})
			It("shows no hook fields in the page's own content (guard)", func() {
				Expect(out["/list/"]).To(ContainSubstring("BODY[A=,;B=,;]"))
			})
			It("leaves pagination over site.data alone (guard)", func() {
				Expect(out["/team/"]).To(ContainSubstring("TEAM[Ann=,;Bob=,;]"))
			})
		})
	}

	// alloy dev renders the list page again on a rebuild; the as: name must
	// show the hook fields there too.
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
				b, err := os.ReadFile(filepath.Join(root, "_site", "list", "index.html"))
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

			write("content/list.md", "---\ntitle: List\nlayout: list\npagination:\n  data: collections.blog\n  perPage: 2\n  as: posts\n---\nagain")
			_, err = pipeline.BuildIncremental(cfg, nil, first.Cache, []string{"content/list.md"},
				pipeline.BuildOptions{PipelineState: s})
			Expect(err).NotTo(HaveOccurred())
			rebuilt = read()
		})

		It("shows hook fields in the as: name in the full build", func() {
			Expect(full).To(ContainSubstring("AS[A=LA,DA;B=LB,DB;]"))
		})
		It("shows hook fields in the as: name after the rebuild", func() {
			Expect(rebuilt).To(ContainSubstring("<p>again</p>"))
			Expect(rebuilt).To(ContainSubstring("AS[A=LA,DA;B=LB,DB;]"))
		})
	})
})
