package pipeline_test

import (
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/zeroedin/alloy/internal/config"
	"github.com/zeroedin/alloy/internal/pipeline"
)

// ── alloy dev re-renders lists after a body-only edit when a content hook
// runs (issue #1437) ───────────────────────────────────────────────────────
// A list shows each page's front matter, and the fields onContentLoaded and
// onDataCascadeReady add. A hook can build a field from a page's body, such
// as an excerpt from its html. So on a site with either hook, a body-only
// edit re-renders every page and regenerates the tag pages, as a front
// matter edit does (issue #1320). That holds whatever the hook's pages:
// scope.
//
// Without either hook, a list shows nothing a body edit changes, and a
// body-only edit stays incremental.
//
// Before, a body-only edit never re-rendered the lists, so a field a hook
// built from the body stayed stale on list and tag pages.
//
// See PLAN.md → "List and tag pages stay current (issue #1320)".
var _ = Describe("alloy dev re-renders lists after a body-only edit when a content hook runs (issue #1437)", func() {
	write := func(root, rel, body string) {
		GinkgoHelper()
		path := filepath.Join(root, rel)
		Expect(os.MkdirAll(filepath.Dir(path), 0o755)).To(Succeed())
		Expect(os.WriteFile(path, []byte(body), 0o644)).To(Succeed())
	}
	post := func(title, body string) string {
		return "---\ntitle: " + title + "\ndate: 2026-01-01\ntags: [\"a\"]\nlayout: post\n---\n" + body
	}

	// excerpt: onContentLoaded sets each page's excerpt from its html.
	const excerpt = "export default function(alloy) {\n" +
		"  alloy.hook('onContentLoaded', { pages: true }, (pages) => pages.map(p => ({ ...p,\n" +
		"    frontMatter: { ...p.frontMatter, excerpt: (p.html || '').replace(/<[^>]+>/g, '').trim() } })));\n" +
		"}\n"

	type built struct {
		root  string
		cfg   *config.Config
		state *pipeline.PipelineState
		first *pipeline.BuildResult
	}

	// build writes a site with the plugin src (none if empty) and any extra
	// files, and runs the first build. The home page lists site.pages; the
	// tag page lists its pages. Both print each post's excerpt, and the
	// cascaded field an onDataCascadeReady hook may add.
	build := func(src string, extra ...string) built {
		GinkgoHelper()
		root := GinkgoT().TempDir()
		list := "{% if p.layout == 'post' %}[{{ p.title }}={{ p.excerpt }}{{ p.cascaded }}]{% endif %}"
		write(root, "layouts/default.liquid", "{{ content }}")
		write(root, "layouts/post.liquid", "{{ content }}")
		write(root, "layouts/home.liquid", "LIST:{% for p in site.pages %}"+list+"{% endfor %}")
		write(root, "layouts/taxonomies/tags.liquid", "TAG:{% for p in taxonomy.pages %}"+list+"{% endfor %}")
		write(root, "content/index.md", "---\ntitle: Home\nlayout: home\n---\nhome")
		write(root, "content/blog/one.md", post("One", "first"))
		write(root, "content/blog/two.md", post("Two", "second"))
		if src != "" {
			write(root, "plugins/hooks.js", src)
		}
		for i := 0; i+1 < len(extra); i += 2 {
			write(root, extra[i], extra[i+1])
		}
		cfg := &config.Config{
			Title:       "Body Edit",
			BaseURL:     "https://example.com",
			ProjectRoot: root,
			Build:       config.BuildConfig{Output: filepath.Join(root, "_site")},
			Structure:   config.StructureConfig{Content: "content", Layouts: "layouts"},
			Taxonomies:  map[string]*config.TaxonomyConfig{"tags": {}},
		}
		config.ApplyDefaults(cfg)
		registry, hooks, _ := pipeline.DiscoverPlugins(cfg)
		DeferCleanup(registry.Close)
		state, err := pipeline.InitPipelineState(cfg, registry, hooks)
		Expect(err).NotTo(HaveOccurred())
		first, err := pipeline.Build(cfg, pipeline.BuildOptions{PipelineState: state, CaptureRenderedContent: true})
		Expect(err).NotTo(HaveOccurred())
		return built{root: root, cfg: cfg, state: state, first: first}
	}

	// editBody changes only post One's body, to "edited", and rebuilds.
	editBody := func(b built) *pipeline.BuildResult {
		GinkgoHelper()
		write(b.root, "content/blog/one.md", post("One", "edited"))
		result, err := pipeline.BuildIncremental(b.cfg, nil, b.first.Cache, []string{"content/blog/one.md"},
			pipeline.BuildOptions{PipelineState: b.state, CaptureRenderedContent: true})
		Expect(err).NotTo(HaveOccurred())
		return result
	}

	read := func(b built, rel string) string {
		GinkgoHelper()
		out, err := os.ReadFile(filepath.Join(b.root, "_site", rel))
		Expect(err).NotTo(HaveOccurred(), "%s must exist in the output", rel)
		return string(out)
	}

	Describe("with an onContentLoaded hook that builds a field from the html", Ordered, ContinueOnFailure, func() {
		var b built
		var result *pipeline.BuildResult
		BeforeAll(func() {
			b = build(excerpt)
			Expect(read(b, "index.html")).To(ContainSubstring("[One=first]"),
				"sanity: the first build lists the hook's excerpt")
			Expect(read(b, "tags/a/index.html")).To(ContainSubstring("[One=first]"),
				"sanity: the first build's tag page lists the hook's excerpt")
			result = editBody(b)
		})

		It("renders the edited page (guard)", func() {
			Expect(read(b, "blog/one/index.html")).To(ContainSubstring("edited"))
		})
		It("shows the new excerpt on a page that lists site.pages", func() {
			Expect(result.PagesRendered).To(ContainElement("index.md"))
			Expect(read(b, "index.html")).To(ContainSubstring("[One=edited]"))
		})
		It("shows the new excerpt on the tag page", func() {
			Expect(read(b, "tags/a/index.html")).To(ContainSubstring("[One=edited]"))
		})
	})

	It("re-renders the lists with an onContentLoaded hook whose scope leaves the edited page out", func() {
		// The hook only reaches /other/x/, which it marks.
		b := build("export default function(alloy) {\n"+
			"  alloy.hook('onContentLoaded', { pages: '/other/**' }, (pages) => pages.map(p => ({ ...p,\n"+
			"    frontMatter: { ...p.frontMatter, excerpt: 'X' } })));\n"+
			"}\n",
			"content/other/x.md", "---\ntitle: X\nlayout: post\n---\nx")
		Expect(read(b, "index.html")).To(ContainSubstring("[X=X]"), "sanity: the plugin loaded")
		Expect(editBody(b).PagesRendered).To(ContainElement("index.md"))
	})

	It("re-renders the lists with only an onDataCascadeReady hook", func() {
		b := build("export default function(alloy) {\n" +
			"  alloy.hook('onDataCascadeReady', { pages: true }, (pages) =>\n" +
			"    pages.map(p => ({ ...p, data: { cascaded: 'C' } })));\n" +
			"}\n")
		Expect(read(b, "index.html")).To(ContainSubstring("[One=C]"), "sanity: the plugin loaded")
		Expect(editBody(b).PagesRendered).To(ContainElement("index.md"))
	})

	It("keeps a body-only edit incremental with only an onPagesReady hook (guard)", func() {
		b := build("export default function(alloy) {\n" +
			"  alloy.hook('onPagesReady', { pages: false }, () => ({ addPages: [{ path: 'virtual.md', url: '/virtual/',\n" +
			"    frontMatter: { title: 'V' }, content: 'v' }] }));\n" +
			"}\n")
		Expect(read(b, "virtual/index.html")).To(ContainSubstring("v"), "sanity: the plugin loaded")
		result := editBody(b)
		Expect(result.PagesRendered).To(ContainElement("blog/one.md"))
		Expect(result.PagesRendered).NotTo(ContainElement("index.md"),
			"onPagesReady can't change what a list shows about a page")
	})

	It("keeps a body-only edit incremental with no plugin (guard)", func() {
		b := build("")
		result := editBody(b)
		Expect(result.PagesRendered).To(ContainElement("blog/one.md"))
		Expect(result.PagesRendered).NotTo(ContainElement("index.md"))
	})
})
