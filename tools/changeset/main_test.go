package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestChangeset(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Changeset Tool Suite")
}

var _ = Describe("Changeset Tool", func() {

	Describe("parseChangeset", func() {
		It("parses a patch changeset", func() {
			cs, err := parseChangeset("---\ntype: patch\n---\n\nFixed a bug.\n")
			Expect(err).NotTo(HaveOccurred())
			Expect(cs.bumpType).To(Equal("patch"))
			Expect(cs.summary).To(Equal("Fixed a bug."))
		})

		It("parses a minor changeset", func() {
			cs, err := parseChangeset("---\ntype: minor\n---\n\nAdded a feature.\n")
			Expect(err).NotTo(HaveOccurred())
			Expect(cs.bumpType).To(Equal("minor"))
			Expect(cs.summary).To(Equal("Added a feature."))
		})

		It("parses a major changeset", func() {
			cs, err := parseChangeset("---\ntype: major\n---\n\nBreaking change.\n")
			Expect(err).NotTo(HaveOccurred())
			Expect(cs.bumpType).To(Equal("major"))
			Expect(cs.summary).To(Equal("Breaking change."))
		})

		It("preserves multiline body with code blocks", func() {
			input := "---\ntype: minor\n---\n\nAdded config support.\n\n```yaml\ntitle: \"My Site\"\nbaseURL: \"https://example.com\"\nstructure:\n  content: \"src/content\"\n```\n"
			cs, err := parseChangeset(input)
			Expect(err).NotTo(HaveOccurred())
			Expect(cs.bumpType).To(Equal("minor"))
			Expect(cs.summary).To(ContainSubstring("```yaml"))
			Expect(cs.summary).To(ContainSubstring("src/content"))
		})

		It("preserves shell-like variable expansion syntax", func() {
			input := "---\ntype: minor\n---\n\n```js\nreturn `Hello, ${args[0]}!`;\n```\n"
			cs, err := parseChangeset(input)
			Expect(err).NotTo(HaveOccurred())
			Expect(cs.summary).To(ContainSubstring("${args[0]}"))
		})

		It("rejects missing frontmatter", func() {
			_, err := parseChangeset("no frontmatter here")
			Expect(err).To(HaveOccurred())
		})

		It("rejects invalid bump type", func() {
			_, err := parseChangeset("---\ntype: huge\n---\n\nSomething.\n")
			Expect(err).To(HaveOccurred())
		})

		It("rejects missing type field", func() {
			_, err := parseChangeset("---\nfoo: bar\n---\n\nSomething.\n")
			Expect(err).To(HaveOccurred())
		})
	})

	Describe("highestBump", func() {
		It("returns patch for a single patch", func() {
			Expect(highestBump([]changeset{{bumpType: "patch"}})).To(Equal("patch"))
		})

		It("returns minor when patch and minor are present", func() {
			Expect(highestBump([]changeset{
				{bumpType: "patch"},
				{bumpType: "minor"},
			})).To(Equal("minor"))
		})

		It("returns major when minor and major are present", func() {
			Expect(highestBump([]changeset{
				{bumpType: "minor"},
				{bumpType: "major"},
			})).To(Equal("major"))
		})

		It("returns major regardless of order", func() {
			Expect(highestBump([]changeset{
				{bumpType: "patch"},
				{bumpType: "major"},
				{bumpType: "minor"},
			})).To(Equal("major"))
		})
	})

	Describe("buildDescriptions", func() {
		It("returns flat output with no headers for a single bump level", func() {
			result := buildDescriptions([]changeset{
				{bumpType: "patch", summary: "Fixed bug A."},
				{bumpType: "patch", summary: "Fixed bug B."},
			})
			Expect(result).NotTo(ContainSubstring("###"))
			Expect(result).To(ContainSubstring("Fixed bug A."))
			Expect(result).To(ContainSubstring("Fixed bug B."))
		})

		It("groups under headers when multiple bump levels are present", func() {
			result := buildDescriptions([]changeset{
				{bumpType: "patch", summary: "Fixed a bug."},
				{bumpType: "minor", summary: "Added a feature."},
			})
			Expect(result).To(ContainSubstring("### Minor Changes"))
			Expect(result).To(ContainSubstring("### Patch Changes"))
			Expect(result).NotTo(ContainSubstring("### Major Changes"))
		})

		It("orders headers major then minor then patch", func() {
			result := buildDescriptions([]changeset{
				{bumpType: "patch", summary: "Fix."},
				{bumpType: "major", summary: "Breaking."},
				{bumpType: "minor", summary: "Feature."},
			})
			majorIdx := strings.Index(result, "### Major Changes")
			minorIdx := strings.Index(result, "### Minor Changes")
			patchIdx := strings.Index(result, "### Patch Changes")
			Expect(majorIdx).To(BeNumerically("<", minorIdx))
			Expect(minorIdx).To(BeNumerically("<", patchIdx))
		})

		It("places each description under its correct header", func() {
			result := buildDescriptions([]changeset{
				{bumpType: "minor", summary: "Added feature X."},
				{bumpType: "patch", summary: "Fixed bug Y."},
			})
			minorIdx := strings.Index(result, "### Minor Changes")
			patchIdx := strings.Index(result, "### Patch Changes")
			featureIdx := strings.Index(result, "Added feature X.")
			bugIdx := strings.Index(result, "Fixed bug Y.")
			Expect(featureIdx).To(BeNumerically(">", minorIdx))
			Expect(featureIdx).To(BeNumerically("<", patchIdx))
			Expect(bugIdx).To(BeNumerically(">", patchIdx))
		})

		It("includes multiple entries under the same header", func() {
			result := buildDescriptions([]changeset{
				{bumpType: "minor", summary: "Feature A."},
				{bumpType: "patch", summary: "Fix A."},
				{bumpType: "patch", summary: "Fix B."},
			})
			patchIdx := strings.Index(result, "### Patch Changes")
			fixAIdx := strings.Index(result, "Fix A.")
			fixBIdx := strings.Index(result, "Fix B.")
			Expect(fixAIdx).To(BeNumerically(">", patchIdx))
			Expect(fixBIdx).To(BeNumerically(">", patchIdx))
		})
	})

	Describe("bumpVersion", func() {
		DescribeTable("calculates the next version",
			func(current, bump, expected string) {
				result, err := bumpVersion(current, bump)
				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(Equal(expected))
			},
			Entry("0.0.0 patch", "0.0.0", "patch", "0.0.1"),
			Entry("0.0.0 minor", "0.0.0", "minor", "0.1.0"),
			Entry("0.0.0 major", "0.0.0", "major", "1.0.0"),
			Entry("1.2.3 patch", "1.2.3", "patch", "1.2.4"),
			Entry("1.2.3 minor", "1.2.3", "minor", "1.3.0"),
			Entry("1.2.3 major", "1.2.3", "major", "2.0.0"),
			Entry("minor resets patch", "0.1.0", "minor", "0.2.0"),
		)

		It("rejects invalid version strings", func() {
			_, err := bumpVersion("invalid", "patch")
			Expect(err).To(HaveOccurred())
		})
	})

	Describe("readChangesets", func() {
		It("finds changeset files and ignores README, AGENTS, CLAUDE, GEMINI", func() {
			dir := GinkgoT().TempDir()
			orig, _ := os.Getwd()
			Expect(os.Chdir(dir)).To(Succeed())
			DeferCleanup(func() { os.Chdir(orig) })

			Expect(os.MkdirAll(".changeset", 0o755)).To(Succeed())
			Expect(os.WriteFile(".changeset/README.md", []byte("# Changesets"), 0o644)).To(Succeed())
			Expect(os.WriteFile(".changeset/AGENTS.md", []byte("# Agents"), 0o644)).To(Succeed())
			Expect(os.WriteFile(".changeset/fix-bug.md", []byte("---\ntype: patch\n---\n\nFixed a bug.\n"), 0o644)).To(Succeed())
			Expect(os.WriteFile(".changeset/add-feature.md", []byte("---\ntype: minor\n---\n\nAdded a feature.\n"), 0o644)).To(Succeed())

			changesets, files, err := readChangesets()
			Expect(err).NotTo(HaveOccurred())
			Expect(changesets).To(HaveLen(2))
			Expect(files).To(HaveLen(2))

			for _, f := range files {
				base := strings.ToLower(filepath.Base(f))
				Expect(base).NotTo(Equal("readme.md"))
				Expect(base).NotTo(Equal("agents.md"))
			}
		})

		It("returns empty when .changeset directory does not exist", func() {
			dir := GinkgoT().TempDir()
			orig, _ := os.Getwd()
			Expect(os.Chdir(dir)).To(Succeed())
			DeferCleanup(func() { os.Chdir(orig) })

			changesets, files, err := readChangesets()
			Expect(err).NotTo(HaveOccurred())
			Expect(changesets).To(BeEmpty())
			Expect(files).To(BeEmpty())
		})
	})

	Describe("readVersion", func() {
		It("extracts the version from cmd/version.go", func() {
			dir := GinkgoT().TempDir()
			orig, _ := os.Getwd()
			Expect(os.Chdir(dir)).To(Succeed())
			DeferCleanup(func() { os.Chdir(orig) })

			Expect(os.MkdirAll("cmd", 0o755)).To(Succeed())
			Expect(os.WriteFile("cmd/version.go", []byte("package cmd\n\nvar Version = \"1.2.3\"\n"), 0o644)).To(Succeed())

			v, err := readVersion()
			Expect(err).NotTo(HaveOccurred())
			Expect(v).To(Equal("1.2.3"))
		})
	})

	Describe("writeVersion", func() {
		It("replaces the version string in cmd/version.go", func() {
			dir := GinkgoT().TempDir()
			orig, _ := os.Getwd()
			Expect(os.Chdir(dir)).To(Succeed())
			DeferCleanup(func() { os.Chdir(orig) })

			Expect(os.MkdirAll("cmd", 0o755)).To(Succeed())
			Expect(os.WriteFile("cmd/version.go", []byte("package cmd\n\nvar Version = \"1.2.3\"\n"), 0o644)).To(Succeed())

			Expect(writeVersion("1.2.3", "1.3.0")).To(Succeed())

			data, err := os.ReadFile("cmd/version.go")
			Expect(err).NotTo(HaveOccurred())
			Expect(string(data)).To(ContainSubstring(`Version = "1.3.0"`))
			Expect(string(data)).NotTo(ContainSubstring(`Version = "1.2.3"`))
		})
	})

	Describe("writeChangelog", func() {
		It("creates CHANGELOG.md when none exists", func() {
			dir := GinkgoT().TempDir()
			orig, _ := os.Getwd()
			Expect(os.Chdir(dir)).To(Succeed())
			DeferCleanup(func() { os.Chdir(orig) })

			Expect(writeChangelog("0.1.0", "- Added feature A.\n- Added feature B.\n")).To(Succeed())

			data, err := os.ReadFile("CHANGELOG.md")
			Expect(err).NotTo(HaveOccurred())
			content := string(data)
			Expect(content).To(HavePrefix("## v0.1.0 ("))
			Expect(content).To(ContainSubstring("- Added feature A."))
			Expect(content).To(ContainSubstring("- Added feature B."))
		})

		It("prepends new version above existing content", func() {
			dir := GinkgoT().TempDir()
			orig, _ := os.Getwd()
			Expect(os.Chdir(dir)).To(Succeed())
			DeferCleanup(func() { os.Chdir(orig) })

			Expect(os.WriteFile("CHANGELOG.md", []byte("## v0.1.0 (2025-01-01)\n\n- Old entry.\n"), 0o644)).To(Succeed())
			Expect(writeChangelog("0.2.0", "- New entry.\n")).To(Succeed())

			data, err := os.ReadFile("CHANGELOG.md")
			Expect(err).NotTo(HaveOccurred())
			content := string(data)

			v02 := strings.Index(content, "## v0.2.0")
			v01 := strings.Index(content, "## v0.1.0")
			Expect(v02).To(BeNumerically("<", v01),
				"new version must appear before old version")
			Expect(content).To(ContainSubstring("- New entry."))
			Expect(content).To(ContainSubstring("- Old entry."))
		})

		It("preserves code blocks with shell variable syntax", func() {
			dir := GinkgoT().TempDir()
			orig, _ := os.Getwd()
			Expect(os.Chdir(dir)).To(Succeed())
			DeferCleanup(func() { os.Chdir(orig) })

			body := "- **Plugins**: Drop a JS file in `plugins/`\n\n  ```js\n  alloy.shortcode(\"greeting\", (args) => {\n      return `<p>Hello, ${args[0]}!</p>`;\n  });\n  ```\n"
			Expect(writeChangelog("0.1.0", body)).To(Succeed())

			data, err := os.ReadFile("CHANGELOG.md")
			Expect(err).NotTo(HaveOccurred())
			content := string(data)
			Expect(content).To(ContainSubstring("${args[0]}"),
				"code block content must not be corrupted by shell expansion")
			Expect(content).To(ContainSubstring("```js"))
		})
	})

	Describe("slugify", func() {
		DescribeTable("generates URL-safe slugs",
			func(input, expected string) {
				Expect(slugify(input)).To(Equal(expected))
			},
			Entry("simple words", "Fixed a bug", "fixed-a-bug"),
			Entry("mixed case", "Added YAML support", "added-yaml-support"),
			Entry("extra spaces", "  multiple   spaces  ", "multiple-spaces"),
			Entry("trailing dash", "trailing-dash-", "trailing-dash"),
			Entry("special characters", "special!@#chars", "special-chars"),
		)
	})

	Describe("runVersion (end-to-end)", func() {
		It("multiple changesets produce a single non-empty changelog entry", func() {
			dir := GinkgoT().TempDir()
			orig, _ := os.Getwd()
			Expect(os.Chdir(dir)).To(Succeed())
			DeferCleanup(func() { os.Chdir(orig) })

			Expect(os.MkdirAll("cmd", 0o755)).To(Succeed())
			Expect(os.WriteFile("cmd/version.go", []byte("package cmd\n\nvar Version = \"0.0.0\"\n"), 0o644)).To(Succeed())
			Expect(os.MkdirAll(".changeset", 0o755)).To(Succeed())
			Expect(os.WriteFile(".changeset/README.md", []byte("# Changesets\n"), 0o644)).To(Succeed())

			Expect(os.WriteFile(".changeset/fix-bug.md", []byte("---\ntype: patch\n---\n\nFixed rendering bug in liquid templates.\n"), 0o644)).To(Succeed())
			Expect(os.WriteFile(".changeset/add-feature.md", []byte("---\ntype: minor\n---\n\nAdded YAML data file support.\n"), 0o644)).To(Succeed())
			Expect(os.WriteFile(".changeset/breaking.md", []byte("---\ntype: major\n---\n\nRenamed `output` config key to `build.output`.\n"), 0o644)).To(Succeed())

			Expect(runVersion()).To(Succeed())

			clData, err := os.ReadFile("CHANGELOG.md")
			Expect(err).NotTo(HaveOccurred())
			cl := string(clData)

			By("using the highest bump type (major)")
			Expect(cl).To(ContainSubstring("## v1.0.0"))

			By("including all three changeset descriptions")
			Expect(cl).To(ContainSubstring("Fixed rendering bug in liquid templates."))
			Expect(cl).To(ContainSubstring("Added YAML data file support."))
			Expect(cl).To(ContainSubstring("Renamed `output` config key to `build.output`."))

			By("producing a changelog with more than just the heading")
			lines := strings.Split(strings.TrimSpace(cl), "\n")
			Expect(len(lines)).To(BeNumerically(">", 2),
				"changelog must contain the heading plus all descriptions — not just the heading alone")
		})

		It("changelog is non-empty after processing the exact initial-release changeset", func() {
			dir := GinkgoT().TempDir()
			orig, _ := os.Getwd()
			Expect(os.Chdir(dir)).To(Succeed())
			DeferCleanup(func() { os.Chdir(orig) })

			Expect(os.MkdirAll("cmd", 0o755)).To(Succeed())
			Expect(os.WriteFile("cmd/version.go", []byte("package cmd\n\nvar Version = \"0.0.0\"\n"), 0o644)).To(Succeed())
			Expect(os.MkdirAll(".changeset", 0o755)).To(Succeed())
			Expect(os.WriteFile(".changeset/README.md", []byte("# Changesets\n"), 0o644)).To(Succeed())

			// The exact content that broke PRs #748, #749, #750, #751
			initialRelease := `---
type: minor
---

Initial release of Alloy — a fast, extensible static site generator written in Go.

- **Config**: Customize your project structure, build output, content formats, and plugin settings in YAML, TOML, or JSON

  ` + "```yaml" + `
  title: "My Site"
  baseURL: "https://example.com"
  structure:
    content: "src/content"
    layouts: "src/layouts"
  templates:
    engine: "liquid"
  ` + "```" + `

- **Content**: Write pages in Markdown or plain HTML with YAML frontmatter
- **Data**: Load YAML, JSON, and CSV data files — available globally in templates as ` + "`site.data`" + `
- **Cascade**: Inherit layout, metadata, and configuration down the directory tree via ` + "`_data.yaml`" + ` files with deep merge
- **Permalinks**: Control output URLs per-collection with token-based patterns
- **Collections**: Group content and generate taxonomy pages
- **Templates**: Liquid and Go ` + "`html/template`" + ` engines with shortcodes, filters, and composable layouts
- **Output**: Generate sitemaps, feeds, and multiple output formats per page
- **Assets**: Process assets through the build pipeline with built-in cache-busting support
- **Static**: Copy static files with passthrough mappings and glob-based exclude patterns
- **Pagination**: Paginate collections with configurable page size and custom permalink patterns
- **i18n**: Build multilingual sites with per-language content directories, URL prefixing, and translation strings
- **Pipeline**: Incremental rebuilds that only reprocess changed files
- **Plugins (QuickJS)**: Drop a JS file in ` + "`plugins/`" + ` for in-process filters, hooks, and shortcodes — no Node.js required

  ` + "```js" + `
  export default function(alloy) {
      alloy.shortcode("greeting", (args) => {
          return ` + "`" + `<p>Hello, ${args[0]}!</p>` + "`" + `;
      });
  }
  ` + "```" + `

- **Plugins (WASM)**: Compile filters from Rust, TinyGo, or AssemblyScript for near-native performance
- **Hooks**: React to build lifecycle events and inject virtual pages
- **CLI**: ` + "`alloy build`" + `, ` + "`alloy dev`" + ` (development server with file watcher and live reload), ` + "`alloy serve`" + `, ` + "`alloy init`" + `, and ` + "`alloy version`" + `
`
			Expect(os.WriteFile(".changeset/initial-release.md", []byte(initialRelease), 0o644)).To(Succeed())

			Expect(runVersion()).To(Succeed())

			clData, err := os.ReadFile("CHANGELOG.md")
			Expect(err).NotTo(HaveOccurred())
			cl := string(clData)

			By("having the version heading")
			Expect(cl).To(ContainSubstring("## v0.1.0"))

			By("having substantial content, not just a heading")
			Expect(len(cl)).To(BeNumerically(">", 500),
				"CHANGELOG.md must contain the full initial release description — "+
					"this is the exact content that produced empty changelogs in PRs #748-#751")

			By("preserving every feature line")
			for _, feature := range []string{
				"**Config**",
				"**Content**",
				"**Data**",
				"**Cascade**",
				"**Permalinks**",
				"**Collections**",
				"**Templates**",
				"**Output**",
				"**Assets**",
				"**Static**",
				"**Pagination**",
				"**i18n**",
				"**Pipeline**",
				"**Plugins (QuickJS)**",
				"**Plugins (WASM)**",
				"**Hooks**",
				"**CLI**",
			} {
				Expect(cl).To(ContainSubstring(feature),
					"changelog must contain %s", feature)
			}

			By("preserving code blocks intact")
			Expect(cl).To(ContainSubstring("```yaml"))
			Expect(cl).To(ContainSubstring("```js"))
			Expect(cl).To(ContainSubstring("${args[0]}"),
				"${args[0]} must not be eaten by shell variable expansion")

			By("preserving backtick-wrapped inline code")
			Expect(cl).To(ContainSubstring("`site.data`"))
			Expect(cl).To(ContainSubstring("`alloy build`"))
		})

		It("bumps version, writes changelog, deletes changesets", func() {
			dir := GinkgoT().TempDir()
			orig, _ := os.Getwd()
			Expect(os.Chdir(dir)).To(Succeed())
			DeferCleanup(func() { os.Chdir(orig) })

			Expect(os.MkdirAll("cmd", 0o755)).To(Succeed())
			Expect(os.WriteFile("cmd/version.go", []byte("package cmd\n\nvar Version = \"0.0.0\"\n"), 0o644)).To(Succeed())

			Expect(os.MkdirAll(".changeset", 0o755)).To(Succeed())
			Expect(os.WriteFile(".changeset/README.md", []byte("# Changesets\n"), 0o644)).To(Succeed())

			body := "Initial release.\n\n- **Config**: YAML, TOML, JSON support\n\n  ```yaml\n  title: \"My Site\"\n  ```\n\n- **Plugins**: In-process JS\n\n  ```js\n  alloy.shortcode(\"hi\", (args) => `Hello, ${args[0]}!`);\n  ```\n"
			Expect(os.WriteFile(".changeset/initial.md", []byte("---\ntype: minor\n---\n\n"+body), 0o644)).To(Succeed())

			Expect(runVersion()).To(Succeed())

			By("bumping the version in cmd/version.go")
			vData, err := os.ReadFile("cmd/version.go")
			Expect(err).NotTo(HaveOccurred())
			Expect(string(vData)).To(ContainSubstring(`Version = "0.1.0"`))

			By("writing CHANGELOG.md with full content")
			clData, err := os.ReadFile("CHANGELOG.md")
			Expect(err).NotTo(HaveOccurred())
			cl := string(clData)
			Expect(cl).To(ContainSubstring("## v0.1.0"))
			Expect(cl).To(ContainSubstring("${args[0]}"),
				"code block content must survive the full pipeline")
			Expect(cl).To(ContainSubstring("```yaml"))

			By("deleting consumed changeset files")
			_, err = os.Stat(".changeset/initial.md")
			Expect(os.IsNotExist(err)).To(BeTrue())

			By("preserving README.md")
			_, err = os.Stat(".changeset/README.md")
			Expect(err).NotTo(HaveOccurred())

			By("writing output files for the workflow")
			version, err := os.ReadFile("/tmp/release-version.txt")
			Expect(err).NotTo(HaveOccurred())
			Expect(string(version)).To(Equal("0.1.0"))

			notes, err := os.ReadFile("/tmp/release-notes.md")
			Expect(err).NotTo(HaveOccurred())
			Expect(string(notes)).To(ContainSubstring("${args[0]}"))
		})
	})
})
