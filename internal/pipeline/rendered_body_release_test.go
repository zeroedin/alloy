package pipeline_test

import (
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/zeroedin/alloy/internal/config"
	"github.com/zeroedin/alloy/internal/pipeline"
)

var _ = Describe("Release page.RenderedBody after disk write (issue #1107)", func() {

	// Direction 3 from #1098. After Directions 1+2 land (#1104), the
	// remaining memory bottleneck is page.RenderedBody — the []byte
	// that holds each page's rendered HTML from render through output
	// writing. Currently all pages' RenderedBody slices live
	// simultaneously in heap, so peak memory is O(total site HTML).
	// Releasing each page's RenderedBody immediately after its output
	// write converts peak to O(largest single page).
	//
	// The pipeline must:
	// 1. Build the CaptureRenderedContent map (if enabled) BEFORE the
	//    output writing loop — currently it runs after output writing,
	//    but release makes that position read nil RenderedBody.
	// 2. After writing each page's output (primary HTML, FormatBodies,
	//    aliases), call page.ReleaseRenderedBody() to nil RenderedBody,
	//    clear renderedStr, and nil FormatBodies.
	// 3. Apply the same pattern in BuildIncremental().

	Describe("Build() — CaptureRenderedContent with release", func() {

		It("captured RenderedContent matches disk output for every page", func() {
			tmpDir := GinkgoT().TempDir()
			contentDir := filepath.Join(tmpDir, "content")
			blogDir := filepath.Join(contentDir, "blog")
			layoutsDir := filepath.Join(tmpDir, "layouts")
			outputDir := filepath.Join(tmpDir, "_site")

			Expect(os.MkdirAll(blogDir, 0755)).To(Succeed())
			Expect(os.MkdirAll(layoutsDir, 0755)).To(Succeed())

			Expect(os.WriteFile(filepath.Join(contentDir, "index.md"),
				[]byte("---\ntitle: Home\nlayout: default\n---\n# Home Page"),
				0644)).To(Succeed())
			Expect(os.WriteFile(filepath.Join(contentDir, "about.md"),
				[]byte("---\ntitle: About\nlayout: default\n---\n# About Us"),
				0644)).To(Succeed())
			Expect(os.WriteFile(filepath.Join(blogDir, "post.md"),
				[]byte("---\ntitle: First Post\nlayout: default\n---\n# Hello World from Blog"),
				0644)).To(Succeed())

			Expect(os.WriteFile(filepath.Join(layoutsDir, "default.liquid"),
				[]byte("<html><body>{{ content }}</body></html>"),
				0644)).To(Succeed())

			cfg := &config.Config{
				Title:       "Capture Matches Disk",
				BaseURL:     "https://example.com",
				ProjectRoot: tmpDir,
				Build:       config.BuildConfig{Output: outputDir},
				Structure: config.StructureConfig{
					Content: "content",
					Layouts: "layouts",
				},
			}
			config.ApplyDefaults(cfg)

			result, err := pipeline.Build(cfg, pipeline.BuildOptions{
				CaptureRenderedContent: true,
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.RenderedContent).NotTo(BeNil(),
				"CaptureRenderedContent must populate the map even after "+
					"RenderedBody release is implemented — the map must be "+
					"built BEFORE the output write loop that releases pages")
			Expect(result.RenderedContent).To(HaveLen(3),
				"all 3 pages must appear in RenderedContent")

			// For each page, the captured HTML must match what was written
			// to disk byte-for-byte. This is the key disambiguation test:
			// if CaptureRenderedContent runs AFTER release, the map will
			// contain empty strings (page.HTML() returns "" after release).
			// If it runs BEFORE release, the captured HTML and disk content
			// will match exactly.
			pageMap := map[string]string{
				"index.md":     filepath.Join(outputDir, "index.html"),
				"about.md":     filepath.Join(outputDir, "about", "index.html"),
				"blog/post.md": filepath.Join(outputDir, "blog", "post", "index.html"),
			}

			for relPath, diskPath := range pageMap {
				captured, ok := result.RenderedContent[relPath]
				Expect(ok).To(BeTrue(),
					relPath+" must be present in RenderedContent")
				Expect(captured).NotTo(BeEmpty(),
					"captured HTML for "+relPath+" must not be empty — if it "+
						"is empty, CaptureRenderedContent is running after "+
						"ReleaseRenderedBody instead of before it (issue #1107)")

				diskContent, readErr := os.ReadFile(diskPath)
				Expect(readErr).NotTo(HaveOccurred(),
					diskPath+" must exist on disk")
				Expect(captured).To(Equal(string(diskContent)),
					"captured RenderedContent for "+relPath+" must match disk "+
						"output byte-for-byte — this proves the CaptureRenderedContent "+
						"map is built from the same data that was written to disk, "+
						"before ReleaseRenderedBody clears it (issue #1107)")
			}
		})

		It("produces correct disk output when CaptureRenderedContent is false", func() {
			tmpDir := GinkgoT().TempDir()
			contentDir := filepath.Join(tmpDir, "content")
			layoutsDir := filepath.Join(tmpDir, "layouts")
			outputDir := filepath.Join(tmpDir, "_site")

			Expect(os.MkdirAll(contentDir, 0755)).To(Succeed())
			Expect(os.MkdirAll(layoutsDir, 0755)).To(Succeed())

			Expect(os.WriteFile(filepath.Join(contentDir, "index.md"),
				[]byte("---\ntitle: Home\nlayout: default\n---\n# Production Mode Page"),
				0644)).To(Succeed())

			Expect(os.WriteFile(filepath.Join(layoutsDir, "default.liquid"),
				[]byte("<html><body>{{ content }}</body></html>"),
				0644)).To(Succeed())

			cfg := &config.Config{
				Title:       "Production Release Test",
				BaseURL:     "https://example.com",
				ProjectRoot: tmpDir,
				Build:       config.BuildConfig{Output: outputDir},
				Structure: config.StructureConfig{
					Content: "content",
					Layouts: "layouts",
				},
			}
			config.ApplyDefaults(cfg)

			// Production mode: CaptureRenderedContent is false (default)
			result, err := pipeline.Build(cfg)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.PageCount).To(Equal(1))
			Expect(result.RenderedContent).To(BeNil(),
				"production builds must not populate RenderedContent")

			// Output file must still be correct — the release must happen
			// AFTER the disk write, not before it
			indexPath := filepath.Join(outputDir, "index.html")
			Expect(indexPath).To(BeAnExistingFile())
			diskContent, readErr := os.ReadFile(indexPath)
			Expect(readErr).NotTo(HaveOccurred())
			Expect(string(diskContent)).To(ContainSubstring("Production Mode Page"),
				"rendered content must be written to disk before page "+
					"RenderedBody is released — if release happens before write, "+
					"the output file would be empty or missing (issue #1107)")
			Expect(string(diskContent)).To(ContainSubstring("<html>"),
				"layout wrapping must still be applied")
		})
	})

	Describe("Build() — alias output files survive release", func() {

		It("writes alias redirect files before releasing RenderedBody", func() {
			tmpDir := GinkgoT().TempDir()
			contentDir := filepath.Join(tmpDir, "content")
			layoutsDir := filepath.Join(tmpDir, "layouts")
			outputDir := filepath.Join(tmpDir, "_site")

			Expect(os.MkdirAll(contentDir, 0755)).To(Succeed())
			Expect(os.MkdirAll(layoutsDir, 0755)).To(Succeed())

			// Page with an alias — the alias output must be written from
			// page.RenderedBody before release clears it
			Expect(os.WriteFile(filepath.Join(contentDir, "new-page.md"),
				[]byte("---\ntitle: New Page\nlayout: default\naliases:\n  - /old-url/\n---\n# New Page Content"),
				0644)).To(Succeed())

			Expect(os.WriteFile(filepath.Join(layoutsDir, "default.liquid"),
				[]byte("<html><body>{{ content }}</body></html>"),
				0644)).To(Succeed())

			cfg := &config.Config{
				Title:       "Alias Release Test",
				BaseURL:     "https://example.com",
				ProjectRoot: tmpDir,
				Build:       config.BuildConfig{Output: outputDir},
				Structure: config.StructureConfig{
					Content: "content",
					Layouts: "layouts",
				},
			}
			config.ApplyDefaults(cfg)

			result, err := pipeline.Build(cfg)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.PageCount).To(Equal(1))

			// Primary output must exist
			primaryPath := filepath.Join(outputDir, "new-page", "index.html")
			Expect(primaryPath).To(BeAnExistingFile(),
				"primary output file must be written before release")

			primaryContent, readErr := os.ReadFile(primaryPath)
			Expect(readErr).NotTo(HaveOccurred())
			Expect(string(primaryContent)).To(ContainSubstring("New Page Content"),
				"primary output must contain the rendered page content — "+
					"if release happens before the primary write, the file "+
					"would be empty (issue #1107)")

			// Alias output must also exist with correct content — the alias
			// redirect file is written using page.RenderedBody, so it must
			// be written before release. BeAnExistingFile() alone is
			// insufficient: os.WriteFile with nil content creates a 0-byte
			// file that passes existence checks.
			aliasPath := filepath.Join(outputDir, "old-url", "index.html")
			Expect(aliasPath).To(BeAnExistingFile(),
				"alias redirect file must be written before ReleaseRenderedBody "+
					"is called — aliases use page.RenderedBody as the redirect "+
					"content source. If release happens between primary write "+
					"and alias write, the alias file would be empty or missing "+
					"(issue #1107)")

			aliasContent, aliasReadErr := os.ReadFile(aliasPath)
			Expect(aliasReadErr).NotTo(HaveOccurred())
			Expect(string(aliasContent)).To(ContainSubstring("New Page Content"),
				"alias file must contain the rendered page content — "+
					"WriteAliasesCached writes page.RenderedBody to each alias "+
					"path. If RenderedBody is nil (premature release), the alias "+
					"file would be 0 bytes (issue #1107)")
			Expect(string(aliasContent)).To(Equal(string(primaryContent)),
				"alias content must be identical to primary output — both are "+
					"written from the same page.RenderedBody before release")
		})
	})

	Describe("Build() — FormatBodies output files survive release", func() {

		It("writes alternate format output files before releasing RenderedBody", func() {
			tmpDir := GinkgoT().TempDir()
			contentDir := filepath.Join(tmpDir, "content")
			layoutsDir := filepath.Join(tmpDir, "layouts")
			outputDir := filepath.Join(tmpDir, "_site")

			Expect(os.MkdirAll(contentDir, 0755)).To(Succeed())
			Expect(os.MkdirAll(layoutsDir, 0755)).To(Succeed())

			// Page declares both html and json output formats
			Expect(os.WriteFile(filepath.Join(contentDir, "index.md"),
				[]byte("---\ntitle: Multi-Format\nlayout: default\noutputs:\n  - html\n  - json\n---\n# Multi-Format Content"),
				0644)).To(Succeed())

			// HTML layout
			Expect(os.WriteFile(filepath.Join(layoutsDir, "default.liquid"),
				[]byte("<html><body>{{ content }}</body></html>"),
				0644)).To(Succeed())

			// JSON format layout — outputs page data as JSON
			Expect(os.WriteFile(filepath.Join(layoutsDir, "default.json.liquid"),
				[]byte(`{"title":"{{ page.title }}","content":"{{ content | strip_html | strip_newlines }}"}`),
				0644)).To(Succeed())

			cfg := &config.Config{
				Title:       "FormatBodies Release Test",
				BaseURL:     "https://example.com",
				ProjectRoot: tmpDir,
				Build:       config.BuildConfig{Output: outputDir},
				Structure: config.StructureConfig{
					Content: "content",
					Layouts: "layouts",
				},
			}
			config.ApplyDefaults(cfg)

			result, err := pipeline.Build(cfg)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.PageCount).To(Equal(1))

			// Primary HTML output must exist with correct content
			htmlPath := filepath.Join(outputDir, "index.html")
			Expect(htmlPath).To(BeAnExistingFile())
			htmlContent, readErr := os.ReadFile(htmlPath)
			Expect(readErr).NotTo(HaveOccurred())
			Expect(string(htmlContent)).To(ContainSubstring("Multi-Format Content"),
				"primary HTML output must be written before release")

			// JSON format output must also exist with correct content.
			// FormatBodies are written in the same loop as primary output
			// (build.go output writing stage). If the developer inserts
			// ReleaseRenderedBody() after the primary HTML write but before
			// the FormatBodies loop, the JSON file would be empty or missing.
			jsonPath := filepath.Join(outputDir, "index.json")
			Expect(jsonPath).To(BeAnExistingFile(),
				"JSON format output file must be written before "+
					"ReleaseRenderedBody — FormatBodies are written from "+
					"page.FormatBodies which is niled by release (issue #1107)")

			jsonContent, jsonReadErr := os.ReadFile(jsonPath)
			Expect(jsonReadErr).NotTo(HaveOccurred())
			Expect(string(jsonContent)).NotTo(BeEmpty(),
				"JSON format output file must not be empty — if "+
					"ReleaseRenderedBody runs before FormatBodies are written, "+
					"the map is nil and WriteFileCached receives nil content")
			Expect(string(jsonContent)).To(ContainSubstring("Multi-Format"),
				"JSON format output must contain the page title — "+
					"this proves FormatBodies were written to disk before "+
					"page.ReleaseRenderedBody() niled them (issue #1107)")
		})
	})

	Describe("Build() — sitemap still works after release", func() {

		It("generates correct sitemap from page metadata after RenderedBody release", func() {
			tmpDir := GinkgoT().TempDir()
			contentDir := filepath.Join(tmpDir, "content")
			layoutsDir := filepath.Join(tmpDir, "layouts")
			outputDir := filepath.Join(tmpDir, "_site")

			Expect(os.MkdirAll(contentDir, 0755)).To(Succeed())
			Expect(os.MkdirAll(layoutsDir, 0755)).To(Succeed())

			Expect(os.WriteFile(filepath.Join(contentDir, "index.md"),
				[]byte("---\ntitle: Home\nlayout: default\n---\n# Home"),
				0644)).To(Succeed())
			Expect(os.WriteFile(filepath.Join(contentDir, "about.md"),
				[]byte("---\ntitle: About\nlayout: default\n---\n# About"),
				0644)).To(Succeed())

			Expect(os.WriteFile(filepath.Join(layoutsDir, "default.liquid"),
				[]byte("<html><body>{{ content }}</body></html>"),
				0644)).To(Succeed())

			cfg := &config.Config{
				Title:       "Sitemap After Release",
				BaseURL:     "https://example.com",
				ProjectRoot: tmpDir,
				Build:       config.BuildConfig{Output: outputDir},
				Structure: config.StructureConfig{
					Content: "content",
					Layouts: "layouts",
				},
				Sitemap: config.SitemapConfig{Enabled: true},
			}
			config.ApplyDefaults(cfg)

			result, err := pipeline.Build(cfg)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.PageCount).To(Equal(2))

			// Sitemap generation runs after output writing and uses page
			// metadata (URL, date) not RenderedBody. It must still work
			// correctly after RenderedBody has been released.
			sitemapPath := filepath.Join(outputDir, "sitemap.xml")
			Expect(sitemapPath).To(BeAnExistingFile(),
				"sitemap.xml must be generated after output writing — "+
					"sitemap generation reads page.URL not page.RenderedBody, "+
					"so it must work even after RenderedBody is released (issue #1107)")

			sitemapContent, readErr := os.ReadFile(sitemapPath)
			Expect(readErr).NotTo(HaveOccurred())
			Expect(string(sitemapContent)).To(ContainSubstring("https://example.com"),
				"sitemap must contain the base URL")
			Expect(string(sitemapContent)).To(ContainSubstring("<loc>"),
				"sitemap must contain page location entries")
		})
	})

	Describe("Build() — cache tracking works after release", func() {

		It("builds cache with content hashes after RenderedBody is released", func() {
			tmpDir := GinkgoT().TempDir()
			contentDir := filepath.Join(tmpDir, "content")
			layoutsDir := filepath.Join(tmpDir, "layouts")
			outputDir := filepath.Join(tmpDir, "_site")

			Expect(os.MkdirAll(contentDir, 0755)).To(Succeed())
			Expect(os.MkdirAll(layoutsDir, 0755)).To(Succeed())

			Expect(os.WriteFile(filepath.Join(contentDir, "index.md"),
				[]byte("---\ntitle: Home\nlayout: default\n---\n# Cache Test"),
				0644)).To(Succeed())

			Expect(os.WriteFile(filepath.Join(layoutsDir, "default.liquid"),
				[]byte("<html><body>{{ content }}</body></html>"),
				0644)).To(Succeed())

			cfg := &config.Config{
				Title:       "Cache After Release",
				BaseURL:     "https://example.com",
				ProjectRoot: tmpDir,
				Build:       config.BuildConfig{Output: outputDir},
				Structure: config.StructureConfig{
					Content: "content",
					Layouts: "layouts",
				},
			}
			config.ApplyDefaults(cfg)

			result, err := pipeline.Build(cfg)
			Expect(err).NotTo(HaveOccurred())

			// Cache is built from page.Content (raw source), not
			// page.RenderedBody. It must still be populated after release.
			Expect(result.Cache).NotTo(BeNil(),
				"build cache must be populated after RenderedBody release — "+
					"the cache uses page.Content (raw source bytes) for content "+
					"hashing, not page.RenderedBody, so release must not affect "+
					"cache correctness (issue #1107)")

			// Verify the cache actually recorded a hash for the page's
			// source content — NotTo(BeNil()) alone is trivially satisfied
			// because cache.New() returns a non-nil *Cache with empty maps.
			contentBytes, readErr := os.ReadFile(filepath.Join(contentDir, "index.md"))
			Expect(readErr).NotTo(HaveOccurred())
			Expect(result.Cache.ShouldSkipFile("index.md", contentBytes)).To(BeTrue(),
				"cache must have recorded the content hash for index.md — "+
					"ShouldSkipFile returns true when the stored hash matches "+
					"the current content. This proves the cache was populated "+
					"from page.Content (not page.RenderedBody) and is unaffected "+
					"by RenderedBody release (issue #1107)")
		})
	})

	Describe("BuildIncremental() — CaptureRenderedContent with release", func() {

		It("captured RenderedContent matches disk output after incremental build", func() {
			tmpDir := GinkgoT().TempDir()
			contentDir := filepath.Join(tmpDir, "content")
			layoutsDir := filepath.Join(tmpDir, "layouts")
			outputDir := filepath.Join(tmpDir, "_site")

			Expect(os.MkdirAll(contentDir, 0755)).To(Succeed())
			Expect(os.MkdirAll(layoutsDir, 0755)).To(Succeed())

			Expect(os.WriteFile(filepath.Join(contentDir, "index.md"),
				[]byte("---\ntitle: Home\nlayout: default\n---\n# Incremental Release Test"),
				0644)).To(Succeed())
			Expect(os.WriteFile(filepath.Join(contentDir, "about.md"),
				[]byte("---\ntitle: About\nlayout: default\n---\n# About Incremental"),
				0644)).To(Succeed())

			Expect(os.WriteFile(filepath.Join(layoutsDir, "default.liquid"),
				[]byte("<html><body>{{ content }}</body></html>"),
				0644)).To(Succeed())

			cfg := &config.Config{
				Title:       "Incremental Capture Match",
				BaseURL:     "https://example.com",
				ProjectRoot: tmpDir,
				Build:       config.BuildConfig{Output: outputDir},
				Structure: config.StructureConfig{
					Content: "content",
					Layouts: "layouts",
				},
			}
			config.ApplyDefaults(cfg)
			registry, hooks, _ := pipeline.DiscoverPlugins(cfg)
			defer registry.Close()
			pipelineState, psErr := pipeline.InitPipelineState(cfg, registry, hooks)
			Expect(psErr).NotTo(HaveOccurred())

			result, err := pipeline.BuildIncremental(cfg, nil, nil, nil,
				pipeline.BuildOptions{
					PipelineState:          pipelineState,
					CaptureRenderedContent: true,
				})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.RenderedContent).NotTo(BeNil(),
				"CaptureRenderedContent must populate the map in "+
					"BuildIncremental even after release is implemented")

			// Verify captured content matches disk for each page
			pageMap := map[string]string{
				"index.md": filepath.Join(outputDir, "index.html"),
				"about.md": filepath.Join(outputDir, "about", "index.html"),
			}

			for relPath, diskPath := range pageMap {
				captured, ok := result.RenderedContent[relPath]
				Expect(ok).To(BeTrue(),
					relPath+" must be present in RenderedContent")
				Expect(captured).NotTo(BeEmpty(),
					"captured HTML for "+relPath+" must not be empty — "+
						"if capture runs after release, the map would contain "+
						"empty strings (issue #1107)")

				diskContent, readErr := os.ReadFile(diskPath)
				Expect(readErr).NotTo(HaveOccurred())
				Expect(captured).To(Equal(string(diskContent)),
					"BuildIncremental captured RenderedContent for "+relPath+
						" must match disk output — same capture-before-release "+
						"ordering as Build() (issue #1107)")
			}
		})

		It("produces correct disk output without CaptureRenderedContent", func() {
			tmpDir := GinkgoT().TempDir()
			contentDir := filepath.Join(tmpDir, "content")
			layoutsDir := filepath.Join(tmpDir, "layouts")
			outputDir := filepath.Join(tmpDir, "_site")

			Expect(os.MkdirAll(contentDir, 0755)).To(Succeed())
			Expect(os.MkdirAll(layoutsDir, 0755)).To(Succeed())

			Expect(os.WriteFile(filepath.Join(contentDir, "index.md"),
				[]byte("---\ntitle: Home\nlayout: default\n---\n# Incremental Production"),
				0644)).To(Succeed())

			Expect(os.WriteFile(filepath.Join(layoutsDir, "default.liquid"),
				[]byte("<html><body>{{ content }}</body></html>"),
				0644)).To(Succeed())

			cfg := &config.Config{
				Title:       "Incremental Production Release",
				BaseURL:     "https://example.com",
				ProjectRoot: tmpDir,
				Build:       config.BuildConfig{Output: outputDir},
				Structure: config.StructureConfig{
					Content: "content",
					Layouts: "layouts",
				},
			}
			config.ApplyDefaults(cfg)
			registry, hooks, _ := pipeline.DiscoverPlugins(cfg)
			defer registry.Close()
			pipelineState, psErr := pipeline.InitPipelineState(cfg, registry, hooks)
			Expect(psErr).NotTo(HaveOccurred())

			result, err := pipeline.BuildIncremental(cfg, nil, nil, nil,
				pipeline.BuildOptions{PipelineState: pipelineState})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.RenderedContent).To(BeNil())

			indexPath := filepath.Join(outputDir, "index.html")
			Expect(indexPath).To(BeAnExistingFile())
			diskContent, readErr := os.ReadFile(indexPath)
			Expect(readErr).NotTo(HaveOccurred())
			Expect(string(diskContent)).To(ContainSubstring("Incremental Production"),
				"BuildIncremental must write correct output to disk before "+
					"releasing RenderedBody (issue #1107)")
		})
	})

	Describe("BuildWithContent() — backward compatibility after release", func() {

		It("still populates RenderedContent for all pages", func() {
			cfg := &config.Config{
				Title:   "BuildWithContent Release Compat",
				BaseURL: "https://example.com",
				Build:   config.BuildConfig{Output: "_site"},
			}
			contentMap := map[string]string{
				"content/index.md":       "---\ntitle: Home\nlayout: default\n---\n# BWC Home",
				"content/about.md":       "---\ntitle: About\nlayout: default\n---\n# BWC About",
				"layouts/default.liquid": "<html><body>{{ content }}</body></html>",
			}

			result, err := pipeline.BuildWithContent(cfg, contentMap)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.RenderedContent).NotTo(BeNil(),
				"BuildWithContent must still populate RenderedContent after "+
					"the RenderedBody release optimization — BuildWithContent "+
					"forces CaptureRenderedContent: true, so the map must be "+
					"built before release clears page data (issue #1107)")
			Expect(result.RenderedContent).To(HaveLen(2))

			Expect(result.RenderedContent["index.md"]).To(ContainSubstring("BWC Home"),
				"captured HTML must contain actual rendered content, not empty "+
					"strings — proves capture-before-release ordering is correct")
			Expect(result.RenderedContent["about.md"]).To(ContainSubstring("BWC About"))
		})
	})

	Describe("Build() — SSR Phase 2 runs before release", func() {

		It("SSR-transformed HTML is written to disk and captured correctly", func() {
			tmpDir := GinkgoT().TempDir()
			contentDir := filepath.Join(tmpDir, "content")
			layoutsDir := filepath.Join(tmpDir, "layouts")
			outputDir := filepath.Join(tmpDir, "_site")

			Expect(os.MkdirAll(contentDir, 0755)).To(Succeed())
			Expect(os.MkdirAll(layoutsDir, 0755)).To(Succeed())

			// Content with a custom element to trigger SSR
			Expect(os.WriteFile(filepath.Join(contentDir, "index.md"),
				[]byte("---\ntitle: Home\nlayout: default\n---\n# SSR Test\n<my-component>SSR content</my-component>"),
				0644)).To(Succeed())

			Expect(os.WriteFile(filepath.Join(layoutsDir, "default.liquid"),
				[]byte("<html><body>{{ content }}</body></html>"),
				0644)).To(Succeed())

			cfg := &config.Config{
				Title:       "SSR Before Release",
				BaseURL:     "https://example.com",
				ProjectRoot: tmpDir,
				Build:       config.BuildConfig{Output: outputDir},
				Structure: config.StructureConfig{
					Content: "content",
					Layouts: "layouts",
				},
				// SSR enabled with "cat" — passes HTML through unchanged
				SSR: &config.SSRConfig{Command: "cat"},
			}
			config.ApplyDefaults(cfg)

			result, err := pipeline.Build(cfg, pipeline.BuildOptions{
				CaptureRenderedContent: true,
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.SSRSkipped).To(BeFalse(),
				"SSR Phase 2 must run before output writing and release")

			// Verify disk output is the SSR-transformed version
			indexPath := filepath.Join(outputDir, "index.html")
			Expect(indexPath).To(BeAnExistingFile())
			diskContent, readErr := os.ReadFile(indexPath)
			Expect(readErr).NotTo(HaveOccurred())
			Expect(string(diskContent)).To(ContainSubstring("my-component"),
				"SSR-transformed content must be on disk — SSR runs before "+
					"output writing, and output writing runs before release. "+
					"The pipeline order is: SSR → capture → write → release "+
					"(issue #1107)")

			// Captured content must also be the SSR-transformed version
			captured, ok := result.RenderedContent["index.md"]
			Expect(ok).To(BeTrue())
			Expect(captured).To(Equal(string(diskContent)),
				"captured RenderedContent must match the SSR-transformed disk "+
					"content — both must reflect the post-SSR HTML, not the "+
					"pre-SSR version (issue #1107)")
		})
	})

})
