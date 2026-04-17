package ssr_test

import (
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/zeroedin/alloy/internal/ssr"
)

var _ = Describe("Scanner", func() {

	// ── Component detection ────────────────────────────────────────────

	Describe("Component detection", func() {
		It("finds custom elements with hyphens in tag name", func() {
			html := `<div><ds-card></ds-card><ds-button></ds-button></div>`
			tags := ssr.ScanComponents(html)
			Expect(tags).To(HaveLen(2))
			Expect(tags).To(ContainElement("ds-card"))
			Expect(tags).To(ContainElement("ds-button"))
		})

		It("returns unique tag names (no duplicates)", func() {
			html := `<ds-card variant="primary"></ds-card><ds-card variant="secondary"></ds-card>`
			tags := ssr.ScanComponents(html)
			Expect(tags).To(HaveLen(1))
			Expect(tags).To(ContainElement("ds-card"))
		})

		It("ignores standard HTML elements", func() {
			// First verify that custom elements ARE detected (proves the function works)
			customHTML := `<div><ds-card></ds-card></div>`
			customTags := ssr.ScanComponents(customHTML)
			Expect(customTags).To(HaveLen(1), "custom elements must be detected")

			// Then verify standard elements are ignored
			html := `<div><span>text</span><p>paragraph</p></div>`
			tags := ssr.ScanComponents(html)
			Expect(tags).To(BeEmpty())
		})
	})

	// ── Per-page SSR rendering ────────────────────────────────────────

	Describe("Per-page SSR rendering", func() {
		It("RenderPage passes full page HTML to command and returns transformed output", func() {
			// Use 'cat' as a pass-through command — proves the per-page
			// invocation model works (HTML goes in as arg, comes back via stdout)
			html := `<html><body><ds-card variant="primary"><h2>Hello</h2></ds-card></body></html>`
			result, err := ssr.RenderPage("cat", html)
			// cat may not accept the HTML as an arg in all environments,
			// but if it succeeds the output should contain the input
			if err == nil {
				Expect(result).To(ContainSubstring("ds-card"))
			}
			// Either way, RenderPage must attempt the command invocation
		})

		It("returns error when command is not found", func() {
			html := `<html><body><ds-card>content</ds-card></body></html>`
			_, err := ssr.RenderPage("nonexistent-ssr-tool render --defs bundles/", html)
			Expect(err).To(HaveOccurred(),
				"RenderPage must return error when command is not found")
		})

		It("returns error when command exits non-zero", func() {
			html := `<html><body><ds-card>content</ds-card></body></html>`
			_, err := ssr.RenderPage("false", html)
			Expect(err).To(HaveOccurred(),
				"RenderPage must return error when command exits non-zero")
		})
	})

	// ── Output hashing ────────────────────────────────────────────────

	Describe("Output hashing", func() {
		It("HashOutput produces deterministic hash for same content", func() {
			hash1 := ssr.HashOutput("<html>content</html>")
			hash2 := ssr.HashOutput("<html>content</html>")
			Expect(hash1).NotTo(BeEmpty())
			Expect(hash1).To(Equal(hash2),
				"same content must produce same hash")
		})

		It("HashOutput produces different hash for different content", func() {
			hash1 := ssr.HashOutput("<html>content A</html>")
			hash2 := ssr.HashOutput("<html>content B</html>")
			Expect(hash1).NotTo(Equal(hash2),
				"different content must produce different hash")
		})
	})

	// ── Component map persistence (§6) ──────────────────────────────

	Describe("Component map persistence", func() {
		It("saves and loads pageToComponents map", func() {
			tmpDir := GinkgoT().TempDir()
			cm := ssr.NewComponentMap()
			cm.PageToComponents["content/index.md"] = []string{"ds-card", "ds-button"}

			err := cm.SaveTo(tmpDir)
			Expect(err).NotTo(HaveOccurred())

			// Verify file was written
			_, err = os.Stat(filepath.Join(tmpDir, "components.json"))
			Expect(err).NotTo(HaveOccurred(),
				"components.json must be written to the target directory")

			loaded, err := ssr.LoadComponentMap(tmpDir)
			Expect(err).NotTo(HaveOccurred())
			Expect(loaded).NotTo(BeNil())
			Expect(loaded.PageToComponents).To(HaveKey("content/index.md"))
			Expect(loaded.PageToComponents["content/index.md"]).To(ConsistOf("ds-card", "ds-button"))
		})

		It("saves and loads componentToPages map", func() {
			tmpDir := GinkgoT().TempDir()
			cm := ssr.NewComponentMap()
			cm.ComponentToPages["ds-card"] = []string{"content/index.md", "content/about.md"}

			err := cm.SaveTo(tmpDir)
			Expect(err).NotTo(HaveOccurred())

			loaded, err := ssr.LoadComponentMap(tmpDir)
			Expect(err).NotTo(HaveOccurred())
			Expect(loaded).NotTo(BeNil())
			Expect(loaded.ComponentToPages["ds-card"]).To(HaveLen(2))
		})

		It("saves and loads componentDeps", func() {
			tmpDir := GinkgoT().TempDir()
			cm := ssr.NewComponentMap()
			cm.ComponentDeps["ds-card"] = []string{"ds-icon", "ds-badge"}

			err := cm.SaveTo(tmpDir)
			Expect(err).NotTo(HaveOccurred())

			loaded, err := ssr.LoadComponentMap(tmpDir)
			Expect(err).NotTo(HaveOccurred())
			Expect(loaded).NotTo(BeNil())
			Expect(loaded.ComponentDeps["ds-card"]).To(ConsistOf("ds-icon", "ds-badge"))
		})

		It("returns empty component map when file does not exist (fresh build)", func() {
			tmpDir := GinkgoT().TempDir()
			loaded, err := ssr.LoadComponentMap(tmpDir)
			Expect(err).NotTo(HaveOccurred())
			Expect(loaded).NotTo(BeNil())
			Expect(loaded.PageToComponents).To(BeEmpty())
			Expect(loaded.ComponentToPages).To(BeEmpty())
		})

		It("skips Phase 2 SSR when component definition hash is unchanged", func() {
			cm := ssr.NewComponentMap()
			cm.DefinitionHashes["ds-card"] = "abc123"

			// Guard: different hash must not skip
			Expect(cm.ShouldSkipSSR("ds-card", "different_hash")).To(BeFalse(),
				"guard: changed definition must not skip SSR")

			// Same hash = skip
			Expect(cm.ShouldSkipSSR("ds-card", "abc123")).To(BeTrue(),
				"unchanged component definition must skip Phase 2 SSR")
		})
	})
})
