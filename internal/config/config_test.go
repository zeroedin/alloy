package config_test

import (
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/zeroedin/alloy/internal/config"
)

func boolPtr(b bool) *bool { return &b }

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
				Expect(cfg.Build.CleanValue()).To(BeTrue())
			})

			It("parses content.formats", func() {
				Expect(err).NotTo(HaveOccurred())
				Expect(cfg.Content.Formats).To(Equal([]string{"md", "html"}))
			})

			It("parses content.markdown.goldmark options", func() {
				Expect(err).NotTo(HaveOccurred())
				Expect(cfg.Content.Markdown.Goldmark.UnsafeValue()).To(BeTrue())
				Expect(cfg.Content.Markdown.Goldmark.Typographer).To(BeTrue())
				Expect(cfg.Content.Markdown.Goldmark.TemplateTagsValue()).To(BeTrue())
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

			// Issue #302: Site-level permalinks removed. Permalink patterns
			// are set via _data.yaml cascade, not alloy.config.yaml.
			// The developer must:
			// 1. Remove Permalinks field from Config struct (compiler catches all callers)
			// 2. Remove permalinks from testdata/*.yaml/toml/json
			// 3. Config loader must not error on unknown keys (old configs with
			//    permalinks: should load without failing)

			It("parses pagination section with path", func() {
				Expect(err).NotTo(HaveOccurred())
				Expect(cfg.Pagination.Path).To(Equal("page"))
			})

			It("parses passthrough array with from/to", func() {
				Expect(err).NotTo(HaveOccurred())
				Expect(cfg.Passthrough).To(HaveLen(4))
				Expect(cfg.Passthrough[0].From).To(Equal("../design-system/dist/elements"))
				Expect(cfg.Passthrough[0].To).To(Equal("elements"))
				Expect(cfg.Passthrough[1].From).To(Equal("../shared-assets/fonts"))
				Expect(cfg.Passthrough[1].To).To(Equal("assets/fonts"))
			})

			It("parses passthrough exclude array (issue #547)", func() {
				Expect(err).NotTo(HaveOccurred())
				Expect(cfg.Passthrough).To(HaveLen(4),
					"valid.yaml must have 4 passthrough mappings (issue #547)")
				Expect(cfg.Passthrough[2].From).To(Equal("elements"))
				Expect(cfg.Passthrough[2].To).To(Equal("assets/packages/elements"))
				Expect(cfg.Passthrough[2].Exclude).To(Equal([]string{"*.html", "demo/"}),
					"exclude field must parse into a string slice (issue #547)")
			})

			It("parses passthrough glob from with exclude (issue #547)", func() {
				Expect(err).NotTo(HaveOccurred())
				Expect(cfg.Passthrough).To(HaveLen(4))
				Expect(cfg.Passthrough[3].From).To(Equal("elements/**/*.{js,css}"),
					"from field must preserve glob pattern with brace expansion as-is (issue #547)")
				Expect(cfg.Passthrough[3].To).To(Equal("assets/elements"))
				Expect(cfg.Passthrough[3].Exclude).To(Equal([]string{"*.min.js"}),
					"exclude on glob-from mapping must parse correctly (issue #547)")
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

			It("parses SSR config with command (exec mode default)", func() {
				Expect(err).NotTo(HaveOccurred())
				Expect(cfg.SSR).NotTo(BeNil())
				Expect(cfg.SSR.Command).To(Equal("golit render --defs bundles/"))
				Expect(cfg.SSR.Mode).To(BeEmpty(),
					"mode must be empty when omitted — defaults to exec at runtime")
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

		Context("SSR stream mode config", func() {
			It("parses ssr.mode and ssr.timeout from config file", func() {
				streamCfg, err := config.Load("testdata/valid_stream.yaml")
				Expect(err).NotTo(HaveOccurred())
				Expect(streamCfg).NotTo(BeNil())
				Expect(streamCfg.SSR).NotTo(BeNil())
				Expect(streamCfg.SSR.Command).To(Equal("golit serve --stdio"))
				Expect(streamCfg.SSR.Mode).To(Equal("stream"))
				Expect(streamCfg.SSR.Timeout).To(Equal("30s"))
			})
		})

		Context("validation", func() {
			It("returns error for invalid YAML", func() {
				_, err := config.Load("testdata/invalid.yaml")
				Expect(err).To(HaveOccurred())
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
			Expect(cfg.Content.Markdown.Goldmark.TemplateTagsValue()).To(BeTrue(),
				"TemplateTagsValue() must return true for minimal config — "+
					"omitted *bool defaults to true")
		})

		// ── GoldmarkConfig *bool tri-state semantics (issue #398) ────────
		// Omitted fields default to true. Explicit false is preserved.
		// These tests use the helper methods (UnsafeValue, etc.) which
		// return the effective value: nil → true, *false → false.

		It("omitted goldmark fields default to true via helper methods", func() {
			Expect(err).NotTo(HaveOccurred())
			Expect(cfg.Content.Markdown.Goldmark.UnsafeValue()).To(BeTrue(),
				"UnsafeValue() must return true when unsafe is omitted from config — "+
					"nil *bool defaults to true")
			Expect(cfg.Content.Markdown.Goldmark.TemplateTagsValue()).To(BeTrue(),
				"TemplateTagsValue() must return true when templateTags is omitted")
			Expect(cfg.Content.Markdown.Goldmark.AutoHeadingIDValue()).To(BeTrue(),
				"AutoHeadingIDValue() must return true when autoHeadingID is omitted")
		})

		It("defaults pagination.path to page", func() {
			Expect(err).NotTo(HaveOccurred())
			Expect(cfg.Pagination.Path).To(Equal("page"))
		})

		It("defaults plugins.timeout to 5000", func() {
			Expect(err).NotTo(HaveOccurred())
			Expect(cfg.Plugins.Timeout).To(Equal(5000))
		})

		It("defaults plugins.workers to auto (issue #491)", func() {
			Expect(err).NotTo(HaveOccurred())
			Expect(cfg.Plugins.Workers).To(Equal("auto"),
				"plugins.workers must default to 'auto' — "+
					"auto-scales subprocess worker count based on CPU")
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
			Expect(cfg.Build.CleanValue()).To(BeTrue(),
				"build.clean must default to true per spec")
		})

		It("defaults structure.content to content", func() {
			Expect(err).NotTo(HaveOccurred())
			Expect(cfg.Structure.Content).To(Equal("content"),
				"structure.content must default to 'content'")
		})

		It("defaults structure.plugins to plugins (issue #802)", func() {
			Expect(err).NotTo(HaveOccurred())
			Expect(cfg.Structure.Plugins).To(Equal("plugins"),
				"structure.plugins must default to 'plugins' — "+
					"same pattern as all other structure fields (issue #802)")
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

		It("normalizes engine \"go\" to \"gotemplate\" (issue #818)", func() {
			cfg := &config.Config{
				Templates: config.TemplatesConfig{Engine: "go"},
			}
			config.ApplyDefaults(cfg)
			Expect(cfg.Templates.Engine).To(Equal("gotemplate"),
				"\"go\" must be normalized to \"gotemplate\" — the short form is "+
					"intuitive but the engine checks only match \"gotemplate\"; without "+
					"normalization, \"go\" silently falls through to Liquid (issue #818)")
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
				Build: config.BuildConfig{Output: "_site", Clean: boolPtr(true)},
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
			Expect(cfg.Build.CleanValue()).To(BeTrue(),
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

	// ── Templates.Engine validation (issue #818) ─────────────────────
	// Unknown engine values must fail validation with a clear error.
	// Known values ("liquid", "gotemplate") must pass.

	Describe("Templates.Engine validation (issue #818)", func() {
		It("rejects unknown engine value", func() {
			cfg := &config.Config{
				Title:   "Test",
				BaseURL: "https://example.com",
				Templates: config.TemplatesConfig{Engine: "jinja"},
			}
			err := config.Validate(cfg)
			Expect(err).To(HaveOccurred(),
				"unknown engine \"jinja\" must fail validation — without this "+
					"guard, a typo silently falls through to Liquid at 4 comparison "+
					"sites, then fails with a confusing \"no layout found\" error "+
					"(issue #818)")
			Expect(err.Error()).To(ContainSubstring("jinja"),
				"error message must name the invalid value so the user knows what to fix")
		})

		It("accepts known engine values", func() {
			for _, engine := range []string{"liquid", "gotemplate"} {
				cfg := &config.Config{
					Title:   "Test",
					BaseURL: "https://example.com",
					Templates: config.TemplatesConfig{Engine: engine},
				}
				err := config.Validate(cfg)
				Expect(err).NotTo(HaveOccurred(),
					"engine %q is a known value and must pass validation", engine)
			}
		})
	})

	// ── Site-wide sitemap disable (issue #825) ───────────────────────
	// sitemap: false in config must disable generation entirely.
	// sitemap: {changefreq: ...} object form must keep working.
	// PLAN.md:505-508 specifies this behavior.

	Describe("Site-wide sitemap disable (issue #825)", func() {
		It("parses sitemap: false as disabled", func() {
			dir := GinkgoT().TempDir()
			configContent := `title: "Sitemap Disable Test"
baseURL: "https://example.com"
sitemap: false
`
			Expect(os.WriteFile(filepath.Join(dir, "alloy.config.yaml"), []byte(configContent), 0644)).To(Succeed())

			cfg, err := config.Load(filepath.Join(dir, "alloy.config.yaml"))
			Expect(err).NotTo(HaveOccurred(),
				"sitemap: false must not cause a YAML unmarshal error — "+
					"SitemapConfig needs a custom UnmarshalYAML to accept the "+
					"boolean form alongside the object form (issue #825)")
			Expect(cfg.Sitemap.Enabled).To(BeFalse(),
				"sitemap: false must set Enabled to false so the build "+
					"pipeline skips sitemap.xml generation")
		})

		It("parses sitemap object form with Enabled defaulting to true", func() {
			dir := GinkgoT().TempDir()
			configContent := `title: "Sitemap Object Test"
baseURL: "https://example.com"
sitemap:
  changefreq: "daily"
  priority: 0.8
`
			Expect(os.WriteFile(filepath.Join(dir, "alloy.config.yaml"), []byte(configContent), 0644)).To(Succeed())

			cfg, err := config.Load(filepath.Join(dir, "alloy.config.yaml"))
			Expect(err).NotTo(HaveOccurred())
			Expect(cfg.Sitemap.Enabled).To(BeTrue(),
				"sitemap object form must have Enabled=true — the boolean "+
					"form opts out, the object form opts in with settings")
			Expect(cfg.Sitemap.ChangeFreq).To(Equal("daily"))
			Expect(cfg.Sitemap.Priority).To(Equal(0.8))
		})

		It("defaults Sitemap.Enabled to true when sitemap is omitted", func() {
			cfg := &config.Config{
				Title:   "Test",
				BaseURL: "https://example.com",
			}
			config.ApplyDefaults(cfg)
			Expect(cfg.Sitemap.Enabled).To(BeTrue(),
				"when sitemap is not specified in config, ApplyDefaults must "+
					"set Enabled=true — sitemaps are on by default (issue #825)")
		})
	})

	// ── Configurable plugins directory (issue #802) ────────────────────
	// The plugins directory must be configurable via structure.plugins,
	// following the same pattern as content, layouts, assets, static, data.

	Describe("Structure.Plugins config (issue #802)", func() {
		It("parses structure.plugins from YAML config", func() {
			dir := GinkgoT().TempDir()
			configContent := `title: "Plugin Config Test"
baseURL: "https://example.com"
structure:
  plugins: "tools/plugins"
`
			Expect(os.WriteFile(filepath.Join(dir, "alloy.config.yaml"), []byte(configContent), 0644)).To(Succeed())

			cfg, err := config.Load(filepath.Join(dir, "alloy.config.yaml"))
			Expect(err).NotTo(HaveOccurred())
			Expect(cfg.Structure.Plugins).To(Equal("tools/plugins"),
				"structure.plugins must parse custom directory path from config (issue #802)")
		})

		It("rejects watch from: that overlaps configured plugins directory", func() {
			cfg := &config.Config{
				Title:   "Test",
				BaseURL: "https://example.com",
				Structure: config.StructureConfig{
					Plugins: "tools/plugins",
				},
				Watch: []config.WatchMapping{
					{From: "tools/plugins", Type: "content"},
				},
			}
			err := config.Validate(cfg)
			Expect(err).To(HaveOccurred(),
				"watch from: matching configured plugins directory must fail validation — "+
					"plugins is a managed directory and cannot be watched separately (issue #802)")
			Expect(err.Error()).To(ContainSubstring("tools/plugins"),
				"error must name the conflicting directory (issue #802)")
		})
	})

	// ── Watch config (issue #530) ────────────────────────────────────
	// The watch: config key registers extra directories for pipeline-
	// triggering file watching during serve mode. Unlike passthrough
	// (which triggers RebuildRecopy), watch dirs trigger RebuildPipeline.

	Describe("Watch config (issue #530)", func() {
		Context("YAML parsing", func() {
			It("parses watch array with from and type", func() {
				cfg, err := config.Load("testdata/valid.yaml")
				Expect(err).NotTo(HaveOccurred())
				Expect(cfg.Watch).To(HaveLen(2),
					"valid.yaml must include watch entries for testing (issue #530)")
				Expect(cfg.Watch[0].From).To(Equal("elements"),
					"watch[0].from must parse the directory path (issue #530)")
				Expect(cfg.Watch[0].Type).To(Equal("content"),
					"watch[0].type must parse the watch type (issue #530)")
				Expect(cfg.Watch[1].From).To(Equal("shared-layouts"),
					"watch[1].from must parse the second entry (issue #530)")
				Expect(cfg.Watch[1].Type).To(Equal("layout"),
					"watch[1].type must parse the second entry's type (issue #530)")
			})
		})

		Context("validation", func() {
			It("rejects watch entry with empty from", func() {
				cfg := &config.Config{
					Title:   "Test",
					BaseURL: "https://example.com",
					Watch:   []config.WatchMapping{{From: "", Type: "content"}},
				}
				err := config.Validate(cfg)
				Expect(err).To(HaveOccurred(),
					"watch entry with empty from must fail validation (issue #530)")
				Expect(err.Error()).To(ContainSubstring("from"),
					"error must mention the invalid field (issue #530)")
			})

			It("rejects watch entry with invalid type", func() {
				cfg := &config.Config{
					Title:   "Test",
					BaseURL: "https://example.com",
					Watch:   []config.WatchMapping{{From: "elements", Type: "invalid"}},
				}
				err := config.Validate(cfg)
				Expect(err).To(HaveOccurred(),
					"watch entry with invalid type must fail validation (issue #530)")
				Expect(err.Error()).To(ContainSubstring("type"),
					"error must mention the invalid field (issue #530)")
			})

			It("rejects watch entry with empty type", func() {
				cfg := &config.Config{
					Title:   "Test",
					BaseURL: "https://example.com",
					Watch:   []config.WatchMapping{{From: "elements", Type: ""}},
				}
				err := config.Validate(cfg)
				Expect(err).To(HaveOccurred(),
					"empty type does not match content/layout/data (issue #530)")
				Expect(err.Error()).To(ContainSubstring("type"),
					"error message must mention the invalid field (issue #530)")
			})

			It("accepts valid watch entries for all three types", func() {
				cfg := &config.Config{
					Title:   "Test",
					BaseURL: "https://example.com",
					Watch: []config.WatchMapping{
						{From: "elements", Type: "content"},
						{From: "shared-layouts", Type: "layout"},
						{From: "external-data", Type: "data"},
					},
				}
				err := config.Validate(cfg)
				Expect(err).NotTo(HaveOccurred(),
					"all three watch types must be valid (issue #530)")
			})

			It("accepts config with no watch entries", func() {
				cfg := &config.Config{
					Title:   "Test",
					BaseURL: "https://example.com",
				}
				err := config.Validate(cfg)
				Expect(err).NotTo(HaveOccurred(),
					"omitting watch entirely must be valid (issue #530)")
			})

			It("includes index in validation error for second entry", func() {
				cfg := &config.Config{
					Title:   "Test",
					BaseURL: "https://example.com",
					Watch: []config.WatchMapping{
						{From: "elements", Type: "content"},
						{From: "", Type: "data"},
					},
				}
				err := config.Validate(cfg)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("watch[1]"),
					"validation error must include array index (issue #530)")
			})

			It("rejects duplicate watch entries with same from", func() {
				cfg := &config.Config{
					Title:   "Test",
					BaseURL: "https://example.com",
					Watch: []config.WatchMapping{
						{From: "elements", Type: "content"},
						{From: "elements", Type: "layout"},
					},
				}
				err := config.Validate(cfg)
				Expect(err).To(HaveOccurred(),
					"duplicate watch from: paths create ambiguous classification — "+
						"which type wins? Must reject at validation time (issue #530)")
				Expect(err.Error()).To(ContainSubstring("elements"),
					"error must name the duplicate path (issue #530)")
			})

			It("rejects watch from: that overlaps a base structure directory", func() {
				cfg := &config.Config{
					Title:   "Test",
					BaseURL: "https://example.com",
					Watch: []config.WatchMapping{
						{From: "content", Type: "layout"},
					},
				}
				err := config.Validate(cfg)
				Expect(err).To(HaveOccurred(),
					"watch from: matching a base structure dir (content, layouts, data, "+
						"assets, static) creates conflicting classification — the base dir "+
						"already has a fixed ChangeType (issue #530)")
				Expect(err.Error()).To(ContainSubstring("content"),
					"error must name the conflicting directory (issue #530)")
			})

			It("rejects watch from: directory that does not exist", func() {
				cfg := &config.Config{
					Title:   "Test",
					BaseURL: "https://example.com",
					Watch: []config.WatchMapping{
						{From: "nonexistent-dir", Type: "content"},
					},
				}
				err := config.Validate(cfg)
				Expect(err).To(HaveOccurred(),
					"watch from: pointing to a nonexistent directory must fail "+
						"validation — fail fast with a clear error instead of "+
						"silently watching nothing (issue #530)")
				Expect(err.Error()).To(ContainSubstring("nonexistent-dir"),
					"error must name the missing directory (issue #530)")
			})

			It("normalizes trailing slash in watch from: path", func() {
				cfg := &config.Config{
					Title:   "Test",
					BaseURL: "https://example.com",
					Watch: []config.WatchMapping{
						{From: "elements/", Type: "content"},
					},
				}
				err := config.Validate(cfg)
				Expect(err).NotTo(HaveOccurred(),
					"trailing slash in from: must be accepted and normalized — "+
						"rejecting would punish a harmless typo (issue #530)")
			})
		})
	})

	// ── GoldmarkConfig explicit false (issue #398) ───────────────────
	// When goldmark options are explicitly set to false in config,
	// the *bool tri-state must preserve false — not override to true.

	Describe("GoldmarkConfig explicit false (issue #398)", func() {
		var cfg *config.Config
		var err error

		BeforeEach(func() {
			cfg, err = config.LoadWithDefaults("testdata/goldmark_false.yaml")
		})

		It("explicit unsafe: false is preserved", func() {
			Expect(err).NotTo(HaveOccurred())
			Expect(cfg.Content.Markdown.Goldmark.UnsafeValue()).To(BeFalse(),
				"UnsafeValue() must return false when unsafe is explicitly set to false — "+
					"ApplyDefaults must not overwrite explicit false with default true")
		})

		It("explicit templateTags: false is preserved", func() {
			Expect(err).NotTo(HaveOccurred())
			Expect(cfg.Content.Markdown.Goldmark.TemplateTagsValue()).To(BeFalse(),
				"TemplateTagsValue() must return false when templateTags is explicitly set to false")
		})

		It("explicit autoHeadingID: false is preserved", func() {
			Expect(err).NotTo(HaveOccurred())
			Expect(cfg.Content.Markdown.Goldmark.AutoHeadingIDValue()).To(BeFalse(),
				"AutoHeadingIDValue() must return false when autoHeadingID is explicitly set to false")
		})

		It("parses autoHeadingID field from config", func() {
			Expect(err).NotTo(HaveOccurred())
			Expect(cfg.Content.Markdown.Goldmark.AutoHeadingID).NotTo(BeNil(),
				"AutoHeadingID must be non-nil when explicitly set in config — "+
					"the *bool field must parse the YAML value, not remain nil")
		})
	})

	// ── BuildConfig explicit clean: false (issue #593) ──────────────
	// When build.clean is explicitly set to false in config,
	// the *bool tri-state must preserve false — not override to true.

	Describe("BuildConfig explicit clean: false (issue #593)", func() {
		var cfg *config.Config
		var err error

		BeforeEach(func() {
			cfg, err = config.LoadWithDefaults("testdata/clean_false.yaml")
		})

		It("explicit clean: false is preserved after LoadWithDefaults", func() {
			Expect(err).NotTo(HaveOccurred())
			Expect(cfg.Build.CleanValue()).To(BeFalse(),
				"CleanValue() must return false when clean is explicitly set to false — "+
					"ApplyDefaults must not overwrite explicit false with default true")
		})

		It("Clean field is non-nil when explicitly set", func() {
			Expect(err).NotTo(HaveOccurred())
			Expect(cfg.Build.Clean).NotTo(BeNil(),
				"Clean must be non-nil when explicitly set in config — "+
					"the *bool field must parse the YAML value, not remain nil")
		})
	})
})
