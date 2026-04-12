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
			instances := ssr.ScanComponents(html)
			Expect(instances).To(HaveLen(2))
		})

		It("extracts tag name and attributes", func() {
			html := `<ds-card variant="primary" size="lg"></ds-card>`
			instances := ssr.ScanComponents(html)
			Expect(instances).To(HaveLen(1))
			Expect(instances[0].Tag).To(Equal("ds-card"))
			Expect(instances[0].Attrs).To(HaveKeyWithValue("variant", "primary"))
			Expect(instances[0].Attrs).To(HaveKeyWithValue("size", "lg"))
		})

		It("ignores standard HTML elements", func() {
			// First verify that custom elements ARE detected (proves the function works)
			customHTML := `<div><ds-card></ds-card></div>`
			customInstances := ssr.ScanComponents(customHTML)
			Expect(customInstances).To(HaveLen(1), "custom elements must be detected")

			// Then verify standard elements are ignored
			html := `<div><span>text</span><p>paragraph</p></div>`
			instances := ssr.ScanComponents(html)
			Expect(instances).To(BeEmpty())
		})
	})

	// ── Deduplication ──────────────────────────────────────────────────

	Describe("Deduplication", func() {
		It("same tag+attrs with different slot content produces same hash", func() {
			instances := []ssr.ComponentInstance{
				{Tag: "ds-card", Attrs: map[string]string{"variant": "primary"}},
				{Tag: "ds-card", Attrs: map[string]string{"variant": "primary"}},
			}
			deduped := ssr.DeduplicateInstances(instances)
			Expect(deduped).To(HaveLen(1))
		})

		It("different attribute values produce different hashes", func() {
			instances := []ssr.ComponentInstance{
				{Tag: "ds-card", Attrs: map[string]string{"variant": "primary"}},
				{Tag: "ds-card", Attrs: map[string]string{"variant": "secondary"}},
			}
			deduped := ssr.DeduplicateInstances(instances)
			Expect(deduped).To(HaveLen(2))
		})
	})

	// ── Marker insertion ───────────────────────────────────────────────

	Describe("Marker insertion", func() {
		It("wraps component instances with alloy-ssr comment markers", func() {
			html := `<ds-card variant="primary"></ds-card>`
			instances := []ssr.ComponentInstance{
				{Tag: "ds-card", Attrs: map[string]string{"variant": "primary"}, Hash: "abc123"},
			}
			result := ssr.InsertMarkers(html, instances)
			Expect(result).To(ContainSubstring("<!--alloy-ssr:abc123-->"))
		})
	})

	// ── Stamp-back ─────────────────────────────────────────────────────

	Describe("Stamp-back", func() {
		It("inserts template shadowrootmode inside component tag", func() {
			html := `<!--alloy-ssr:abc123--><ds-card></ds-card><!--/alloy-ssr:abc123-->`
			ssrResults := map[string]string{
				"abc123": `<template shadowrootmode="open"><style>:host{display:block}</style><slot></slot></template>`,
			}
			result := ssr.StampBack(html, ssrResults)
			Expect(result).To(ContainSubstring(`<template shadowrootmode="open"`))
		})

		It("preserves light DOM content (slots)", func() {
			html := `<!--alloy-ssr:def456--><ds-card><p>Card content</p></ds-card><!--/alloy-ssr:def456-->`
			ssrResults := map[string]string{
				"def456": `<template shadowrootmode="open"><slot></slot></template>`,
			}
			result := ssr.StampBack(html, ssrResults)
			Expect(result).To(ContainSubstring("<p>Card content</p>"))
		})
	})

	// ── SSR config parsing ────────────────────────────────────────────

	Describe("SSR config parsing", func() {
		It("parses ssr.build from config map", func() {
			raw := map[string]interface{}{
				"build": "golit render _site/**/*.html",
				"serve": map[string]interface{}{
					"cmd":      "golit serve",
					"endpoint": "http://localhost:6274",
				},
			}
			cfg, err := ssr.ParseSSRConfig(raw)
			Expect(err).NotTo(HaveOccurred())
			Expect(cfg).NotTo(BeNil())
			Expect(cfg.BuildCmd).To(Equal("golit render _site/**/*.html"))
			Expect(cfg.ServeEndpoint).To(Equal("http://localhost:6274"))
		})
	})

	// ── Protocol integration ──────────────────────────────────────────

	Describe("Protocol integration", func() {
		It("RenderViaHTTP POSTs HTML and receives SSR'd HTML", func() {
			result, err := ssr.RenderViaHTTP("http://localhost:6274/render", "<ds-card></ds-card>")
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(ContainSubstring("shadowrootmode"),
				"SSR response must contain DSD template")
		})

		It("RenderViaStdio sends NUL-terminated HTML over stdin", func() {
			result, err := ssr.RenderViaStdio("golit render", "<ds-card></ds-card>")
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeEmpty(),
				"stdio SSR must return rendered HTML")
		})
	})

	// ── Cache key computation ─────────────────────────────────────────

	Describe("Cache key computation", func() {
		It("ComponentCacheKey includes tag, attributes, and definition hash", func() {
			instance := ssr.ComponentInstance{
				Tag:   "ds-card",
				Attrs: map[string]string{"variant": "primary"},
			}
			key := ssr.ComponentCacheKey(instance, "def-hash-abc")
			Expect(key).NotTo(BeEmpty(),
				"cache key must not be empty")
			// Different definition hash must produce different key
			key2 := ssr.ComponentCacheKey(instance, "def-hash-xyz")
			Expect(key).NotTo(Equal(key2),
				"different definition hashes must produce different cache keys")
		})

		It("HashOutput produces deterministic hash for same content", func() {
			hash1 := ssr.HashOutput("<html>content</html>")
			hash2 := ssr.HashOutput("<html>content</html>")
			Expect(hash1).NotTo(BeEmpty())
			Expect(hash1).To(Equal(hash2),
				"same content must produce same hash")
		})
	})

	// ── Component map persistence (§6) ──────────────────────────────

	Describe("Component map persistence", func() {
		It("saves instances map to .alloy/components.json", func() {
			tmpDir := GinkgoT().TempDir()
			cm := ssr.NewComponentMap()
			cm.Instances["hash1"] = ssr.ComponentInstance{Tag: "ds-card", Attrs: map[string]string{"title": "Hi"}, Hash: "hash1"}

			err := cm.SaveTo(tmpDir)
			Expect(err).NotTo(HaveOccurred())

			_, err = os.Stat(filepath.Join(tmpDir, "components.json"))
			Expect(err).NotTo(HaveOccurred(),
				"components.json must be written to the target directory")
		})

		It("saves and loads pageToInstances map", func() {
			tmpDir := GinkgoT().TempDir()
			cm := ssr.NewComponentMap()
			cm.PageToInstances["content/index.md"] = []string{"hash1", "hash2"}

			err := cm.SaveTo(tmpDir)
			Expect(err).NotTo(HaveOccurred())

			loaded, err := ssr.LoadComponentMap(tmpDir)
			Expect(err).NotTo(HaveOccurred())
			Expect(loaded).NotTo(BeNil())
			Expect(loaded.PageToInstances).To(HaveKey("content/index.md"))
			Expect(loaded.PageToInstances["content/index.md"]).To(ConsistOf("hash1", "hash2"))
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
