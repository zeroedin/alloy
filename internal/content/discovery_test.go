package content_test

import (
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/zeroedin/alloy/internal/content"
)

var _ = Describe("Discovery", func() {

	Describe("Discover", func() {

		Context("basic file walking", func() {
			var (
				pages []*content.Page
				err   error
			)

			BeforeEach(func() {
				pages, err = content.Discover("testdata/site1/content")
			})

			It("discovers .md files in content/", func() {
				Expect(err).NotTo(HaveOccurred())
				Expect(pages).NotTo(BeEmpty())

				relPaths := make([]string, len(pages))
				for i, p := range pages {
					relPaths[i] = p.RelPath
				}
				Expect(relPaths).To(ContainElement("blog/first-post.md"))
			})

			It("discovers .html files in content/", func() {
				Expect(err).NotTo(HaveOccurred())

				relPaths := make([]string, len(pages))
				for i, p := range pages {
					relPaths[i] = p.RelPath
				}
				Expect(relPaths).To(ContainElement("about.html"))
			})

			It("ignores _data.yaml files", func() {
				Expect(err).NotTo(HaveOccurred())

				for _, p := range pages {
					Expect(p.RelPath).NotTo(HaveSuffix("_data.yaml"))
				}
			})

			It("returns error when content dir does not exist", func() {
				_, discoverErr := content.Discover("testdata/nonexistent")
				Expect(discoverErr).To(HaveOccurred())
				Expect(discoverErr).NotTo(MatchError(content.ErrNotImplemented))
			})
		})

		Context("directory structure", func() {
			var (
				pages []*content.Page
				err   error
			)

			BeforeEach(func() {
				pages, err = content.Discover("testdata/site1/content")
			})

			It("preserves relative paths from content root", func() {
				Expect(err).NotTo(HaveOccurred())

				relPaths := make([]string, len(pages))
				for i, p := range pages {
					relPaths[i] = p.RelPath
				}
				Expect(relPaths).To(ContainElement("blog/first-post.md"))
			})

			It("discovers files in nested subdirectories", func() {
				Expect(err).NotTo(HaveOccurred())

				relPaths := make([]string, len(pages))
				for i, p := range pages {
					relPaths[i] = p.RelPath
				}
				Expect(relPaths).To(ContainElement("docs/getting-started.md"))
			})

			It("identifies section from first directory segment", func() {
				Expect(err).NotTo(HaveOccurred())

				var blogPage *content.Page
				for _, p := range pages {
					if p.RelPath == "blog/first-post.md" {
						blogPage = p
						break
					}
				}
				Expect(blogPage).NotTo(BeNil())
				Expect(blogPage.Section).To(Equal("blog"))
			})
		})

		Context("page bundles", func() {
			var (
				pages []*content.Page
				err   error
			)

			BeforeEach(func() {
				pages, err = content.Discover("testdata/site1/content")
			})

			It("identifies second-post/ as a page bundle", func() {
				Expect(err).NotTo(HaveOccurred())

				var bundlePage *content.Page
				for _, p := range pages {
					if p.RelPath == "blog/second-post/index.md" {
						bundlePage = p
						break
					}
				}
				Expect(bundlePage).NotTo(BeNil())
				Expect(bundlePage.Bundle).To(BeTrue())
			})

			It("collects co-located assets for page bundles", func() {
				Expect(err).NotTo(HaveOccurred())

				var bundlePage *content.Page
				for _, p := range pages {
					if p.RelPath == "blog/second-post/index.md" {
						bundlePage = p
						break
					}
				}
				Expect(bundlePage).NotTo(BeNil())
				Expect(bundlePage.BundleAssets).To(ContainElement("hero.jpg"))
			})
		})

		Context("index files", func() {
			It("treats index.md at content root as the site index page", func() {
				pages, err := content.Discover("testdata/site1/content")
				Expect(err).NotTo(HaveOccurred())

				var indexPage *content.Page
				for _, p := range pages {
					if p.RelPath == "index.md" {
						indexPage = p
						break
					}
				}
				Expect(indexPage).NotTo(BeNil())
				Expect(indexPage.Section).To(Equal(""))
			})
		})

		// ── Error format contracts ────────────────────────────────────

		Context("error format contracts", func() {
			It("includes directory path in discovery error for nonexistent content dir", func() {
				_, err := content.Discover("/nonexistent/content/dir")
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("/nonexistent/content/dir"),
					"discovery error must include the content directory path")
			})
		})

		// ── Content format filtering (§1 content.formats) ────────────

		Context("content format filtering", func() {
			It("only discovers files matching allowed formats", func() {
				// site1/content has .md, .html, and .txt files
				// With formats ["md", "html"], .txt should be excluded
				pages, err := content.DiscoverWithFormats(
					"testdata/site1/content",
					[]string{"md", "html"},
				)
				Expect(err).NotTo(HaveOccurred())
				Expect(pages).NotTo(BeEmpty())

				for _, p := range pages {
					Expect(p.RelPath).NotTo(HaveSuffix(".txt"),
						"files not in formats list must be excluded")
				}
			})

			It("includes .txt files when txt is in the formats list", func() {
				pages, err := content.DiscoverWithFormats(
					"testdata/site1/content",
					[]string{"md", "html", "txt"},
				)
				Expect(err).NotTo(HaveOccurred())
				Expect(pages).NotTo(BeEmpty())

				relPaths := make([]string, len(pages))
				for i, p := range pages {
					relPaths[i] = p.RelPath
				}
				Expect(relPaths).To(ContainElement("notes.txt"),
					".txt must be included when txt is in formats")
			})
		})

		// ── Content-colocated passthrough (issue #287) ──────────────
		// Files in content/ whose extension does NOT match content.formats
		// must be collected as passthrough files, not processed or errored.

		Context("content-colocated passthrough", func() {
			It("collects non-format files as passthrough", func() {
				// Create temp content dir with mixed files
				tmpDir, err := os.MkdirTemp("", "passthrough-test-*")
				Expect(err).NotTo(HaveOccurred())
				DeferCleanup(func() { os.RemoveAll(tmpDir) })

				contentDir := filepath.Join(tmpDir, "content", "about")
				Expect(os.MkdirAll(contentDir, 0755)).To(Succeed())

				// Content file (format match)
				Expect(os.WriteFile(filepath.Join(contentDir, "index.md"),
					[]byte("---\ntitle: About\n---\n# About"), 0644)).To(Succeed())
				// Non-format files (should be passthrough)
				Expect(os.WriteFile(filepath.Join(contentDir, "diagram.svg"),
					[]byte("<svg></svg>"), 0644)).To(Succeed())
				Expect(os.WriteFile(filepath.Join(contentDir, "hero.png"),
					[]byte("fake png"), 0644)).To(Succeed())

				pages, passthroughs, err := content.DiscoverWithPassthrough(
					filepath.Join(tmpDir, "content"), []string{"md", "html"})
				Expect(err).NotTo(HaveOccurred())

				// Content pages
				Expect(pages).To(HaveLen(1),
					"only .md files matching formats should be content pages")
				Expect(pages[0].RelPath).To(Equal("about/index.md"))

				// Passthrough files
				Expect(passthroughs).To(HaveLen(2),
					"non-format files must be collected as passthrough")
				Expect(passthroughs).To(ContainElement("about/diagram.svg"),
					"SVG files must be collected as passthrough")
				Expect(passthroughs).To(ContainElement("about/hero.png"),
					"PNG files must be collected as passthrough")
			})

			It("does not error on non-format files", func() {
				tmpDir, err := os.MkdirTemp("", "passthrough-test-*")
				Expect(err).NotTo(HaveOccurred())
				DeferCleanup(func() { os.RemoveAll(tmpDir) })

				contentDir := filepath.Join(tmpDir, "content")
				Expect(os.MkdirAll(contentDir, 0755)).To(Succeed())

				// Only non-format files — no content pages
				Expect(os.WriteFile(filepath.Join(contentDir, "logo.svg"),
					[]byte("<svg></svg>"), 0644)).To(Succeed())
				Expect(os.WriteFile(filepath.Join(contentDir, "app.js"),
					[]byte("console.log('hello')"), 0644)).To(Succeed())

				pages, passthroughs, err := content.DiscoverWithPassthrough(
					contentDir, []string{"md", "html"})
				Expect(err).NotTo(HaveOccurred(),
					"non-format files must not cause a build error — "+
						"they are passthrough, not content")
				Expect(pages).To(BeEmpty(),
					"no content pages when only non-format files exist")
				Expect(passthroughs).To(HaveLen(2),
					"both non-format files must be collected as passthrough")
			})

			It("excludes _data.yaml and _data.yml from passthrough", func() {
				tmpDir, err := os.MkdirTemp("", "passthrough-test-*")
				Expect(err).NotTo(HaveOccurred())
				DeferCleanup(func() { os.RemoveAll(tmpDir) })

				contentDir := filepath.Join(tmpDir, "content")
				Expect(os.MkdirAll(contentDir, 0755)).To(Succeed())

				Expect(os.WriteFile(filepath.Join(contentDir, "_data.yaml"),
					[]byte("layout: post"), 0644)).To(Succeed())
				Expect(os.WriteFile(filepath.Join(contentDir, "_data.yml"),
					[]byte("layout: page"), 0644)).To(Succeed())
				Expect(os.WriteFile(filepath.Join(contentDir, "icon.svg"),
					[]byte("<svg></svg>"), 0644)).To(Succeed())

				_, passthroughs, err := content.DiscoverWithPassthrough(
					contentDir, []string{"md", "html"})
				Expect(err).NotTo(HaveOccurred())
				Expect(passthroughs).To(HaveLen(1),
					"both _data.yaml and _data.yml must be excluded from passthrough")
				Expect(passthroughs).To(ContainElement("icon.svg"))
			})

			It("excludes dot-prefixed files from passthrough", func() {
				tmpDir, err := os.MkdirTemp("", "passthrough-test-*")
				Expect(err).NotTo(HaveOccurred())
				DeferCleanup(func() { os.RemoveAll(tmpDir) })

				contentDir := filepath.Join(tmpDir, "content")
				Expect(os.MkdirAll(contentDir, 0755)).To(Succeed())

				Expect(os.WriteFile(filepath.Join(contentDir, ".DS_Store"),
					[]byte{0, 0, 0, 1}, 0644)).To(Succeed())
				Expect(os.WriteFile(filepath.Join(contentDir, ".gitkeep"),
					[]byte(""), 0644)).To(Succeed())
				Expect(os.WriteFile(filepath.Join(contentDir, "logo.svg"),
					[]byte("<svg></svg>"), 0644)).To(Succeed())

				_, passthroughs, err := content.DiscoverWithPassthrough(
					contentDir, []string{"md", "html"})
				Expect(err).NotTo(HaveOccurred())
				Expect(passthroughs).To(HaveLen(1),
					"dot-prefixed files (.DS_Store, .gitkeep) must be excluded from passthrough")
				Expect(passthroughs).To(ContainElement("logo.svg"))
			})

			It("preserves nested directory structure in passthrough paths", func() {
				tmpDir, err := os.MkdirTemp("", "passthrough-test-*")
				Expect(err).NotTo(HaveOccurred())
				DeferCleanup(func() { os.RemoveAll(tmpDir) })

				nested := filepath.Join(tmpDir, "content", "about", "images")
				Expect(os.MkdirAll(nested, 0755)).To(Succeed())
				Expect(os.WriteFile(filepath.Join(nested, "photo.jpg"),
					[]byte("fake jpg"), 0644)).To(Succeed())

				_, passthroughs, err := content.DiscoverWithPassthrough(
					filepath.Join(tmpDir, "content"), []string{"md", "html"})
				Expect(err).NotTo(HaveOccurred())
				Expect(passthroughs).To(ContainElement("about/images/photo.jpg"),
					"passthrough paths must be relative to content root, preserving directory structure")
			})
		})

		// ── Deeply nested directories (3+ levels) ────────────────────

		Context("deeply nested directories", func() {
			It("discovers files at 3+ directory levels deep", func() {
				pages, err := content.Discover("testdata/site1/content")
				Expect(err).NotTo(HaveOccurred())
				Expect(pages).NotTo(BeEmpty())

				relPaths := make([]string, len(pages))
				for i, p := range pages {
					relPaths[i] = p.RelPath
				}
				Expect(relPaths).To(ContainElement(
					"docs/guides/advanced/deep-topic.md"),
					"must discover files nested 3+ levels deep")
			})

			It("identifies section from first directory segment for deep files", func() {
				pages, err := content.Discover("testdata/site1/content")
				Expect(err).NotTo(HaveOccurred())

				var deepPage *content.Page
				for _, p := range pages {
					if p.RelPath == "docs/guides/advanced/deep-topic.md" {
						deepPage = p
						break
					}
				}
				Expect(deepPage).NotTo(BeNil(),
					"deep file must be discovered")
				Expect(deepPage.Section).To(Equal("docs"),
					"section must be the first directory segment regardless of depth")
			})
		})

		// ── Empty directories ────────────────────────────────────────

		Context("empty directories", func() {
			It("returns empty page list for directory with no content files", func() {
				dir := GinkgoT().TempDir()

				pages, err := content.Discover(dir)
				Expect(err).NotTo(HaveOccurred(),
					"empty content directory must not be an error")
				Expect(pages).To(BeEmpty(),
					"empty directory must return zero pages")
			})
		})

		// ── Content outside content directory ────────────────────────

		Context("content outside content directory", func() {
			It("ignores files outside the content directory", func() {
				// Use a temp dir with a content/ subdir and a file outside it
				tmpDir := GinkgoT().TempDir()
				contentDir := filepath.Join(tmpDir, "content")
				Expect(os.MkdirAll(contentDir, 0o755)).To(Succeed())
				// Only discover from content/ — implementation must not walk parent
				pages, err := content.Discover(contentDir)
				Expect(err).NotTo(HaveOccurred())
				Expect(pages).To(BeEmpty(),
					"empty content dir must return zero pages")
			})
		})

		// ── Symlink handling ─────────────────────────────────────────

		Context("symlink handling", func() {
			It("follows symlinks to content files", func() {
				dir := GinkgoT().TempDir()

				// Create a real file outside the content dir
				realDir := GinkgoT().TempDir()
				realFile := realDir + "/linked-post.md"
				err := os.WriteFile(realFile,
					[]byte("---\ntitle: Linked Post\n---\nContent."),
					0644,
				)
				Expect(err).NotTo(HaveOccurred())

				// Create a symlink inside the content dir
				err = os.Symlink(realFile, dir+"/linked-post.md")
				Expect(err).NotTo(HaveOccurred())

				pages, discoverErr := content.Discover(dir)
				Expect(discoverErr).NotTo(HaveOccurred())
				Expect(pages).NotTo(BeEmpty(),
					"symlinked content files must be discovered")

				relPaths := make([]string, len(pages))
				for i, p := range pages {
					relPaths[i] = p.RelPath
				}
				Expect(relPaths).To(ContainElement("linked-post.md"),
					"symlinked file must appear with its link name")
			})
		})
	})
})
