package pipeline_test

import (
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/zeroedin/alloy/internal/config"
	"github.com/zeroedin/alloy/internal/pipeline"
)

// ── Hook results are matched to pages by path and URL (issue #1391) ─────
// onContentLoaded and onDataCascadeReady give back one entry per page they
// got. Alloy applies each entry to the page with its path and url, in a
// full build and in an alloy dev rebuild. A paginated source file makes
// several pages with one path, so each of them gets its own entry.
// onDataCascadeReady entries carry url too. An entry with no url, or a url
// no page sent to the hook has, is matched by path when that path is
// unique, and fails the build when it isn't.
//
// Before, entries were matched by path alone, so every entry for a
// paginated source landed on its last page and the others got nothing.
//
// The site: content/team.md paginates three members, one per page, as
// /team/ann/, /team/bob/ and /team/cy/. content/about.md is an ordinary
// page, and its layout lists every page in site.pages.
//
// See PLAN.md → "Hook results are matched to pages by path and URL (issue #1391)".
var _ = Describe("Hook results are matched to pages by path and URL (issue #1391)", func() {
	const (
		team = `[{"name":"Ann","slug":"ann"},{"name":"Bob","slug":"bob"},{"name":"Cy","slug":"cy"}]`
		// One page per member, all from content/team.md.
		teamPage = "---\ntitle: \"{{ member.name }}\"\nlayout: default\npagination:\n  data: site.data.team\n  perPage: 1\n  as: member\npermalink: \"/team/{{ member.slug }}/\"\n---\n<p>{{ member.name }}</p>"
		// The about page lists, for every page, its url, what onContentLoaded
		// added and what onDataCascadeReady added.
		aboutPage = "---\ntitle: About\nlayout: list\n---\nabout"
		list      = "{{ content }} LIST[{% for p in site.pages %}{{ p.url }}={{ p.badge }}|{{ p.cbadge }};{% endfor %}]"
	)

	var root string

	write := func(path, body string) {
		GinkgoHelper()
		full := filepath.Join(root, path)
		Expect(os.MkdirAll(filepath.Dir(full), 0o755)).To(Succeed())
		Expect(os.WriteFile(full, []byte(body), 0o644)).To(Succeed())
	}
	output := func(path string) string {
		GinkgoHelper()
		body, err := os.ReadFile(filepath.Join(root, "_site", path))
		Expect(err).NotTo(HaveOccurred())
		return string(body)
	}

	// site writes the site with the given plugin and returns its config.
	site := func(plugin string) *config.Config {
		GinkgoHelper()
		root = GinkgoT().TempDir()
		write("plugins/hooks.js", plugin)
		write("data/team.json", team)
		write("content/team.md", teamPage)
		write("content/about.md", aboutPage)
		write("layouts/default.liquid", "{{ content }} BADGE[{{ page.badge }}] CBADGE[{{ page.cbadge }}]")
		write("layouts/list.liquid", list)
		cfg := &config.Config{
			Title:       "Hook Results",
			BaseURL:     "https://example.com",
			ProjectRoot: root,
			Build:       config.BuildConfig{Output: filepath.Join(root, "_site")},
			Structure:   config.StructureConfig{Content: "content", Layouts: "layouts", Data: "data"},
		}
		config.ApplyDefaults(cfg)
		return cfg
	}

	// state discovers the site's plugins, as alloy dev does once.
	state := func(cfg *config.Config) *pipeline.PipelineState {
		GinkgoHelper()
		registry, hooks, _ := pipeline.DiscoverPlugins(cfg)
		DeferCleanup(registry.Close)
		s, err := pipeline.InitPipelineState(cfg, registry, hooks)
		Expect(err).NotTo(HaveOccurred())
		return s
	}

	// Each hook marks every page with its own url. onContentLoaded returns
	// its entries in reverse order, which must not matter.
	// onDataCascadeReady returns only the field it adds: a page's data holds
	// its front matter, badge included, so returning all of it would put the
	// right badge back on a page that onContentLoaded's result missed.
	const marking = `export default function(alloy) {
  alloy.hook('onContentLoaded', { pages: true }, (pages) =>
    pages.map(p => ({ ...p, frontMatter: { ...p.frontMatter, badge: 'L' + p.url }, html: p.html + '<i>H' + p.url + '</i>' })).reverse());
  alloy.hook('onDataCascadeReady', { pages: true }, (pages) =>
    pages.map(p => ({ ...p, data: { cbadge: 'D' + p.url } })));
}
`

	Describe("each page made from one paginated file gets its own result", Ordered, ContinueOnFailure, func() {
		var (
			fullOut, rebuiltOut string
			// full holds each page as the full build wrote it, by member.
			full = map[string]string{}
		)

		BeforeAll(func() {
			cfg := site(marking)
			s := state(cfg)
			first, err := pipeline.Build(cfg, pipeline.BuildOptions{PipelineState: s})
			Expect(err).NotTo(HaveOccurred())
			fullOut = output("about/index.html")
			for _, m := range []string{"ann", "bob", "cy"} {
				full[m] = output("team/" + m + "/index.html")
			}

			// An alloy dev rebuild after an edit to the about page.
			write("content/about.md", aboutPage+" again")
			_, err = pipeline.BuildIncremental(cfg, nil, first.Cache, []string{"content/about.md"},
				pipeline.BuildOptions{PipelineState: s})
			Expect(err).NotTo(HaveOccurred())
			rebuiltOut = output("about/index.html")
		})

		It("applies each onContentLoaded result to its own page, in a full build", func() {
			for _, m := range []string{"ann", "bob", "cy"} {
				Expect(full[m]).To(ContainSubstring("BADGE[L/team/"+m+"/]"), m)
			}
		})
		// onContentLoaded appends <i>H + url</i> to each page's html, so a
		// page that got another page's entry shows that page's body and url.
		It("applies each onContentLoaded html to its own page, in a full build", func() {
			for m, name := range map[string]string{"ann": "Ann", "bob": "Bob", "cy": "Cy"} {
				Expect(full[m]).To(ContainSubstring("<p>"+name+"</p>"), m)
				Expect(full[m]).To(ContainSubstring("<i>H/team/"+m+"/</i>"), m)
			}
			Expect(fullOut).To(ContainSubstring("<p>about</p>"))
			Expect(fullOut).To(ContainSubstring("<i>H/about/</i>"))
		})
		It("applies each onDataCascadeReady result to its own page, in a full build", func() {
			for _, m := range []string{"ann", "bob", "cy"} {
				Expect(full[m]).To(ContainSubstring("CBADGE[D/team/"+m+"/]"), m)
			}
		})
		It("lists each page with its own results, in a full build", func() {
			for _, m := range []string{"ann", "bob", "cy"} {
				u := "/team/" + m + "/"
				Expect(fullOut).To(ContainSubstring(u+"=L"+u+"|D"+u+";"), m)
			}
		})
		It("lists each page with its own results, in an alloy dev rebuild", func() {
			for _, m := range []string{"ann", "bob", "cy"} {
				u := "/team/" + m + "/"
				Expect(rebuiltOut).To(ContainSubstring(u+"=L"+u+"|D"+u+";"), m)
			}
		})

		// Guard: a page with a path of its own gets its results today, from
		// onContentLoaded even though the entries come back reversed.
		It("still applies onContentLoaded's result to a page with its own path (guard)", func() {
			Expect(fullOut).To(ContainSubstring("/about/=L/about/|"))
			Expect(rebuiltOut).To(ContainSubstring("/about/=L/about/|"))
		})
	})

	// onDataCascadeReady entries carry url, as onContentLoaded entries do.
	It("gives onDataCascadeReady each page's url", func() {
		cfg := site(`export default function(alloy) {
  alloy.hook('onDataCascadeReady', { pages: true }, (pages) =>
    pages.map(p => ({ ...p, data: { ...p.data, cbadge: typeof p.url === 'string' ? 'D' + p.url : 'none' } })));
}
`)
		_, err := pipeline.Build(cfg, pipeline.BuildOptions{PipelineState: state(cfg)})
		Expect(err).NotTo(HaveOccurred())
		Expect(output("about/index.html")).To(ContainSubstring("/about/=|D/about/;"))
	})

	// A hook scoped to one page changes that page, not the others made from
	// the same file.
	Describe("a hook scoped to one page of a paginated file", Ordered, ContinueOnFailure, func() {
		BeforeAll(func() {
			cfg := site(`export default function(alloy) {
  alloy.hook('onContentLoaded', { pages: '/team/ann/' }, (pages) =>
    pages.map(p => { p.frontMatter.badge = 'L' + p.url; return p; }));
  alloy.hook('onDataCascadeReady', { pages: '/team/ann/' }, (pages) =>
    pages.map(p => ({ ...p, data: { cbadge: 'D' + p.url } })));
}
`)
			_, err := pipeline.Build(cfg, pipeline.BuildOptions{PipelineState: state(cfg)})
			Expect(err).NotTo(HaveOccurred())
		})

		It("changes the page in its scope", func() {
			Expect(output("team/ann/index.html")).To(ContainSubstring("BADGE[L/team/ann/]"))
			Expect(output("team/ann/index.html")).To(ContainSubstring("CBADGE[D/team/ann/]"))
		})
		It("leaves the other pages from the same file alone", func() {
			Expect(output("team/cy/index.html")).To(ContainSubstring("BADGE[] CBADGE[]"))
			Expect(output("team/bob/index.html")).To(ContainSubstring("BADGE[] CBADGE[]"))
		})
	})

	// An entry Alloy can't place fails the build: nothing is guessed.
	Describe("an entry that can't be matched to one page", func() {
		DescribeTable("fails the build, naming the hook and the path",
			func(hook, body string) {
				cfg := site("export default function(alloy) {\n  alloy.hook('" + hook + "', { pages: true }, (pages) =>\n    " + body + ");\n}\n")
				_, err := pipeline.Build(cfg, pipeline.BuildOptions{PipelineState: state(cfg)})
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring(hook))
				Expect(err.Error()).To(ContainSubstring("team.md"))
			},
			Entry("onContentLoaded, with no url",
				"onContentLoaded", "pages.map(p => ({ path: p.path, frontMatter: { ...p.frontMatter, badge: 'x' } }))"),
			Entry("onContentLoaded, with a url no page has",
				"onContentLoaded", "pages.map(p => ({ ...p, url: '/nowhere/', frontMatter: { ...p.frontMatter, badge: 'x' } }))"),
			Entry("onDataCascadeReady, with no url",
				"onDataCascadeReady", "pages.map(p => ({ path: p.path, data: { ...p.data, cbadge: 'x' } }))"),
		)

		// Guard: an entry for a page with a path of its own needs no url.
		// It is matched by path, as today.
		It("still matches an entry with no url by path when the path is unique (guard)", func() {
			cfg := site(`export default function(alloy) {
  alloy.hook('onContentLoaded', { pages: '/about/' }, (pages) =>
    pages.map(p => ({ path: p.path, frontMatter: { ...p.frontMatter, badge: 'x' } })));
  alloy.hook('onDataCascadeReady', { pages: '/about/' }, (pages) =>
    pages.map(p => ({ path: p.path, data: { ...p.data, cbadge: 'y' } })));
}
`)
			_, err := pipeline.Build(cfg, pipeline.BuildOptions{PipelineState: state(cfg)})
			Expect(err).NotTo(HaveOccurred())
			Expect(output("about/index.html")).To(ContainSubstring("/about/=x|y;"))
		})
	})
})
