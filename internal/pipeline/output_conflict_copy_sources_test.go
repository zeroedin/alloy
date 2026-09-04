package pipeline_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/zeroedin/alloy/internal/config"
	"github.com/zeroedin/alloy/internal/pipeline"
)

// ── Copy sources in output path conflict detection (issue #1238) ──────
//
// PLAN.md §Pre-Build Validation specifies that every source writing into the
// output directory participates in conflict detection, and that conflicts are
// always errors with no priority system. The implementation only fed pages,
// taxonomy pages, and plugin addOutputs to the detector, so the four
// file-copy sources — static/, assets/, passthrough, and content-colocated —
// collided silently, resolved by copy order (last writer wins).
//
// The most damaging case is a content-colocated file claiming a rendered
// page's output path: the page is replaced by a stray file and the build
// still reports success.
//
// Spec: PLAN.md → Pre-Build Validation → "Sources scanned" / "Priority rules".

var _ = Describe("Copy sources in output path conflict detection (issue #1238)", func() {

	const layout = `<html><body>{{ content }}</body></html>`

	baseCfg := func() *config.Config {
		return &config.Config{
			Title:   "Conflict Test",
			BaseURL: "https://example.com",
			Build:   config.BuildConfig{Output: "_site"},
		}
	}

	// ── The silent-data-loss case ─────────────────────────────────────

	Context("A colocated file claiming a rendered page's path", func() {
		It("errors instead of silently replacing the page", func() {
			// content/about.md renders to _site/about/index.html.
			// content/about/index.html is a full document with no front
			// matter, so discovery classifies it as colocated passthrough
			// and copies it to the same path — after the page was written.
			cfg := baseCfg()
			files := map[string]string{
				"layouts/default.liquid":   layout,
				"content/about.md":         "---\ntitle: About\nlayout: default\n---\nRENDERED PAGE BODY\n",
				"content/about/index.html": "<!DOCTYPE html><html><body>COLOCATED STRAY FILE</body></html>\n",
			}
			result, err := pipeline.BuildWithContent(cfg, files)
			Expect(err).To(HaveOccurred(),
				"a colocated file claiming a rendered page's output path must fail "+
					"the build — today it silently replaces the page and the build "+
					"reports success with exit 0")
			Expect(result).To(BeNil(),
				"a failed build must not return a partial result")
			Expect(err.Error()).To(ContainSubstring("output path conflict"))
			Expect(err.Error()).To(ContainSubstring("about/index.html"),
				"the error must name the contested output path")
			Expect(err.Error()).To(ContainSubstring("content/about.md"),
				"the error must name the rendered page as one claimant")
			Expect(err.Error()).To(ContainSubstring("content/about/index.html"),
				"the error must name the colocated file as the other claimant")
		})
	})

	// ── Each copy source must reach the detector ──────────────────────

	Context("Each file-copy source participates", func() {
		It("errors when static and asset files claim one path", func() {
			cfg := baseCfg()
			files := map[string]string{
				"layouts/default.liquid": layout,
				"content/index.md":       "---\ntitle: Home\nlayout: default\n---\nHome\n",
				"static/css/styles.css":  "/* STATIC */\n",
				"assets/css/styles.css":  "/* ASSET */\n",
			}
			_, err := pipeline.BuildWithContent(cfg, files)
			Expect(err).To(HaveOccurred(),
				"static/ and assets/ claiming one output path must be an error — "+
					"there is no layering where static overrides assets")
			Expect(err.Error()).To(ContainSubstring("output path conflict"))
			Expect(err.Error()).To(ContainSubstring("css/styles.css"))
			Expect(err.Error()).To(ContainSubstring("static/css/styles.css"))
			Expect(err.Error()).To(ContainSubstring("assets/css/styles.css"))
		})

		It("errors when a passthrough mapping collides with a static file", func() {
			cfg := baseCfg()
			cfg.Passthrough = []config.PassthroughMapping{{From: "vendor-css", To: "css"}}
			files := map[string]string{
				"layouts/default.liquid": layout,
				"content/index.md":       "---\ntitle: Home\nlayout: default\n---\nHome\n",
				"static/css/styles.css":  "/* STATIC */\n",
				"vendor-css/styles.css":  "/* PASSTHROUGH */\n",
			}
			_, err := pipeline.BuildWithContent(cfg, files)
			Expect(err).To(HaveOccurred(),
				"a passthrough mapping and a static file claiming one output path "+
					"must be an error — this is the exact example PLAN.md gives")
			Expect(err.Error()).To(ContainSubstring("output path conflict"))
			Expect(err.Error()).To(ContainSubstring("css/styles.css"))
			Expect(err.Error()).To(ContainSubstring("vendor-css"),
				"the passthrough claimant must be identifiable by its mapping, "+
					"not just by the resolved file path")
		})

		It("reports every source when all four copy sources claim one path", func() {
			cfg := baseCfg()
			cfg.Passthrough = []config.PassthroughMapping{{From: "vendor-css", To: "css"}}
			files := map[string]string{
				"layouts/default.liquid": layout,
				"content/index.md":       "---\ntitle: Home\nlayout: default\n---\nHome\n",
				"static/css/styles.css":  "/* STATIC */\n",
				"assets/css/styles.css":  "/* ASSET */\n",
				"vendor-css/styles.css":  "/* PASSTHROUGH */\n",
				"content/css/styles.css": "/* COLOCATED */\n",
			}
			_, err := pipeline.BuildWithContent(cfg, files)
			Expect(err).To(HaveOccurred())
			msg := err.Error()
			Expect(msg).To(ContainSubstring("output path conflict"))
			// All four must be named. Reporting only the first two leaves the
			// user fixing one collision per build with no idea how many remain.
			Expect(msg).To(ContainSubstring("static/css/styles.css"),
				"every claimant must be listed, not just the first two")
			Expect(msg).To(ContainSubstring("assets/css/styles.css"))
			Expect(msg).To(ContainSubstring("vendor-css"))
			Expect(msg).To(ContainSubstring("content/css/styles.css"))
		})
	})

	// ── What must NOT be reported ─────────────────────────────────────

	Context("Non-conflicts must keep building", func() {
		It("allows four sources to merge into one directory with unique names", func() {
			// Directory overlap is not a conflict — only identical output
			// paths are. This is the common, correct arrangement and must
			// not regress when the copy sources start feeding the detector.
			cfg := baseCfg()
			cfg.Passthrough = []config.PassthroughMapping{{From: "vendor-css", To: "css"}}
			files := map[string]string{
				"layouts/default.liquid": layout,
				"content/index.md":       "---\ntitle: Home\nlayout: default\n---\nHome\n",
				"static/css/static.css":  "/* STATIC */\n",
				"assets/css/asset.css":   "/* ASSET */\n",
				"vendor-css/vendor.css":  "/* PASSTHROUGH */\n",
				"content/css/inline.css": "/* COLOCATED */\n",
			}
			_, err := pipeline.BuildWithContent(cfg, files)
			Expect(err).NotTo(HaveOccurred(),
				"four sources writing distinct filenames into one output "+
					"directory is correct and must keep working")
		})

		It("does not report a passthrough file excluded by pattern", func() {
			// The scanned set must equal the set actually copied. An excluded
			// file is never written, so it cannot collide with anything.
			cfg := baseCfg()
			cfg.Passthrough = []config.PassthroughMapping{
				{From: "vendor-css", To: "css", Exclude: []string{"styles.css"}},
			}
			files := map[string]string{
				"layouts/default.liquid": layout,
				"content/index.md":       "---\ntitle: Home\nlayout: default\n---\nHome\n",
				"static/css/styles.css":  "/* STATIC */\n",
				"vendor-css/styles.css":  "/* EXCLUDED, never copied */\n",
			}
			_, err := pipeline.BuildWithContent(cfg, files)
			Expect(err).NotTo(HaveOccurred(),
				"a passthrough file excluded by pattern is never copied, so it "+
					"must not be recorded as claiming an output path — collecting "+
					"entries before applying exclude patterns would report a "+
					"conflict that cannot happen")
		})

		It("allows a colocated file whose path no page claims", func() {
			cfg := baseCfg()
			files := map[string]string{
				"layouts/default.liquid":    layout,
				"content/about.md":          "---\ntitle: About\nlayout: default\n---\nAbout\n",
				"content/downloads/doc.pdf": "%PDF-1.4 fake\n",
			}
			_, err := pipeline.BuildWithContent(cfg, files)
			Expect(err).NotTo(HaveOccurred(),
				"colocated files are a normal authoring pattern — only ones "+
					"claiming an already-claimed path are errors")
		})
	})
})
