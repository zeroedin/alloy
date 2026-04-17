package ssr_test

import (
	"context"
	"os"
	"path/filepath"
	"time"

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
		It("RenderPage pipes full page HTML via stdin and returns stdout", func() {
			// Use 'cat' as a pass-through command — reads stdin, writes to stdout.
			// Proves the stdio contract works end-to-end.
			html := `<html><body><ds-card variant="primary"><h2>Hello</h2></ds-card></body></html>`
			result, err := ssr.RenderPage("cat", html)
			Expect(err).NotTo(HaveOccurred(),
				"cat must succeed as a pass-through stdin→stdout command")
			Expect(result).To(ContainSubstring("ds-card"),
				"stdout must contain the HTML that was piped via stdin")
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

	// ── Exec mode timeout ────────────────────────────────────────────

	Describe("Exec mode timeout", func() {
		It("RenderPageWithTimeout returns error when command exceeds deadline", func() {
			// sleep 60 will hang — context timeout must kill it
			ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
			defer cancel()
			html := `<html><body><ds-card>content</ds-card></body></html>`
			_, err := ssr.RenderPageWithTimeout(ctx, "sleep 60", html)
			Expect(err).To(HaveOccurred(),
				"RenderPageWithTimeout must return error when command exceeds timeout")
		})

		It("RenderPageWithTimeout succeeds within deadline", func() {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			html := `<html><body><ds-card>content</ds-card></body></html>`
			result, err := ssr.RenderPageWithTimeout(ctx, "cat", html)
			Expect(err).NotTo(HaveOccurred(),
				"cat must complete within 5s timeout")
			Expect(result).To(ContainSubstring("ds-card"))
		})
	})

	// ── Stream mode SSR rendering ─────────────────────────────────────

	Describe("Stream mode SSR rendering", func() {
		It("NewStreamRenderer starts a persistent process", func() {
			// Use 'cat' — it reads stdin and echoes to stdout, stays alive
			// until stdin is closed. Proves the persistent process model works.
			sr, err := ssr.NewStreamRenderer("cat")
			Expect(err).NotTo(HaveOccurred(),
				"NewStreamRenderer must start a persistent process")
			Expect(sr).NotTo(BeNil())
			Expect(sr.Close()).To(Succeed())
		})

		It("renders multiple pages on a single persistent process", func() {
			sr, err := ssr.NewStreamRenderer("cat")
			Expect(err).NotTo(HaveOccurred())
			defer sr.Close()

			// First page
			html1 := `<html><body><ds-card>Page 1</ds-card></body></html>`
			result1, err := sr.RenderPage(html1)
			Expect(err).NotTo(HaveOccurred())
			Expect(result1).To(ContainSubstring("Page 1"),
				"first page must be rendered via the persistent process")

			// Second page — same process, no restart
			html2 := `<html><body><ds-button>Page 2</ds-button></body></html>`
			result2, err := sr.RenderPage(html2)
			Expect(err).NotTo(HaveOccurred())
			Expect(result2).To(ContainSubstring("Page 2"),
				"second page must be rendered on the same persistent process")
		})

		It("returns error when stream command is not found", func() {
			_, err := ssr.NewStreamRenderer("nonexistent-ssr-tool serve --stdio")
			Expect(err).To(HaveOccurred(),
				"NewStreamRenderer must return error when command is not found")
		})

		It("Close shuts down the persistent process", func() {
			sr, err := ssr.NewStreamRenderer("cat")
			Expect(err).NotTo(HaveOccurred())
			err = sr.Close()
			Expect(err).To(Succeed(),
				"Close must cleanly shut down the persistent process")
		})
	})

	// ── Stream error recovery ─────────────────────────────────────────

	Describe("Stream error recovery", func() {
		It("Restart creates a new process after the previous one is closed", func() {
			sr, err := ssr.NewStreamRenderer("cat")
			Expect(err).NotTo(HaveOccurred())

			// Close the process (simulates crash)
			Expect(sr.Close()).To(Succeed())

			// Restart must create a new process
			err = sr.Restart()
			Expect(err).NotTo(HaveOccurred(),
				"Restart must successfully start a new process")

			// The new process must work
			html := `<html><body><ds-card>after restart</ds-card></body></html>`
			result, err := sr.RenderPage(html)
			Expect(err).NotTo(HaveOccurred(),
				"RenderPage must succeed on the restarted process")
			Expect(result).To(ContainSubstring("after restart"))
			Expect(sr.Close()).To(Succeed())
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
