package pipeline_test

import (
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/zeroedin/alloy/internal/config"
	"github.com/zeroedin/alloy/internal/pipeline"
)

// ── alloy dev picks up _data.yaml edits (issue #1306) ──────────────────
// alloy dev creates one PipelineState at startup and reuses it for every
// incremental rebuild. Before #1306, the state read every _data.yaml once
// and never again, so an edit to folder data was ignored until a restart,
// even when the page under that folder was itself rebuilt. #717 fixed the
// same gap for files in data/, but not for _data.yaml.
//
// These tests drive BuildIncremental the way cmd/dev.go does: one
// PipelineState, the previous build's cache, and the changed paths relative
// to the project root.
//
// See PLAN.md → "alloy dev picks up _data.yaml edits (issue #1306)".

var _ = Describe("alloy dev picks up _data.yaml edits (issue #1306)", func() {
	const layoutBody = "badge=[{{ page.badge }}] {{ content }}"

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

	// newSite writes the files, then creates the PipelineState once, as
	// alloy dev does at startup.
	newSite := func(files map[string]string) site {
		GinkgoHelper()
		root := GinkgoT().TempDir()
		s := site{root: root}
		files["layouts/default.liquid"] = "DEFAULT " + layoutBody
		files["layouts/post.liquid"] = "POST " + layoutBody
		for rel, body := range files {
			write(s, rel, body)
		}
		s.cfg = &config.Config{
			Title:       "Cascade Reload",
			BaseURL:     "https://example.com",
			ProjectRoot: root,
			Build:       config.BuildConfig{Output: filepath.Join(root, "_site")},
			Structure: config.StructureConfig{
				Content: "content",
				Layouts: "layouts",
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

	rebuild := func(s site, prev *pipeline.BuildResult, changed ...string) (*pipeline.BuildResult, error) {
		return pipeline.BuildIncremental(s.cfg, nil, prev.Cache, changed, pipeline.BuildOptions{
			PipelineState:          s.state,
			CaptureRenderedContent: true,
		})
	}

	firstBuild := func(s site) *pipeline.BuildResult {
		GinkgoHelper()
		result, err := pipeline.BuildIncremental(s.cfg, nil, nil, nil, pipeline.BuildOptions{
			PipelineState:          s.state,
			CaptureRenderedContent: true,
		})
		Expect(err).NotTo(HaveOccurred())
		return result
	}

	blogSite := func() site {
		GinkgoHelper()
		return newSite(map[string]string{
			"content/index.md":        "---\ntitle: Home\n---\nhome",
			"content/blog/_data.yaml": "badge: one\n",
			"content/blog/p.md":       "---\ntitle: P\n---\npost",
			"content/blog/deep/q.md":  "---\ntitle: Q\n---\ndeep",
			"content/docs/_data.yaml": "badge: docs\n",
			"content/docs/guide.md":   "---\ntitle: Guide\n---\nguide",
		})
	}

	It("shows an edited _data.yaml value on the pages in that folder", func() {
		s := blogSite()
		first := firstBuild(s)
		Expect(first.RenderedContent["blog/p.md"]).To(ContainSubstring("badge=[one]"),
			"sanity: the first build reads the cascade")

		write(s, "content/blog/_data.yaml", "badge: two\n")
		result, err := rebuild(s, first, "content/blog/_data.yaml")
		Expect(err).NotTo(HaveOccurred())

		Expect(result.PagesRendered).To(ContainElement("blog/p.md"),
			"a _data.yaml change must re-render the pages in its folder")
		Expect(result.RenderedContent["blog/p.md"]).To(ContainSubstring("badge=[two]"),
			"the rebuilt page must use the edited _data.yaml, not the copy read at startup")
	})

	It("shows the edit on pages in folders below it", func() {
		s := blogSite()
		first := firstBuild(s)

		write(s, "content/blog/_data.yaml", "badge: two\n")
		result, err := rebuild(s, first, "content/blog/_data.yaml")
		Expect(err).NotTo(HaveOccurred())

		Expect(result.PagesRendered).To(ContainElement("blog/deep/q.md"),
			"folder data cascades to subfolders, so their pages are re-rendered too")
		Expect(result.RenderedContent["blog/deep/q.md"]).To(ContainSubstring("badge=[two]"))
	})

	It("does not re-render pages outside that folder", func() {
		s := blogSite()
		first := firstBuild(s)

		write(s, "content/blog/_data.yaml", "badge: two\n")
		result, err := rebuild(s, first, "content/blog/_data.yaml")
		Expect(err).NotTo(HaveOccurred())

		Expect(result.PagesRendered).NotTo(ContainElement("index.md"),
			"a page above the changed folder doesn't read its data")
		Expect(result.PagesRendered).NotTo(ContainElement("docs/guide.md"),
			"a page in a sibling folder doesn't read its data")
	})

	It("uses the edit when a page in the folder is rebuilt later", func() {
		s := blogSite()
		first := firstBuild(s)

		write(s, "content/blog/_data.yaml", "badge: two\n")
		second, err := rebuild(s, first, "content/blog/_data.yaml")
		Expect(err).NotTo(HaveOccurred())

		write(s, "content/blog/p.md", "---\ntitle: P\n---\npost edited")
		third, err := rebuild(s, second, "content/blog/p.md")
		Expect(err).NotTo(HaveOccurred())
		Expect(third.RenderedContent["blog/p.md"]).To(ContainSubstring("badge=[two]"),
			"a later page edit must not bring back the startup value")
		Expect(third.RenderedContent["blog/p.md"]).To(ContainSubstring("post edited"))
	})

	It("drops the values of a deleted _data.yaml", func() {
		s := blogSite()
		first := firstBuild(s)

		Expect(os.Remove(filepath.Join(s.root, "content/blog/_data.yaml"))).To(Succeed())
		result, err := rebuild(s, first, "content/blog/_data.yaml")
		Expect(err).NotTo(HaveOccurred())

		Expect(result.RenderedContent["blog/p.md"]).To(ContainSubstring("badge=[]"),
			"once the file is gone, its values no longer apply")
	})

	It("applies a _data.yaml added while alloy dev is running", func() {
		s := blogSite()
		first := firstBuild(s)

		write(s, "content/blog/deep/_data.yaml", "badge: deep\n")
		result, err := rebuild(s, first, "content/blog/deep/_data.yaml")
		Expect(err).NotTo(HaveOccurred())

		Expect(result.RenderedContent["blog/deep/q.md"]).To(ContainSubstring("badge=[deep]"),
			"a new _data.yaml applies to the pages in its folder")
	})

	It("handles the _data.yml spelling too", func() {
		s := newSite(map[string]string{
			"content/blog/_data.yml": "badge: one\n",
			"content/blog/p.md":      "---\ntitle: P\n---\npost",
		})
		first := firstBuild(s)

		write(s, "content/blog/_data.yml", "badge: two\n")
		result, err := rebuild(s, first, "content/blog/_data.yml")
		Expect(err).NotTo(HaveOccurred())

		Expect(result.RenderedContent["blog/p.md"]).To(ContainSubstring("badge=[two]"),
			"_data.yml is reloaded the same way as _data.yaml")
	})

	It("works with a content directory that isn't named content", func() {
		root := GinkgoT().TempDir()
		s := site{root: root}
		write(s, "layouts/default.liquid", "DEFAULT "+layoutBody)
		write(s, "src/blog/_data.yaml", "badge: one\n")
		write(s, "src/blog/p.md", "---\ntitle: P\n---\npost")
		s.cfg = &config.Config{
			Title:       "Cascade Reload",
			BaseURL:     "https://example.com",
			ProjectRoot: root,
			Build:       config.BuildConfig{Output: filepath.Join(root, "_site")},
			Structure:   config.StructureConfig{Content: "src", Layouts: "layouts"},
		}
		config.ApplyDefaults(s.cfg)
		registry, hooks, _ := pipeline.DiscoverPlugins(s.cfg)
		DeferCleanup(registry.Close)
		state, err := pipeline.InitPipelineState(s.cfg, registry, hooks)
		Expect(err).NotTo(HaveOccurred())
		s.state = state
		first := firstBuild(s)
		Expect(first.RenderedContent["blog/p.md"]).To(ContainSubstring("badge=[one]"))

		write(s, "src/blog/_data.yaml", "badge: two\n")
		result, err := rebuild(s, first, "src/blog/_data.yaml")
		Expect(err).NotTo(HaveOccurred())

		Expect(result.RenderedContent["blog/p.md"]).To(ContainSubstring("badge=[two]"),
			"the folder is matched against structure.content, not a hard-coded content/")
	})

	// The case that surfaced #1306: with #1296, a misspelled cascade layout
	// fails the build. Fixing the file has to clear the error.
	It("reports a misspelled cascade layout, then clears it once fixed", func() {
		s := newSite(map[string]string{
			"content/blog/_data.yaml": "layout: post\n",
			"content/blog/p.md":       "---\ntitle: P\n---\npost",
		})
		first := firstBuild(s)
		Expect(first.RenderedContent["blog/p.md"]).To(HavePrefix("POST "),
			"sanity: the cascade layout applies")

		write(s, "content/blog/_data.yaml", "layout: psot\n")
		_, err := rebuild(s, first, "content/blog/_data.yaml")
		Expect(err).To(MatchError(ContainSubstring(`layout "psot" not found for page "blog/p.md"`)),
			"the misspelled name must reach the build, which then fails (issue #1296)")

		// alloy dev keeps the last good cache after a failed rebuild.
		write(s, "content/blog/_data.yaml", "layout: default\n")
		result, err := rebuild(s, first, "content/blog/_data.yaml")
		Expect(err).NotTo(HaveOccurred(),
			"fixing the file must clear the error without a restart")
		Expect(result.RenderedContent["blog/p.md"]).To(HavePrefix("DEFAULT "))
	})

	// alloy dev runs a full Build for some changes (10 or more files at once,
	// or a plugin or component change), then goes back to BuildIncremental
	// with the same PipelineState.
	It("uses the current _data.yaml after a full rebuild", func() {
		s := blogSite()
		first := firstBuild(s)
		Expect(first.RenderedContent["blog/p.md"]).To(ContainSubstring("badge=[one]"))

		write(s, "content/blog/_data.yaml", "badge: two\n")
		full, err := pipeline.Build(s.cfg, pipeline.BuildOptions{CaptureRenderedContent: true})
		Expect(err).NotTo(HaveOccurred())
		Expect(full.RenderedContent["blog/p.md"]).To(ContainSubstring("badge=[two]"),
			"sanity: a full build reads the cascade from disk")

		write(s, "content/blog/p.md", "---\ntitle: P\n---\npost edited")
		result, err := rebuild(s, full, "content/blog/p.md")
		Expect(err).NotTo(HaveOccurred())
		Expect(result.RenderedContent["blog/p.md"]).To(ContainSubstring("badge=[two]"),
			"the next incremental rebuild must not go back to the startup value")
	})

	// A root content/_data.yaml applies to every page, so editing it
	// re-renders the whole site (issue #1322).
	It("re-renders every page when the root _data.yaml changes", func() {
		s := newSite(map[string]string{
			"content/_data.yaml":     "badge: root1\n",
			"content/index.md":       "---\ntitle: Home\n---\nhome",
			"content/docs/guide.md":  "---\ntitle: Guide\n---\nguide",
			"content/blog/deep/q.md": "---\ntitle: Q\n---\ndeep",
		})
		first := firstBuild(s)
		Expect(first.RenderedContent["docs/guide.md"]).To(ContainSubstring("badge=[root1]"),
			"sanity: the root cascade applies to pages in subfolders")

		write(s, "content/_data.yaml", "badge: root2\n")
		result, err := rebuild(s, first, "content/_data.yaml")
		Expect(err).NotTo(HaveOccurred())

		for _, page := range []string{"index.md", "docs/guide.md", "blog/deep/q.md"} {
			Expect(result.PagesRendered).To(ContainElement(page),
				"a root _data.yaml change must re-render %s", page)
			Expect(result.RenderedContent[page]).To(ContainSubstring("badge=[root2]"),
				"%s must show the edited root value", page)
		}
	})

	// While any _data.yaml can't be parsed, the cascade can't load, so pages
	// rendered in that rebuild get no folder data. Once the broken file is
	// fixed, the next rebuild renders every page, once (issue #1322).
	It("renders every page once after a broken _data.yaml is fixed", func() {
		s := blogSite()
		first := firstBuild(s)

		write(s, "content/docs/_data.yaml", "badge: [unclosed\n")
		write(s, "content/blog/_data.yaml", "badge: two\n")
		broken, err := rebuild(s, first, "content/docs/_data.yaml", "content/blog/_data.yaml")
		Expect(err).NotTo(HaveOccurred(),
			"sanity: a _data.yaml that can't be parsed warns, it doesn't fail the rebuild")

		write(s, "content/docs/_data.yaml", "badge: docs\n")
		fixed, err := rebuild(s, broken, "content/docs/_data.yaml")
		Expect(err).NotTo(HaveOccurred())
		Expect(fixed.RenderedContent["blog/p.md"]).To(ContainSubstring("badge=[two]"),
			"fixing the broken file must also re-render pages outside its folder that were "+
				"rendered without folder data while the cascade couldn't load")

		write(s, "content/index.md", "---\ntitle: Home\n---\nhome edited")
		next, err := rebuild(s, fixed, "content/index.md")
		Expect(err).NotTo(HaveOccurred())
		Expect(next.PagesRendered).NotTo(ContainElement("blog/p.md"),
			"the full render happens only once; the rebuild after it is incremental again")
	})

	// Guard: the fix must not make every rebuild re-render the folder.
	It("doesn't re-render a folder's pages for an unrelated edit", func() {
		s := blogSite()
		first := firstBuild(s)

		write(s, "content/index.md", "---\ntitle: Home\n---\nhome edited")
		result, err := rebuild(s, first, "content/index.md")
		Expect(err).NotTo(HaveOccurred())

		Expect(result.PagesRendered).To(ContainElement("index.md"))
		Expect(result.PagesRendered).NotTo(ContainElement("blog/p.md"))
		Expect(result.PagesRendered).NotTo(ContainElement("docs/guide.md"))
	})
})
