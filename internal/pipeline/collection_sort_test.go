package pipeline_test

import (
	"os"
	"path/filepath"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/zeroedin/alloy/internal/config"
	"github.com/zeroedin/alloy/internal/pipeline"
)

// ── Collections and taxonomy terms are sorted (issue #1418) ──────────────
// The build sorts every collection by its sortBy and order (default: date,
// newest first), and every taxonomy term's pages by date, newest first.
// Missing values go last in both orders; ties go by file path, A to Z.
//
// Before #1418 the build sorted nothing: every list came out in the order
// the files were found, by path. Each fixture below names its files so
// that path order differs from the order the spec asks for.
//
// See PLAN.md → "How collections and taxonomies are sorted (issue #1418)".
var _ = Describe("Collections and taxonomy terms are sorted (issue #1418)", func() {
	// titles prints the title of every page in a list: B;D;C; for pages
	// titled B, D and C.
	titles := func(list string) string {
		return "{% for p in " + list + " %}{{ p.title }};{% endfor %}"
	}
	page := func(title, extra string) string {
		return "---\ntitle: " + title + "\n" + extra + "---\n" + strings.ToLower(title)
	}

	// onPagesReady adds page V, dated and tagged x. onContentLoaded gives
	// post A the newest date of all, which must not move it (issue #1385).
	const hooks = "export default function(alloy) {\n" +
		"  alloy.hook('onPagesReady', { pages: false }, () => ({ addPages: [{ path: 'virtual.md', url: '/virtual/',\n" +
		"    frontMatter: { title: 'V', date: '2026-02-15', tags: ['x'] }, content: 'v' }] }));\n" +
		"  alloy.hook('onContentLoaded', { pages: true }, (pages) => pages.map(p =>\n" +
		"    p.frontMatter.title === 'A' ? { ...p, frontMatter: { ...p.frontMatter, date: '2026-12-01' } } : p));\n" +
		"}\n"

	files := func() map[string]string {
		return map[string]string{
			"content/index.md": "---\ntitle: Home\nlayout: home\n---\nhome",
			"content/list.md":  "---\ntitle: List\nlayout: list\npagination:\n  data: collections.posts\n  perPage: 2\n---\nlist",

			// posts: a section with a date in its permalink and no
			// collections: entry, so the default order: newest first. C and
			// D share a day; D is later that day. E and F share a date.
			"content/posts/_data.yaml": "permalink: \"/posts/:year/:slug/\"",
			"content/posts/a.md":       page("A", "date: 2026-01-01\ntags: [x]\n"),
			"content/posts/b.md":       page("B", "date: 2026-03-01\ntags: [x]\n"),
			"content/posts/c.md":       page("C", "date: 2026-02-01T09:00:00Z\ntags: [x]\n"),
			"content/posts/d.md":       page("D", "date: 2026-02-01T14:30:00Z\ntags: [x]\n"),
			"content/posts/e.md":       page("E", "date: 2026-01-15\n"),
			"content/posts/f.md":       page("F", "date: 2026-01-15\n"),

			// notes: `notes:` with nothing under it. N0 and N2 have no date.
			"content/notes/n0.md": page("N0", "tags: [x]\n"),
			"content/notes/n1.md": page("N1", "date: 2026-01-01\n"),
			"content/notes/n2.md": page("N2", ""),
			"content/notes/n3.md": page("N3", "date: 2026-02-01\n"),

			// log: order asc. L0 and L3 have no date. L1 is tagged x.
			"content/log/l0.md": page("L0", ""),
			"content/log/l1.md": page("L1", "date: 2026-02-01\ntags: [x]\n"),
			"content/log/l2.md": page("L2", "date: 2026-01-01\n"),
			"content/log/l3.md": page("L3", ""),

			// guides: sortBy weight, order asc. G5's weight is a digit-only
			// string; G3 has none.
			"content/guides/g1.md": page("G1", "weight: 10\n"),
			"content/guides/g2.md": page("G2", "weight: 2\n"),
			"content/guides/g3.md": page("G3", ""),
			"content/guides/g4.md": page("G4", "weight: 2\n"),
			"content/guides/g5.md": page("G5", "weight: \"3\"\n"),

			// ranked: sortBy score, no order, so desc. 10 must beat 9 as a
			// number. R2 has no score.
			"content/ranked/r0.md": page("R0", "score: 5\n"),
			"content/ranked/r1.md": page("R1", "score: 5\n"),
			"content/ranked/r2.md": page("R2", ""),
			"content/ranked/r3.md": page("R3", "score: 9\n"),
			"content/ranked/r4.md": page("R4", "score: 10\n"),

			"layouts/home.liquid": "POSTS[" + titles("collections.posts") + "] " +
				"NOTES[" + titles("collections.notes") + "] " +
				"LOG[" + titles("collections.log") + "] " +
				"GUIDES[" + titles("collections.guides") + "] " +
				"RANKED[" + titles("collections.ranked") + "] " +
				"TAGX[" + titles("taxonomies.tags.x") + "]",
			"layouts/list.liquid":            "PAG[" + titles("pagination.items") + "]",
			"layouts/taxonomies/tags.liquid": "TAXP[" + titles("taxonomy.pages") + "]",
			"plugins/hooks.js":               hooks,
		}
	}

	newConfig := func() *config.Config {
		return &config.Config{
			Title:      "Collection Sort",
			BaseURL:    "https://example.com",
			Build:      config.BuildConfig{Output: "_site"},
			Taxonomies: map[string]*config.TaxonomyConfig{"tags": {}},
			Collections: map[string]*config.CollectionConfig{
				"notes":  nil,
				"log":    {Order: "asc"},
				"guides": {SortBy: "weight", Order: "asc"},
				"ranked": {SortBy: "score"},
			},
		}
	}

	Describe("in a full build", Ordered, ContinueOnFailure, func() {
		var out map[string]string
		BeforeAll(func() {
			result, err := pipeline.BuildWithContent(newConfig(), files(), pipeline.BuildOptions{CaptureRenderedContent: true})
			Expect(err).NotTo(HaveOccurred())
			out = result.RenderedContent
		})

		It("sorts a collection by date, newest first, by default", func() {
			// D before C: same day, D later. E before F: same date, by path.
			Expect(out["index.md"]).To(ContainSubstring("POSTS[B;D;C;E;F;A;]"))
		})
		It("puts pages with no date last, by path", func() {
			Expect(out["index.md"]).To(ContainSubstring("NOTES[N3;N1;N0;N2;]"))
		})
		It("sorts oldest first with order: asc, and still puts pages with no date last", func() {
			Expect(out["index.md"]).To(ContainSubstring("LOG[L2;L1;L0;L3;]"))
		})
		It("sorts by another key as numbers, with ties by path and missing values last", func() {
			Expect(out["index.md"]).To(ContainSubstring("GUIDES[G2;G4;G5;G1;G3;]"))
		})
		It("sorts by another key, desc by default, with missing values still last", func() {
			Expect(out["index.md"]).To(ContainSubstring("RANKED[R4;R3;R0;R1;R2;]"))
		})
		It("follows the collection's order in pagination.items", func() {
			Expect(out["/list/"]).To(ContainSubstring("PAG[B;D;]"))
			Expect(out["/list/page/2/"]).To(ContainSubstring("PAG[C;E;]"))
			Expect(out["/list/page/3/"]).To(ContainSubstring("PAG[F;A;]"))
		})

		// A term holds pages from posts, log (order: asc) and notes, and a
		// page onPagesReady added: all by date, newest first.
		It("sorts taxonomies.<name>.<term> by date, newest first, whatever each collection's order", func() {
			Expect(out["index.md"]).To(ContainSubstring("TAGX[B;V;D;C;L1;A;N0;]"))
		})
		It("sorts taxonomy.pages on a term page the same way", func() {
			Expect(out["/tags/x/"]).To(ContainSubstring("TAXP[B;V;D;C;L1;A;N0;]"))
		})
	})

	// alloy dev sorts again on a rebuild: moving a post's date moves it.
	Describe("in an alloy dev rebuild", Ordered, ContinueOnFailure, func() {
		var full, rebuilt, fullTag, rebuiltTag string
		BeforeAll(func() {
			root := GinkgoT().TempDir()
			write := func(rel, body string) {
				GinkgoHelper()
				p := filepath.Join(root, rel)
				Expect(os.MkdirAll(filepath.Dir(p), 0o755)).To(Succeed())
				Expect(os.WriteFile(p, []byte(body), 0o644)).To(Succeed())
			}
			read := func(rel string) string {
				GinkgoHelper()
				b, err := os.ReadFile(filepath.Join(root, "_site", rel))
				Expect(err).NotTo(HaveOccurred())
				return string(b)
			}
			for rel, body := range files() {
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
			full, fullTag = read("index.html"), read("tags/x/index.html")

			// A goes from oldest to newest.
			write("content/posts/a.md", page("A", "date: 2026-04-01\ntags: [x]\n"))
			_, err = pipeline.BuildIncremental(cfg, nil, first.Cache, []string{"content/posts/a.md"},
				pipeline.BuildOptions{PipelineState: s})
			Expect(err).NotTo(HaveOccurred())
			rebuilt, rebuiltTag = read("index.html"), read("tags/x/index.html")
		})

		It("sorts the collection in the full build", func() {
			Expect(full).To(ContainSubstring("POSTS[B;D;C;E;F;A;]"))
		})
		It("moves a page whose date changed, after the rebuild", func() {
			Expect(rebuilt).To(ContainSubstring("POSTS[A;B;D;C;E;F;]"))
		})
		It("moves it in taxonomies.<name>.<term> and on its term page too", func() {
			Expect(fullTag).To(ContainSubstring("TAXP[B;V;D;C;L1;A;N0;]"))
			Expect(rebuilt).To(ContainSubstring("TAGX[A;B;V;D;C;L1;N0;]"))
			Expect(rebuiltTag).To(ContainSubstring("TAXP[A;B;V;D;C;L1;N0;]"))
		})
	})

	// The docs site lists collections.releases and shows the latest release
	// in its header. Its config sorts releases newest first, so its
	// templates must take that order as it is. The release pages here are
	// made up, so path order (v0.10.0, v0.2.0, v0.9.0) differs from date
	// order; the templates and the config are the real ones.
	Describe("on the docs site's release pages", Ordered, ContinueOnFailure, func() {
		var releases string
		BeforeAll(func() {
			repo := func(rel string) string {
				GinkgoHelper()
				b, err := os.ReadFile(filepath.Join("..", "..", rel))
				Expect(err).NotTo(HaveOccurred())
				return string(b)
			}
			siteCfg, err := config.Load(filepath.Join("..", "..", "alloy.config.yaml"))
			Expect(err).NotTo(HaveOccurred())
			Expect(siteCfg.Collections).To(HaveKey("releases"))

			release := func(v, date string) string {
				return "---\ntitle: " + v + "\ndate: " + date + "\ntype: minor\ndescription: notes for " + v + "\n---\n" + v
			}
			content := map[string]string{
				"content/releases/index.md":      repo("docs/content/releases/index.md"),
				"content/releases/_data.yaml":    repo("docs/content/releases/_data.yaml"),
				"content/releases/v0.2.0.md":     release("v0.2.0", "2026-06-25"),
				"content/releases/v0.9.0.md":     release("v0.9.0", "2026-08-01"),
				"content/releases/v0.10.0.md":    release("v0.10.0", "2026-09-01"),
				"layouts/partials/header.liquid": repo("docs/layouts/partials/header.liquid"),
				"layouts/doc.liquid":             "{% include \"partials/header\" %}{{ content }}",
				"layouts/release.liquid":         "{{ content }}",
			}
			cfg := &config.Config{
				Title:       "Docs Releases",
				BaseURL:     "https://example.com",
				Build:       config.BuildConfig{Output: "_site"},
				Collections: siteCfg.Collections,
			}
			result, err := pipeline.BuildWithContent(cfg, content, pipeline.BuildOptions{CaptureRenderedContent: true})
			Expect(err).NotTo(HaveOccurred())
			releases = result.RenderedContent["releases/index.md"]
			Expect(releases).NotTo(BeEmpty())
		})

		It("shows the newest release in the header", func() {
			Expect(releases).To(ContainSubstring(`<span class="latest-release" aria-hidden="true">v0.10.0</span>`))
		})
		It("lists releases newest first, with the Latest badge on the newest", func() {
			Expect(releases).To(MatchRegexp(`(?s)>v0\.10\.0</a></h2>.*release-badge-latest.*>v0\.9\.0</a></h2>.*>v0\.2\.0</a></h2>`))
			Expect(strings.Count(releases, "release-badge-latest")).To(Equal(1))
		})
	})
})
