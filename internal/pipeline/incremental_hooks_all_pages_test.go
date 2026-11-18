package pipeline_test

import (
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/zeroedin/alloy/internal/config"
	"github.com/zeroedin/alloy/internal/pipeline"
)

// ── Hooks see every page in an incremental rebuild (issue #1377) ────────
// In an alloy dev rebuild, onContentLoaded and onDataCascadeReady run on
// every page, as in a full build. onContentLoaded gets every page's
// rendered body; onDataCascadeReady gets path and data only. A hook
// is asked again on every rebuild, not answered from a previous one. Only
// the pages the rebuild already re-renders go through their layout and get
// written.
//
// Before, the hooks ran only on the re-rendered pages. Every page is read
// from disk on each rebuild, so the others came back without what the hooks
// added, and a page listing them showed it for the edited page only.
//
// The site has no collection and no taxonomy, so the rebuild re-renders
// only the edited page (issue #1320).
//
// See PLAN.md → "Hooks see every page in an incremental rebuild (issue #1377)".
var _ = Describe("Hooks see every page in an incremental rebuild (issue #1377)", Ordered, ContinueOnFailure, func() {
	// onContentLoaded adds four fields to every page:
	//   loaded   - "L"
	//   words    - the number of words in the page's rendered body, so an
	//              empty body shows as 0
	//   rendered - "Y" if the body it got is rendered HTML (has a <p>), "N"
	//              if it is the Markdown source
	//   lead     - the number of words in page A's body, the page the test
	//              edits. Every page reads it, so a value kept from an earlier
	//              build shows as the old count.
	// onDataCascadeReady adds two:
	//   cascaded - "D"
	//   clead    - the lead the page has when this hook runs, so a value kept
	//              from an earlier build shows as the old count
	const plugin = `const words = (p) => (p.html || '').replace(/<[^>]*>/g, ' ').split(/\s+/).filter(Boolean).length;
export default function(alloy) {
  alloy.hook('onContentLoaded', { pages: true }, (pages) => {
    const a = pages.find(p => p.path.endsWith('a.md'));
    return pages.map(p => {
      p.frontMatter.loaded = 'L';
      p.frontMatter.words = words(p);
      p.frontMatter.rendered = /<p>/.test(p.html || '') ? 'Y' : 'N';
      p.frontMatter.lead = a ? words(a) : -1;
      return p;
    });
  });
  alloy.hook('onDataCascadeReady', { pages: true }, (pages) =>
    pages.map(p => { p.data = Object.assign({}, p.data, { cascaded: 'D', clead: p.data.lead }); return p; }));
}
`

	// The layout prints each field for every page in site.pages.
	field := func(label, name string) string {
		return label + "[{% for p in site.pages %}{{ p." + name + " }};{% endfor %}]"
	}
	const layout = `{{ content }}`

	var (
		root       string
		cfg        *config.Config
		fullOut    string
		rebuilt    *pipeline.BuildResult
		rebuiltOut string
	)

	write := func(path, body string) {
		GinkgoHelper()
		full := filepath.Join(root, path)
		Expect(os.MkdirAll(filepath.Dir(full), 0o755)).To(Succeed())
		Expect(os.WriteFile(full, []byte(body), 0o644)).To(Succeed())
	}
	// output reads a written page from the output directory.
	output := func(path string) string {
		GinkgoHelper()
		body, err := os.ReadFile(filepath.Join(root, "_site", path))
		Expect(err).NotTo(HaveOccurred())
		return string(body)
	}
	post := func(title, body string) string {
		return "---\ntitle: " + title + "\nlayout: default\n---\n" + body
	}

	BeforeAll(func() {
		root = GinkgoT().TempDir()
		write("plugins/fields.js", plugin)
		write("layouts/default.liquid", layout+
			field("LOADED", "loaded")+" "+
			field("CASCADED", "cascaded")+" "+
			field("LEAD", "lead")+" "+
			field("CLEAD", "clead")+" "+
			field("RENDERED", "rendered")+" "+
			field("WORDS", "words"))
		// 2, 3 and 4 words.
		write("content/blog/a.md", post("A", "alpha one"))
		write("content/blog/b.md", post("B", "beta two three"))
		write("content/blog/c.md", post("C", "gamma four five six"))

		cfg = &config.Config{
			Title:       "Incremental Hooks",
			BaseURL:     "https://example.com",
			ProjectRoot: root,
			Build:       config.BuildConfig{Output: filepath.Join(root, "_site")},
			Structure:   config.StructureConfig{Content: "content", Layouts: "layouts", Data: "data"},
		}
		config.ApplyDefaults(cfg)
		registry, hooks, _ := pipeline.DiscoverPlugins(cfg)
		DeferCleanup(registry.Close)
		state, err := pipeline.InitPipelineState(cfg, registry, hooks)
		Expect(err).NotTo(HaveOccurred())

		// The first build in alloy dev is a full one.
		first, err := pipeline.Build(cfg, pipeline.BuildOptions{PipelineState: state})
		Expect(err).NotTo(HaveOccurred())
		fullOut = output("blog/a/index.html")

		// A body-only edit: 3 words now.
		write("content/blog/a.md", post("A", "alpha one again"))
		rebuilt, err = pipeline.BuildIncremental(cfg, nil, first.Cache, []string{"content/blog/a.md"},
			pipeline.BuildOptions{PipelineState: state})
		Expect(err).NotTo(HaveOccurred())
		rebuiltOut = output("blog/a/index.html")
	})

	It("keeps what onContentLoaded added on pages the rebuild didn't re-render", func() {
		Expect(rebuiltOut).To(ContainSubstring("LOADED[L;L;L;]"))
	})
	It("keeps what onDataCascadeReady added on pages the rebuild didn't re-render", func() {
		Expect(rebuiltOut).To(ContainSubstring("CASCADED[D;D;D;]"))
	})
	It("asks onContentLoaded again for every page, with every page in its input", func() {
		Expect(rebuiltOut).To(ContainSubstring("LEAD[3;3;3;]"),
			"every page reads the edited page's new word count, 3: a 2 is a value kept from the first build")
	})
	It("asks onDataCascadeReady again for every page", func() {
		Expect(rebuiltOut).To(ContainSubstring("CLEAD[3;3;3;]"),
			"every page's cascade field copies the new word count, 3: a 2 is a value kept from the first build")
	})
	It("gives onContentLoaded every page's rendered body", func() {
		Expect(rebuiltOut).To(ContainSubstring("WORDS[3;3;4;]"),
			"each page's word count comes from its rendered body: a 0 means the hook got an empty body")
		Expect(rebuiltOut).To(ContainSubstring("RENDERED[Y;Y;Y;]"),
			"each page's body is rendered HTML: an N means the hook got the Markdown source")
	})

	// Guards: these work today and must keep working.
	It("re-renders only the edited page (guard)", func() {
		Expect(rebuilt.PagesRendered).To(ConsistOf("blog/a.md"))
	})
	It("gives every page the hooks' fields in a full build (guard)", func() {
		Expect(fullOut).To(ContainSubstring("LOADED[L;L;L;]"))
		Expect(fullOut).To(ContainSubstring("CASCADED[D;D;D;]"))
		Expect(fullOut).To(ContainSubstring("LEAD[2;2;2;]"))
		Expect(fullOut).To(ContainSubstring("CLEAD[2;2;2;]"))
		Expect(fullOut).To(ContainSubstring("WORDS[2;3;4;]"))
		Expect(fullOut).To(ContainSubstring("RENDERED[Y;Y;Y;]"))
	})
	It("shows the edited page's new body in the rebuild (guard)", func() {
		Expect(rebuiltOut).To(ContainSubstring("alpha one again"))
	})
})
