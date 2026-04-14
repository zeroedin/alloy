package config_test

import (
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/zeroedin/alloy/internal/config"
)

var _ = Describe("Config", func() {

	Describe("Load", func() {

		Context("YAML config", func() {
			var (
				cfg *config.Config
				err error
			)

			BeforeEach(func() {
				cfg, err = config.Load("testdata/valid.yaml")
			})

			It("loads without error", func() {
				Expect(err).NotTo(HaveOccurred())
			})

			It("returns a non-nil Config", func() {
				Expect(cfg).NotTo(BeNil())
			})

			It("parses title", func() {
				Expect(err).NotTo(HaveOccurred())
				Expect(cfg.Title).To(Equal("Test Site"))
			})

			It("parses baseURL", func() {
				Expect(err).NotTo(HaveOccurred())
				Expect(cfg.BaseURL).To(Equal("https://example.com"))
			})

			It("parses language", func() {
				Expect(err).NotTo(HaveOccurred())
				Expect(cfg.Language).To(Equal("en"))
			})

			It("parses build.output", func() {
				Expect(err).NotTo(HaveOccurred())
				Expect(cfg.Build.Output).To(Equal("_site"))
			})

			It("parses build.clean", func() {
				Expect(err).NotTo(HaveOccurred())
				Expect(cfg.Build.Clean).To(BeTrue())
			})

			It("parses content.formats", func() {
				Expect(err).NotTo(HaveOccurred())
				Expect(cfg.Content.Formats).To(Equal([]string{"md", "html"}))
			})

			It("parses content.markdown.goldmark options", func() {
				Expect(err).NotTo(HaveOccurred())
				Expect(cfg.Content.Markdown.Goldmark.Unsafe).To(BeTrue())
				Expect(cfg.Content.Markdown.Goldmark.Typographer).To(BeTrue())
				Expect(cfg.Content.Markdown.Goldmark.TemplateTags).To(BeTrue())
			})

			It("parses templates.engine", func() {
				Expect(err).NotTo(HaveOccurred())
				Expect(cfg.Templates.Engine).To(Equal("liquid"))
			})

			It("parses plugins.node", func() {
				Expect(err).NotTo(HaveOccurred())
				Expect(cfg.Plugins.Node).To(BeTrue())
			})

			It("parses plugins.timeout", func() {
				Expect(err).NotTo(HaveOccurred())
				Expect(cfg.Plugins.Timeout).To(Equal(5000))
			})

			It("parses taxonomies map with permalink and layout", func() {
				Expect(err).NotTo(HaveOccurred())
				Expect(cfg.Taxonomies).To(HaveKey("tags"))
				Expect(cfg.Taxonomies["tags"].Permalink).To(Equal("/tags/:slug/"))
				Expect(cfg.Taxonomies["tags"].Layout).To(Equal("tags"))
				Expect(cfg.Taxonomies).To(HaveKey("categories"))
				Expect(cfg.Taxonomies["categories"].Permalink).To(Equal("/categories/:slug/"))
				Expect(cfg.Taxonomies["categories"].Layout).To(Equal("categories"))
			})

			It("parses permalinks map", func() {
				Expect(err).NotTo(HaveOccurred())
				Expect(cfg.Permalinks).To(HaveKeyWithValue("blog", "/:year/:month/:slug/"))
				Expect(cfg.Permalinks).To(HaveKeyWithValue("default", "/:slug/"))
			})

			It("parses pagination section with path", func() {
				Expect(err).NotTo(HaveOccurred())
				Expect(cfg.Pagination.Path).To(Equal("page"))
			})

			It("parses passthrough array with from/to", func() {
				Expect(err).NotTo(HaveOccurred())
				Expect(cfg.Passthrough).To(HaveLen(2))
				Expect(cfg.Passthrough[0].From).To(Equal("../design-system/dist/elements"))
				Expect(cfg.Passthrough[0].To).To(Equal("elements"))
				Expect(cfg.Passthrough[1].From).To(Equal("../shared-assets/fonts"))
				Expect(cfg.Passthrough[1].To).To(Equal("assets/fonts"))
			})

			It("parses sources map with rest type", func() {
				Expect(err).NotTo(HaveOccurred())
				Expect(cfg.Sources).To(HaveKey("posts"))
				Expect(cfg.Sources["posts"].Type).To(Equal("rest"))
				Expect(cfg.Sources["posts"].URL).To(Equal("https://api.example.com/posts.json"))
				Expect(cfg.Sources["posts"].Cache).To(Equal(3600))
				Expect(cfg.Sources["posts"].As).To(Equal("posts"))
			})

			It("parses sources map with graphql type", func() {
				Expect(err).NotTo(HaveOccurred())
				Expect(cfg.Sources).To(HaveKey("products"))
				Expect(cfg.Sources["products"].Type).To(Equal("graphql"))
				Expect(cfg.Sources["products"].Endpoint).To(Equal("https://api.example.com/graphql"))
				Expect(cfg.Sources["products"].Query).To(Equal("{ products { id, name, price, slug } }"))
				Expect(cfg.Sources["products"].Cache).To(Equal(1800))
				Expect(cfg.Sources["products"].As).To(Equal("products"))
			})

			It("parses sitemap config", func() {
				Expect(err).NotTo(HaveOccurred())
				Expect(cfg.Sitemap.ChangeFreq).To(Equal("weekly"))
				Expect(cfg.Sitemap.Priority).To(Equal(0.5))
			})

			It("parses structure config with custom directory paths", func() {
				Expect(err).NotTo(HaveOccurred())
				Expect(cfg.Structure.Content).To(Equal("content"))
				Expect(cfg.Structure.Layouts).To(Equal("layouts"))
				Expect(cfg.Structure.Assets).To(Equal("assets"))
				Expect(cfg.Structure.Static).To(Equal("static"))
				Expect(cfg.Structure.Data).To(Equal("data"))
			})

			It("parses collections config with sortBy and order", func() {
				Expect(err).NotTo(HaveOccurred())
				Expect(cfg.Collections).To(HaveKey("blog"))
				Expect(cfg.Collections["blog"].SortBy).To(Equal("date"))
				Expect(cfg.Collections["blog"].Order).To(Equal("desc"))
			})

			It("parses SSR config with build and serve", func() {
				Expect(err).NotTo(HaveOccurred())
				Expect(cfg.SSR).NotTo(BeNil())
				Expect(cfg.SSR.Build).To(Equal("golit transform _site/"))
				Expect(cfg.SSR.Serve.Cmd).To(Equal("golit serve --defs bundles/"))
				Expect(cfg.SSR.Serve.Endpoint).To(Equal("http://localhost:9777/render"))
			})

			It("parses languages config with title, weight, root, and strings", func() {
				Expect(err).NotTo(HaveOccurred())
				Expect(cfg.Languages).To(HaveKey("en"))
				Expect(cfg.Languages["en"].Title).To(Equal("Test Site"))
				Expect(cfg.Languages["en"].Weight).To(Equal(1))
				Expect(cfg.Languages["en"].Root).To(BeTrue())
				Expect(cfg.Languages["en"].Strings).To(HaveKeyWithValue("read_more", "Read more"))
				Expect(cfg.Languages["en"].Strings).To(HaveKeyWithValue("posted_on", "Posted on"))

				Expect(cfg.Languages).To(HaveKey("fr"))
				Expect(cfg.Languages["fr"].Title).To(Equal("Site Test"))
				Expect(cfg.Languages["fr"].Weight).To(Equal(2))
				Expect(cfg.Languages["fr"].Strings).To(HaveKeyWithValue("read_more", "Lire la suite"))
				Expect(cfg.Languages["fr"].Strings).To(HaveKeyWithValue("posted_on", "Publi\u00e9 le"))
			})
		})

		Context("TOML config", func() {
			It("loads alloy.config.toml and produces same Config struct as YAML", func() {
				tomlCfg, err := config.Load("testdata/valid.toml")
				Expect(err).NotTo(HaveOccurred())
				Expect(tomlCfg).NotTo(BeNil())
				Expect(tomlCfg.Title).To(Equal("Test Site"))
				Expect(tomlCfg.BaseURL).To(Equal("https://example.com"))
				Expect(tomlCfg.Build.Output).To(Equal("_site"))
			})
		})

		Context("JSON config", func() {
			It("loads alloy.config.json and produces same Config struct as YAML", func() {
				jsonCfg, err := config.Load("testdata/valid.json")
				Expect(err).NotTo(HaveOccurred())
				Expect(jsonCfg).NotTo(BeNil())
				Expect(jsonCfg.Title).To(Equal("Test Site"))
				Expect(jsonCfg.BaseURL).To(Equal("https://example.com"))
				Expect(jsonCfg.Build.Output).To(Equal("_site"))
			})
		})

		Context("validation", func() {
			It("returns error for invalid YAML", func() {
				_, err := config.Load("testdata/invalid.yaml")
				Expect(err).To(HaveOccurred())
				Expect(err).NotTo(MatchError(config.ErrNotImplemented))
			})
		})
	})

	Describe("DetectConfigFile", func() {
		Context("when no config file exists", func() {
			It("returns error when no config file found in empty dir", func() {
				dir, err := os.MkdirTemp("", "alloy-empty-*")
				Expect(err).NotTo(HaveOccurred())
				defer os.RemoveAll(dir)

				_, detectErr := config.DetectConfigFile(dir)
				Expect(detectErr).To(HaveOccurred())
				Expect(detectErr).NotTo(MatchError(config.ErrNotImplemented))
			})
		})
	})

	Describe("LoadWithDefaults", func() {
		var (
			cfg *config.Config
			err error
		)

		BeforeEach(func() {
			cfg, err = config.LoadWithDefaults("testdata/minimal.yaml")
		})

		It("loads without error", func() {
			Expect(err).NotTo(HaveOccurred())
		})

		It("defaults build.output to _site", func() {
			Expect(err).NotTo(HaveOccurred())
			Expect(cfg.Build.Output).To(Equal("_site"))
		})

		It("defaults templates.engine to liquid", func() {
			Expect(err).NotTo(HaveOccurred())
			Expect(cfg.Templates.Engine).To(Equal("liquid"))
		})

		It("defaults content.markdown.goldmark.templateTags to true", func() {
			Expect(err).NotTo(HaveOccurred())
			Expect(cfg.Content.Markdown.Goldmark.TemplateTags).To(BeTrue())
		})

		It("defaults pagination.path to page", func() {
			Expect(err).NotTo(HaveOccurred())
			Expect(cfg.Pagination.Path).To(Equal("page"))
		})

		It("defaults plugins.timeout to 5000", func() {
			Expect(err).NotTo(HaveOccurred())
			Expect(cfg.Plugins.Timeout).To(Equal(5000))
		})

		It("defaults content.formats to md and html", func() {
			Expect(err).NotTo(HaveOccurred())
			Expect(cfg.Content.Formats).To(Equal([]string{"md", "html"}),
				"content.formats must default to md and html per spec")
		})

		It("defaults language to en", func() {
			Expect(err).NotTo(HaveOccurred())
			Expect(cfg.Language).To(Equal("en"),
				"language must default to en per spec")
		})

		It("defaults build.clean to true", func() {
			Expect(err).NotTo(HaveOccurred())
			Expect(cfg.Build.Clean).To(BeTrue(),
				"build.clean must default to true per spec")
		})

		It("defaults structure.content to content", func() {
			Expect(err).NotTo(HaveOccurred())
			Expect(cfg.Structure.Content).To(Equal("content"),
				"structure.content must default to 'content'")
		})
	})

	// ── ApplyDefaults nil taxonomy handling (issue #65) ─────────────

	Describe("ApplyDefaults", func() {
		It("replaces nil TaxonomyConfig with zero-value struct", func() {
			cfg := &config.Config{
				Taxonomies: map[string]*config.TaxonomyConfig{"tags": nil},
			}
			config.ApplyDefaults(cfg)
			Expect(cfg.Taxonomies["tags"]).NotTo(BeNil(),
				"nil TaxonomyConfig must be replaced with zero-value struct")
		})

		It("replaces multiple nil TaxonomyConfig entries", func() {
			cfg := &config.Config{
				Taxonomies: map[string]*config.TaxonomyConfig{
					"tags":       nil,
					"categories": nil,
				},
			}
			config.ApplyDefaults(cfg)
			Expect(cfg.Taxonomies["tags"]).NotTo(BeNil(),
				"nil tags entry must be replaced")
			Expect(cfg.Taxonomies["categories"]).NotTo(BeNil(),
				"nil categories entry must be replaced")
		})

		It("preserves non-nil TaxonomyConfig entries", func() {
			cfg := &config.Config{
				Taxonomies: map[string]*config.TaxonomyConfig{
					"tags": {Permalink: "/tags/:slug/", Layout: "tags"},
				},
			}
			config.ApplyDefaults(cfg)
			Expect(cfg.Taxonomies["tags"].Permalink).To(Equal("/tags/:slug/"),
				"existing permalink must not be overwritten")
			Expect(cfg.Taxonomies["tags"].Layout).To(Equal("tags"),
				"existing layout must not be overwritten")
		})
	})

	// ── Config file auto-detection (§1 Data Formats) ─────────────────

	Describe("DetectConfigFile auto-detection", func() {
		It("detects alloy.config.yaml", func() {
			dir := GinkgoT().TempDir()
			err := os.WriteFile(dir+"/alloy.config.yaml", []byte("title: Test"), 0644)
			Expect(err).NotTo(HaveOccurred())

			path, detectErr := config.DetectConfigFile(dir)
			Expect(detectErr).NotTo(HaveOccurred(),
				"must detect alloy.config.yaml")
			Expect(path).To(ContainSubstring("alloy.config.yaml"))
		})

		It("detects alloy.config.yml as alternate YAML extension", func() {
			dir := GinkgoT().TempDir()
			err := os.WriteFile(dir+"/alloy.config.yml", []byte("title: Test"), 0644)
			Expect(err).NotTo(HaveOccurred())

			path, detectErr := config.DetectConfigFile(dir)
			Expect(detectErr).NotTo(HaveOccurred(),
				"must detect alloy.config.yml")
			Expect(path).To(ContainSubstring("alloy.config.yml"))
		})

		It("detects alloy.config.toml", func() {
			dir := GinkgoT().TempDir()
			err := os.WriteFile(dir+"/alloy.config.toml", []byte(`title = "Test"`), 0644)
			Expect(err).NotTo(HaveOccurred())

			path, detectErr := config.DetectConfigFile(dir)
			Expect(detectErr).NotTo(HaveOccurred(),
				"must detect alloy.config.toml")
			Expect(path).To(ContainSubstring("alloy.config.toml"))
		})

		It("detects alloy.config.json", func() {
			dir := GinkgoT().TempDir()
			err := os.WriteFile(dir+"/alloy.config.json", []byte(`{"title":"Test"}`), 0644)
			Expect(err).NotTo(HaveOccurred())

			path, detectErr := config.DetectConfigFile(dir)
			Expect(detectErr).NotTo(HaveOccurred(),
				"must detect alloy.config.json")
			Expect(path).To(ContainSubstring("alloy.config.json"))
		})
	})

	// ── CLI flag override merging (§2, §9) ───────────────────────────

	Describe("MergeFlags", func() {
		It("--output flag overrides build.output from config", func() {
			cfg := &config.Config{
				Title: "Test Site",
				Build: config.BuildConfig{Output: "_site"},
			}
			config.MergeFlags(cfg, map[string]interface{}{
				"output": "dist",
			})
			Expect(cfg.Build.Output).To(Equal("dist"),
				"--output flag must override build.output")
		})

		It("flags not set leave config values unchanged", func() {
			cfg := &config.Config{
				Title: "Test Site",
				Build: config.BuildConfig{Output: "_site", Clean: true},
			}

			// Guard: setting a flag must actually change the value
			config.MergeFlags(cfg, map[string]interface{}{
				"output": "dist",
			})
			Expect(cfg.Build.Output).To(Equal("dist"),
				"guard: MergeFlags must apply set flags")

			// Reset and test: empty flags must leave values unchanged
			cfg.Build.Output = "_site"
			config.MergeFlags(cfg, map[string]interface{}{})
			Expect(cfg.Build.Output).To(Equal("_site"),
				"unset flags must not change config values")
			Expect(cfg.Build.Clean).To(BeTrue(),
				"unset flags must not change config values")
		})

		It("--verbose flag is merged into config", func() {
			cfg := &config.Config{Title: "Test Site"}
			config.MergeFlags(cfg, map[string]interface{}{
				"verbose": true,
			})
			Expect(cfg.Verbose).To(BeTrue(),
				"--verbose flag must be accessible on config after merge")
		})

		It("--root flag overrides ProjectRoot", func() {
			cfg := &config.Config{
				Title:       "Test Site",
				ProjectRoot: "/original/project",
			}
			config.MergeFlags(cfg, map[string]interface{}{
				"root": "/override/root",
			})
			Expect(cfg.ProjectRoot).To(Equal("/override/root"),
				"--root flag must override ProjectRoot from config file location")
		})

		It("--root empty string does not override ProjectRoot", func() {
			cfg := &config.Config{
				Title:       "Test Site",
				ProjectRoot: "/original/project",
			}
			config.MergeFlags(cfg, map[string]interface{}{
				"root": "",
			})
			Expect(cfg.ProjectRoot).To(Equal("/original/project"),
				"empty --root must not change ProjectRoot")
		})

		It("--root with relative path resolves to absolute", func() {
			cfg := &config.Config{
				Title:       "Test Site",
				ProjectRoot: "/original/project",
			}
			config.MergeFlags(cfg, map[string]interface{}{
				"root": ".",
			})
			Expect(cfg.ProjectRoot).NotTo(Equal("/original/project"),
				"relative --root must override ProjectRoot")
			Expect(cfg.ProjectRoot).NotTo(Equal("."),
				"relative --root must be resolved to absolute path")
			Expect(filepath.IsAbs(cfg.ProjectRoot)).To(BeTrue(),
				"--root must always resolve to an absolute path")
		})

		It("--root with relative subdirectory resolves to absolute", func() {
			cfg := &config.Config{
				Title:       "Test Site",
				ProjectRoot: "/original/project",
			}
			config.MergeFlags(cfg, map[string]interface{}{
				"root": "./deploy",
			})
			Expect(filepath.IsAbs(cfg.ProjectRoot)).To(BeTrue(),
				"--root with relative subdir must resolve to absolute path")
			Expect(cfg.ProjectRoot).To(HaveSuffix("/deploy"),
				"resolved path must end with the relative directory name")
		})
	})

	// ── Config validation (semantic errors) ──────────────────────────

	Describe("Validate", func() {
		It("returns error for empty baseURL", func() {
			cfg := &config.Config{
				Title:   "Test Site",
				BaseURL: "",
			}
			err := config.Validate(cfg)
			Expect(err).To(HaveOccurred(),
				"empty baseURL must be a validation error")
			Expect(err.Error()).To(ContainSubstring("baseURL"),
				"error must mention the invalid field")
		})

		It("returns error for negative plugin timeout", func() {
			cfg := &config.Config{
				Title:   "Test Site",
				BaseURL: "https://example.com",
				Plugins: config.PluginsConfig{Timeout: -1},
			}
			err := config.Validate(cfg)
			Expect(err).To(HaveOccurred(),
				"negative timeout must be a validation error")
			Expect(err.Error()).To(ContainSubstring("timeout"),
				"error must mention the invalid field")
		})

		It("includes field name in baseURL validation error", func() {
			cfg := &config.Config{
				Title:   "Test",
				BaseURL: "not-a-url",
			}
			err := config.Validate(cfg)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("baseURL"),
				"validation error must name the invalid field")
		})

		It("includes field name in negative timeout error", func() {
			cfg := &config.Config{
				Title:   "Test",
				BaseURL: "https://example.com",
				Plugins: config.PluginsConfig{Timeout: -5},
			}
			err := config.Validate(cfg)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(
				SatisfyAny(
					ContainSubstring("timeout"),
					ContainSubstring("negative"),
				),
				"validation error must reference the invalid timeout field",
			)
		})

		It("returns nil for a valid config", func() {
			cfg := &config.Config{
				Title:   "Test Site",
				BaseURL: "https://example.com",
				Plugins: config.PluginsConfig{Timeout: 5000},
			}
			err := config.Validate(cfg)
			Expect(err).NotTo(HaveOccurred(),
				"valid config must pass validation")
		})

		It("accepts config with empty taxonomies map", func() {
			cfg := &config.Config{
				Title:      "Test Site",
				BaseURL:    "https://example.com",
				Plugins:    config.PluginsConfig{Timeout: 5000},
				Taxonomies: map[string]*config.TaxonomyConfig{},
			}
			err := config.Validate(cfg)
			Expect(err).NotTo(HaveOccurred(),
				"config with zero taxonomies must be valid")
		})
	})
})
