package output_test

import (
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/zeroedin/alloy/internal/output"
)

var _ = Describe("Output Writer", func() {

	Describe("File writing", func() {
		It("writes content to the correct output path", func() {
			tmpDir := GinkgoT().TempDir()
			content := []byte("<html><body>Hello</body></html>")

			err := output.WriteFile(tmpDir, "index.html", content)
			Expect(err).NotTo(HaveOccurred())

			written, err := os.ReadFile(filepath.Join(tmpDir, "index.html"))
			Expect(err).NotTo(HaveOccurred())
			Expect(written).To(Equal(content))
		})

		It("creates intermediate directories as needed", func() {
			tmpDir := GinkgoT().TempDir()
			content := []byte("<html><body>Post</body></html>")

			err := output.WriteFile(tmpDir, "blog/2024/my-post/index.html", content)
			Expect(err).NotTo(HaveOccurred())

			written, err := os.ReadFile(filepath.Join(tmpDir, "blog", "2024", "my-post", "index.html"))
			Expect(err).NotTo(HaveOccurred())
			Expect(written).To(Equal(content))
		})
	})

	// ── Pretty URL generation ─────────────────────────────────────────

	Describe("Pretty URL generation", func() {
		It("converts /about/ to about/index.html", func() {
			result := output.ComputeOutputPath("/about/")
			Expect(result).To(Equal("about/index.html"))
		})

		It("converts root / to index.html", func() {
			result := output.ComputeOutputPath("/")
			Expect(result).To(Equal("index.html"))
		})

		It("converts /blog/my-post/ to blog/my-post/index.html", func() {
			result := output.ComputeOutputPath("/blog/my-post/")
			Expect(result).To(Equal("blog/my-post/index.html"))
		})
	})

	// ── Multi-format output ───────────────────────────────────────────

	Describe("Multi-format output", func() {
		It("writes same page in multiple formats (html + json)", func() {
			tmpDir := GinkgoT().TempDir()
			formats := map[string][]byte{
				"html": []byte("<html><body>Hello</body></html>"),
				"json": []byte(`{"title":"Hello"}`),
			}

			err := output.WritePageFormats(tmpDir, "/about/", formats)
			Expect(err).NotTo(HaveOccurred())

			htmlContent, err := os.ReadFile(filepath.Join(tmpDir, "about", "index.html"))
			Expect(err).NotTo(HaveOccurred())
			Expect(htmlContent).To(Equal(formats["html"]))

			jsonContent, err := os.ReadFile(filepath.Join(tmpDir, "about", "index.json"))
			Expect(err).NotTo(HaveOccurred())
			Expect(jsonContent).To(Equal(formats["json"]))
		})
	})

	// ── permalink: false (no output) ──────────────────────────────────

	Describe("permalink: false pages", func() {
		It("ShouldWrite returns false for empty permalink", func() {
			// Guard: non-empty permalink must return true
			Expect(output.ShouldWrite("/about/")).To(BeTrue(),
				"guard: non-empty permalink must be writable")

			Expect(output.ShouldWrite("")).To(BeFalse(),
				"empty permalink (permalink: false) must not be written")
		})
	})

	// ── Alias output ──────────────────────────────────────────────────

	Describe("Alias output", func() {
		It("writes same content to all alias paths", func() {
			tmpDir := GinkgoT().TempDir()
			content := []byte("<html><body>About Us</body></html>")
			aliases := []string{"/about-us/", "/team/"}

			err := output.WriteAliases(tmpDir, aliases, content)
			Expect(err).NotTo(HaveOccurred())

			aboutUs, err := os.ReadFile(filepath.Join(tmpDir, "about-us", "index.html"))
			Expect(err).NotTo(HaveOccurred())
			Expect(aboutUs).To(Equal(content))

			team, err := os.ReadFile(filepath.Join(tmpDir, "team", "index.html"))
			Expect(err).NotTo(HaveOccurred())
			Expect(team).To(Equal(content))
		})
	})

	Describe("Output directory management", func() {
		It("cleans output directory when requested", func() {
			tmpDir := GinkgoT().TempDir()

			// Seed the directory with some files
			subDir := filepath.Join(tmpDir, "sub")
			Expect(os.MkdirAll(subDir, 0o755)).To(Succeed())
			Expect(os.WriteFile(filepath.Join(tmpDir, "old.html"), []byte("old"), 0o644)).To(Succeed())
			Expect(os.WriteFile(filepath.Join(subDir, "nested.html"), []byte("nested"), 0o644)).To(Succeed())

			err := output.CleanOutputDir(tmpDir)
			Expect(err).NotTo(HaveOccurred())

			entries, err := os.ReadDir(tmpDir)
			Expect(err).NotTo(HaveOccurred())
			Expect(entries).To(BeEmpty())
		})
	})

	// ── Directory cache (issue #365) ─────────────────────────────────
	// WriteFile calls os.MkdirAll per file. DirectoryCache tracks which
	// directories have been created so subsequent calls are a map lookup.

	Describe("DirectoryCache", func() {
		It("EnsureDir creates a directory that does not exist (issue #365)", func() {
			tmp := GinkgoT().TempDir()
			dc := output.NewDirectoryCache()
			dir := filepath.Join(tmp, "blog", "2024", "posts")
			err := dc.EnsureDir(dir)
			Expect(err).NotTo(HaveOccurred())
			info, statErr := os.Stat(dir)
			Expect(statErr).NotTo(HaveOccurred(),
				"EnsureDir must create the full directory tree including "+
					"intermediate directories (issue #365)")
			Expect(info.IsDir()).To(BeTrue())
		})

		It("EnsureDir skips already-created directories on subsequent calls (issue #365)", func() {
			tmp := GinkgoT().TempDir()
			dc := output.NewDirectoryCache()
			dir := filepath.Join(tmp, "about")
			Expect(dc.EnsureDir(dir)).To(Succeed())
			Expect(dc.EnsureDir(dir)).To(Succeed(),
				"EnsureDir must be idempotent — the second call for the same "+
					"directory must succeed without error (issue #365)")
		})

		It("EnsureDir creates deeply nested directory trees (issue #365)", func() {
			tmp := GinkgoT().TempDir()
			dc := output.NewDirectoryCache()
			dir := filepath.Join(tmp, "docs", "api", "v2", "endpoints", "users")
			Expect(dc.EnsureDir(dir)).To(Succeed())
			_, statErr := os.Stat(dir)
			Expect(statErr).NotTo(HaveOccurred(),
				"EnsureDir must handle deeply nested paths — os.MkdirAll "+
					"creates all intermediate components in one call")
		})
	})
})
