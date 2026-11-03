package pipeline_test

import (
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/zeroedin/alloy/internal/config"
	"github.com/zeroedin/alloy/internal/pipeline"
)

// ── Liquid is the only template engine (issue #1285) ─────────────────
// Go template support and the `templates.engine` setting are removed. A
// leftover `templates:` key in a site's config is ignored like any other
// unknown key, and every layout renders with Liquid: `.liquid` first, then
// the bare `.html` fallback.
//
// See PLAN.md → "Liquid is the only template engine (issue #1285)".

var _ = Describe("Liquid is the only template engine (issue #1285)", func() {

	// buildFromDisk writes a site whose alloy.config.yaml still says
	// `templates: engine: gotemplate`, loads that config the way the CLI
	// does, and returns the rendered index page.
	buildFromDisk := func(layoutName, layout string) string {
		GinkgoHelper()
		dir := GinkgoT().TempDir()
		files := map[string]string{
			"alloy.config.yaml": "title: T\nbaseURL: https://example.com\n" +
				"templates:\n  engine: gotemplate\n",
			"content/index.md":      "---\ntitle: Home\nlayout: default\n---\nBody",
			"layouts/" + layoutName: layout,
		}
		for name, body := range files {
			path := filepath.Join(dir, name)
			Expect(os.MkdirAll(filepath.Dir(path), 0755)).To(Succeed())
			Expect(os.WriteFile(path, []byte(body), 0644)).To(Succeed())
		}
		cfg, err := config.Load(filepath.Join(dir, "alloy.config.yaml"))
		Expect(err).NotTo(HaveOccurred(), "a leftover templates: key must not fail config loading")
		result, err := pipeline.Build(cfg, pipeline.BuildOptions{CaptureRenderedContent: true})
		Expect(err).NotTo(HaveOccurred())
		return result.RenderedContent["index.md"]
	}

	It("ignores a leftover templates.engine and renders the .liquid layout with Liquid", func() {
		Expect(buildFromDisk("default.liquid", "L[{{ page.title | upcase }}]")).To(
			ContainSubstring("L[HOME]"),
			"templates.engine is gone, so a site that still sets it to gotemplate "+
				"renders with Liquid (#1285)")
	})

	It("reads a bare .html layout as Liquid", func() {
		Expect(buildFromDisk("default.html", "H[{{ page.title | upcase }}]")).To(
			ContainSubstring("H[HOME]"),
			"a .html layout is Liquid's bare-extension fallback, never a Go template (#1285)")
	})

	It("fails the build on a safe* filter, which no longer exists", func() {
		_, err := pipeline.BuildWithContent(&config.Config{
			Title:   "Safe Filter",
			BaseURL: "https://example.com",
			Build:   config.BuildConfig{Output: "_site"},
		}, map[string]string{
			"content/index.md":       "---\ntitle: Home\nlayout: default\n---\nBody",
			"layouts/default.liquid": `S[{{ "<b>x</b>" | safeHTML }}]`,
		})
		Expect(err).To(HaveOccurred(),
			"safeHTML existed only for Go templates and is removed with them (#1285)")
		Expect(err.Error()).To(ContainSubstring("undefined filter safeHTML"))
	})
})
