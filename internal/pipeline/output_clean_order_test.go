package pipeline_test

import (
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/zeroedin/alloy/internal/config"
	"github.com/zeroedin/alloy/internal/pipeline"
)

// ── Output cleaning order (issue #1255) ───────────────────────────────
//
// Build() cleans the output directory at build.go:~235 and validates at
// ~629, so a build that fails validation has already destroyed the previous
// output before deciding the new one was invalid. That also made the claim
// added for issue #1238 false: "Detection runs before any output is written,
// so a failed build leaves no partial output directory behind."
//
// The fix is ordering, not mode-dependence: validate → clean → create →
// render → write, in every mode. Cleaning stays on dev's mid-session full
// rebuild, because a plugin edit re-renders every page and the clean is what
// prevents output from the previous plugin version surviving alongside it.
//
// Spec: PLAN.md → "Output cleaning order (issue #1255)".

var _ = Describe("Output cleaning order (issue #1255)", func() {

	// Two sequential Build() calls against one ProjectRoot are the whole
	// point, which BuildWithContent cannot express — it allocates a fresh
	// temp directory per call.
	newSite := func() (*config.Config, string) {
		tmpDir := GinkgoT().TempDir()
		contentDir := filepath.Join(tmpDir, "content")
		layoutsDir := filepath.Join(tmpDir, "layouts")
		outputDir := filepath.Join(tmpDir, "_site")

		Expect(os.MkdirAll(contentDir, 0755)).To(Succeed())
		Expect(os.MkdirAll(layoutsDir, 0755)).To(Succeed())
		Expect(os.WriteFile(filepath.Join(contentDir, "index.md"),
			[]byte("---\ntitle: Home\nlayout: default\n---\nHome\n"), 0644)).To(Succeed())
		Expect(os.WriteFile(filepath.Join(contentDir, "about.md"),
			[]byte("---\ntitle: About\nlayout: default\n---\nAbout\n"), 0644)).To(Succeed())
		Expect(os.WriteFile(filepath.Join(layoutsDir, "default.liquid"),
			[]byte("<html><body>{{ content }}</body></html>"), 0644)).To(Succeed())

		cfg := &config.Config{
			Title:       "Clean Order Test",
			BaseURL:     "https://example.com",
			ProjectRoot: tmpDir,
			Build:       config.BuildConfig{Output: outputDir},
			Structure: config.StructureConfig{
				Content: "content",
				Layouts: "layouts",
			},
		}
		config.ApplyDefaults(cfg)
		return cfg, outputDir
	}

	outputFiles := func(outputDir string) []string {
		var files []string
		_ = filepath.Walk(outputDir, func(path string, info os.FileInfo, err error) error {
			if err != nil || info.IsDir() {
				return nil
			}
			rel, _ := filepath.Rel(outputDir, path)
			files = append(files, rel)
			return nil
		})
		return files
	}

	// ── Validation failures preserve the previous output ──────────────

	Context("A validation failure runs before the clean", func() {
		It("leaves the previous build's output intact", func() {
			cfg, outputDir := newSite()

			_, err := pipeline.Build(cfg)
			Expect(err).NotTo(HaveOccurred(), "the first build must succeed")
			before := outputFiles(outputDir)
			Expect(before).NotTo(BeEmpty(), "the first build must write output")

			// A colocated file claiming about.md's output path.
			aboutDir := filepath.Join(cfg.ProjectRoot, "content", "about")
			Expect(os.MkdirAll(aboutDir, 0755)).To(Succeed())
			Expect(os.WriteFile(filepath.Join(aboutDir, "index.html"),
				[]byte("<!DOCTYPE html><html><body>stray</body></html>\n"), 0644)).To(Succeed())

			_, err = pipeline.Build(cfg)
			Expect(err).To(HaveOccurred(), "the second build must fail on the conflict")
			Expect(err.Error()).To(ContainSubstring("output path conflict"))

			Expect(outputFiles(outputDir)).To(ConsistOf(before),
				"a conflict is detected before rendering needs to touch anything, so "+
					"it must not destroy the last good build in order to report itself — "+
					"the clean must run after validation, not before it")
		})

		It("still reports the conflict in full", func() {
			// Preserving output must not come at the cost of the diagnostic.
			cfg, _ := newSite()
			_, err := pipeline.Build(cfg)
			Expect(err).NotTo(HaveOccurred())

			aboutDir := filepath.Join(cfg.ProjectRoot, "content", "about")
			Expect(os.MkdirAll(aboutDir, 0755)).To(Succeed())
			Expect(os.WriteFile(filepath.Join(aboutDir, "index.html"),
				[]byte("<!DOCTYPE html><html><body>stray</body></html>\n"), 0644)).To(Succeed())

			_, err = pipeline.Build(cfg)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("about/index.html"),
				"the contested path must still be named")
			Expect(err.Error()).To(ContainSubstring("content/about.md"),
				"both claimants must still be named")
			Expect(err.Error()).To(ContainSubstring("content/about/index.html"))
		})
	})

	// ── The documented boundary ───────────────────────────────────────

	Context("A failure after validation is not covered", func() {
		It("empties the output directory when rendering fails", func() {
			// Documented limit, not an aspiration: rendering fails after the
			// clean has run. Covering it would mean building into a temporary
			// directory and swapping on success — a different design.
			cfg, outputDir := newSite()

			_, err := pipeline.Build(cfg)
			Expect(err).NotTo(HaveOccurred())
			Expect(outputFiles(outputDir)).NotTo(BeEmpty())

			Expect(os.WriteFile(filepath.Join(cfg.ProjectRoot, "content", "broken.md"),
				[]byte("---\ntitle: Broken\nlayout: default\n---\n{{ oops | no_such_filter }}\n"),
				0644)).To(Succeed())

			_, err = pipeline.Build(cfg)
			Expect(err).To(HaveOccurred(), "the render error must fail the build")
			Expect(outputFiles(outputDir)).To(BeEmpty(),
				"a post-validation failure happens after the clean, so the output "+
					"directory is left empty — PLAN.md states this boundary rather "+
					"than implying output is always preserved")
		})
	})

	// ── Cleaning still happens, and still cleans ──────────────────────

	Context("Cleaning is unchanged for successful builds", func() {
		It("removes output that the new build no longer produces", func() {
			// The reorder must not weaken the clean. A page removed between
			// builds must not leave its old output behind — this is what makes
			// a plugin edit safe, since every page re-renders and stale output
			// from the previous plugin version must not survive.
			cfg, outputDir := newSite()

			_, err := pipeline.Build(cfg)
			Expect(err).NotTo(HaveOccurred())
			Expect(outputFiles(outputDir)).To(ContainElement(filepath.Join("about", "index.html")))

			Expect(os.Remove(filepath.Join(cfg.ProjectRoot, "content", "about.md"))).To(Succeed())

			_, err = pipeline.Build(cfg)
			Expect(err).NotTo(HaveOccurred())
			Expect(outputFiles(outputDir)).NotTo(ContainElement(filepath.Join("about", "index.html")),
				"a successful build must still clean — output for a page that no "+
					"longer exists must not survive")
		})

		It("keeps previous output when build.clean is false", func() {
			cfg, outputDir := newSite()

			_, err := pipeline.Build(cfg)
			Expect(err).NotTo(HaveOccurred())

			stray := filepath.Join(outputDir, "leftover.txt")
			Expect(os.WriteFile(stray, []byte("kept\n"), 0644)).To(Succeed())

			clean := false
			cfg.Build.Clean = &clean

			_, err = pipeline.Build(cfg)
			Expect(err).NotTo(HaveOccurred())
			Expect(stray).To(BeAnExistingFile(),
				"build.clean: false suppresses the clean entirely — the reorder "+
					"changes when a clean happens, never whether an opted-out "+
					"clean is reinstated")
		})
	})
})
