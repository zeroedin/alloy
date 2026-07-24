package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
	"gopkg.in/yaml.v3"
)

// Config represents the full alloy.config.yaml structure.
type Config struct {
	ProjectRoot   string                       `yaml:"-" toml:"-" json:"-"` // set by Load; directory containing the config file
	Title         string                       `yaml:"title" toml:"title" json:"title"`
	BaseURL       string                       `yaml:"baseURL" toml:"baseURL" json:"baseURL"`
	Language      string                       `yaml:"language" toml:"language" json:"language"`
	Verbose       bool                         `yaml:"-" toml:"-" json:"-"` // CLI-only, set via MergeFlags
	Quiet         bool                         `yaml:"-" toml:"-" json:"-"` // CLI-only, set via MergeFlags
	Refetch       bool                         `yaml:"-" toml:"-" json:"-"` // CLI-only: --refetch bypasses fetch cache
	IncludeDrafts bool                         `yaml:"-" toml:"-" json:"-"` // CLI-only: dev server includes drafts
	Data          DataConfig                   `yaml:"data" toml:"data" json:"data"`
	Build         BuildConfig                  `yaml:"build" toml:"build" json:"build"`
	Content       ContentConfig                `yaml:"content" toml:"content" json:"content"`
	Templates     TemplatesConfig              `yaml:"templates" toml:"templates" json:"templates"`
	Plugins       PluginsConfig                `yaml:"plugins" toml:"plugins" json:"plugins"`
	Taxonomies    map[string]*TaxonomyConfig   `yaml:"taxonomies" toml:"taxonomies" json:"taxonomies"`
	Pagination    PaginationConfig             `yaml:"pagination" toml:"pagination" json:"pagination"`
	Passthrough   []PassthroughMapping         `yaml:"passthrough" toml:"passthrough" json:"passthrough"`
	Watch         []WatchMapping               `yaml:"watch" toml:"watch" json:"watch"`
	Sources       map[string]*SourceConfig     `yaml:"sources" toml:"sources" json:"sources"`
	Sitemap       SitemapConfig                `yaml:"sitemap" toml:"sitemap" json:"sitemap"`
	Structure     StructureConfig              `yaml:"structure" toml:"structure" json:"structure"`
	Languages     map[string]*LanguageConfig   `yaml:"languages" toml:"languages" json:"languages"`
	Collections   map[string]*CollectionConfig `yaml:"collections" toml:"collections" json:"collections"`
	SSR           *SSRConfig                   `yaml:"ssr" toml:"ssr" json:"ssr"`
	UpdateCheck   *bool                        `yaml:"updateCheck" toml:"updateCheck" json:"updateCheck"`
}

// UpdateCheckValue returns the effective UpdateCheck setting.
// nil (omitted) defaults to false — no outbound network request
// without explicit opt-in. Only explicit true enables update checking.
// This is the inverse of other *bool config fields (which default to true).
func (c *Config) UpdateCheckValue() bool {
	return c.UpdateCheck != nil && *c.UpdateCheck
}

// BuildConfig holds output directory and clean settings.
type BuildConfig struct {
	Output string `yaml:"output" toml:"output" json:"output"`
	Clean  *bool  `yaml:"clean" toml:"clean" json:"clean"`
}

// CleanValue returns the effective Clean setting.
// nil (omitted) defaults to true; only explicit false disables.
func (b *BuildConfig) CleanValue() bool {
	return b.Clean == nil || *b.Clean
}

// DataConfig holds external data file mappings.
// Files maps a data key to a file path relative to the project root.
type DataConfig struct {
	Files map[string]string `yaml:"files" toml:"files" json:"files"`
}

// StructureConfig holds custom directory paths for project structure.
// All paths are relative to project root. When not specified, defaults apply.
type StructureConfig struct {
	Content string `yaml:"content" toml:"content" json:"content"` // default: "content"
	Layouts string `yaml:"layouts" toml:"layouts" json:"layouts"` // default: "layouts"
	Assets  string `yaml:"assets" toml:"assets" json:"assets"`    // default: "assets"
	Static  string `yaml:"static" toml:"static" json:"static"`    // default: "static"
	Data    string `yaml:"data" toml:"data" json:"data"`          // default: "data"
	Plugins    string `yaml:"plugins" toml:"plugins" json:"plugins"`       // default: "plugins"
	Components string `yaml:"components" toml:"components" json:"components"` // default: "components"
}

// ContentConfig holds content format and Markdown settings.
type ContentConfig struct {
	Formats  []string       `yaml:"formats" toml:"formats" json:"formats"`
	Markdown MarkdownConfig `yaml:"markdown" toml:"markdown" json:"markdown"`
}

// MarkdownConfig holds goldmark options and Alloy-level markdown settings.
// TOC uses *bool so ApplyDefaults can distinguish "not set" (nil → true)
// from "explicitly false".
type MarkdownConfig struct {
	TOC      *bool          `yaml:"toc" toml:"toc" json:"toc"`
	Goldmark GoldmarkConfig `yaml:"goldmark" toml:"goldmark" json:"goldmark"`
}

// TOCValue returns the effective TOC setting.
// nil (omitted) defaults to true; only explicit false disables.
func (m *MarkdownConfig) TOCValue() bool {
	return m.TOC == nil || *m.TOC
}

// GoldmarkConfig holds goldmark-specific options.
// Boolean fields that default to true use *bool so ApplyDefaults
// can distinguish "not set" (nil → true) from "explicitly false".
type GoldmarkConfig struct {
	Unsafe         *bool `yaml:"unsafe" toml:"unsafe" json:"unsafe"`
	Typographer    bool  `yaml:"typographer" toml:"typographer" json:"typographer"`
	TemplateTags   *bool `yaml:"templateTags" toml:"templateTags" json:"templateTags"`
	AutoHeadingID  *bool `yaml:"autoHeadingID" toml:"autoHeadingID" json:"autoHeadingID"`
	CustomElements *bool `yaml:"customElements" toml:"customElements" json:"customElements"`
}

// UnsafeValue returns the effective Unsafe setting.
// nil (omitted) defaults to true; only explicit false disables.
func (g *GoldmarkConfig) UnsafeValue() bool {
	return g.Unsafe == nil || *g.Unsafe
}

// TemplateTagsValue returns the effective TemplateTags setting.
// nil (omitted) defaults to true; only explicit false disables.
func (g *GoldmarkConfig) TemplateTagsValue() bool {
	return g.TemplateTags == nil || *g.TemplateTags
}

// AutoHeadingIDValue returns the effective AutoHeadingID setting.
// nil (omitted) defaults to true; only explicit false disables.
func (g *GoldmarkConfig) AutoHeadingIDValue() bool {
	return g.AutoHeadingID == nil || *g.AutoHeadingID
}

// CustomElementsValue returns the effective CustomElements setting.
// nil (omitted) defaults to true; only explicit false disables.
func (g *GoldmarkConfig) CustomElementsValue() bool {
	return g.CustomElements == nil || *g.CustomElements
}

// TemplatesConfig holds the template engine selection.
type TemplatesConfig struct {
	Engine string `yaml:"engine" toml:"engine" json:"engine"`
}

// PluginsConfig holds plugin system settings.
type PluginsConfig struct {
	Node    bool        `yaml:"node" toml:"node" json:"node"`
	Timeout int         `yaml:"timeout" toml:"timeout" json:"timeout"`
	Workers interface{} `yaml:"workers" toml:"workers" json:"workers"` // "auto" (default) or int
}

// TaxonomyConfig holds per-taxonomy settings.
type TaxonomyConfig struct {
	Permalink string `yaml:"permalink" toml:"permalink" json:"permalink"`
	Layout    string `yaml:"layout" toml:"layout" json:"layout"`
	Render    *bool  `yaml:"render" toml:"render" json:"render"`
}

// ShouldRender returns whether taxonomy pages should be generated.
// nil (omitted) and true both mean render; only explicit false suppresses.
func (tc *TaxonomyConfig) ShouldRender() bool {
	return tc.Render == nil || *tc.Render
}

// PaginationConfig holds pagination path settings.
type PaginationConfig struct {
	Path string `yaml:"path" toml:"path" json:"path"`
}

// PassthroughMapping maps an external path (directory, file, or glob pattern) to an output path.
type PassthroughMapping struct {
	From    string   `yaml:"from" toml:"from" json:"from"`
	To      string   `yaml:"to" toml:"to" json:"to"`
	Exclude []string `yaml:"exclude" toml:"exclude" json:"exclude"`
}

// WatchMapping registers an external directory for pipeline-triggering
// file watching during serve mode.
type WatchMapping struct {
	From string `yaml:"from" toml:"from" json:"from"`
	Type string `yaml:"type" toml:"type" json:"type"`
}

// SourceConfig holds external data source settings.
type SourceConfig struct {
	Type     string `yaml:"type" toml:"type" json:"type"`
	URL      string `yaml:"url" toml:"url" json:"url"`
	Endpoint string `yaml:"endpoint" toml:"endpoint" json:"endpoint"`
	Query    string `yaml:"query" toml:"query" json:"query"`
	Plugin   string `yaml:"plugin" toml:"plugin" json:"plugin"`
	Cache    int    `yaml:"cache" toml:"cache" json:"cache"`
	As       string `yaml:"as" toml:"as" json:"as"`
}

// SitemapConfig holds sitemap generation settings.
type SitemapConfig struct {
	Enabled    bool    `yaml:"-" toml:"-" json:"-"`
	enabledSet bool    // true when Enabled was explicitly set (by UnmarshalYAML or caller)
	ChangeFreq string  `yaml:"changefreq" toml:"changefreq" json:"changefreq"`
	Priority   float64 `yaml:"priority" toml:"priority" json:"priority"`
}

// UnmarshalYAML accepts both `sitemap: false` (boolean) and
// `sitemap: {changefreq: ..., priority: ...}` (object) forms.
func (s *SitemapConfig) UnmarshalYAML(unmarshal func(interface{}) error) error {
	var boolVal bool
	if err := unmarshal(&boolVal); err == nil {
		s.Enabled = boolVal
		s.enabledSet = true
		return nil
	}
	type plain SitemapConfig
	var p plain
	if err := unmarshal(&p); err != nil {
		return err
	}
	*s = SitemapConfig(p)
	s.Enabled = true
	s.enabledSet = true
	return nil
}

// LanguageConfig holds per-language settings for i18n.
type LanguageConfig struct {
	Title   string            `yaml:"title" toml:"title" json:"title"`
	Weight  int               `yaml:"weight" toml:"weight" json:"weight"`
	Root    bool              `yaml:"root" toml:"root" json:"root"`
	Strings map[string]string `yaml:"strings" toml:"strings" json:"strings"`
}

// CollectionConfig holds per-collection sort settings.
type CollectionConfig struct {
	SortBy string `yaml:"sortBy" toml:"sortBy" json:"sortBy"`
	Order  string `yaml:"order" toml:"order" json:"order"`
}

// SSRConfig holds SSR engine settings.
type SSRConfig struct {
	Command string `yaml:"command" toml:"command" json:"command"`
	Mode    string `yaml:"mode" toml:"mode" json:"mode"`
	Timeout string `yaml:"timeout" toml:"timeout" json:"timeout"`
}

// Load reads and parses a config file at the given path.
func Load(path string) (*Config, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	cfg := &Config{}
	ext := strings.ToLower(filepath.Ext(path))

	switch ext {
	case ".yaml", ".yml":
		if err := yaml.Unmarshal(b, cfg); err != nil {
			return nil, fmt.Errorf("parsing config YAML: %w", err)
		}
	case ".toml":
		if err := toml.Unmarshal(b, cfg); err != nil {
			return nil, fmt.Errorf("parsing config TOML: %w", err)
		}
	case ".json":
		if err := jsonCodec.Unmarshal(b, cfg); err != nil {
			return nil, fmt.Errorf("parsing config JSON: %w", err)
		}
	default:
		return nil, fmt.Errorf("unsupported config file format: %s", ext)
	}

	absPath, err := filepath.Abs(path)
	if err == nil {
		cfg.ProjectRoot = filepath.Dir(absPath)
	}

	return cfg, nil
}

// LoadWithDefaults loads a config file and applies default values.
func LoadWithDefaults(path string) (*Config, error) {
	cfg, err := Load(path)
	if err != nil {
		return nil, err
	}
	ApplyDefaults(cfg)
	return cfg, nil
}

// ApplyDefaults sets default values on a Config struct.
// Exported so that callers (e.g. pipeline.Build) that receive a Config
// without going through LoadWithDefaults can still apply canonical defaults.
func ApplyDefaults(cfg *Config) {
	if cfg.Build.Output == "" {
		cfg.Build.Output = "_site"
	}
	if cfg.Templates.Engine == "" {
		cfg.Templates.Engine = "liquid"
	}
	if cfg.Templates.Engine == "go" {
		cfg.Templates.Engine = "gotemplate"
	}
	if len(cfg.Content.Formats) == 0 {
		cfg.Content.Formats = []string{"md", "html"}
	}
	// Unsafe, TemplateTags, and AutoHeadingID default to true via *bool
	// nil semantics — no overwrite needed here.
	if cfg.Pagination.Path == "" {
		cfg.Pagination.Path = "page"
	}
	if cfg.Plugins.Timeout == 0 {
		cfg.Plugins.Timeout = 5000
	}
	if cfg.Plugins.Workers == nil {
		cfg.Plugins.Workers = "auto"
	}
	if cfg.Language == "" {
		cfg.Language = "en"
	}
	// Build.Clean defaults to true via *bool (nil = true)
	if cfg.Structure.Content == "" {
		cfg.Structure.Content = "content"
	}
	if cfg.Structure.Layouts == "" {
		cfg.Structure.Layouts = "layouts"
	}
	if cfg.Structure.Assets == "" {
		cfg.Structure.Assets = "assets"
	}
	if cfg.Structure.Static == "" {
		cfg.Structure.Static = "static"
	}
	if cfg.Structure.Data == "" {
		cfg.Structure.Data = "data"
	}
	if cfg.Structure.Plugins == "" {
		cfg.Structure.Plugins = "plugins"
	}
	if cfg.Structure.Components == "" {
		cfg.Structure.Components = "components"
	}
	if !cfg.Sitemap.enabledSet {
		cfg.Sitemap.Enabled = true
	}
	// Replace nil TaxonomyConfig entries with zero-value structs.
	// YAML `tags:` with no value produces a nil *TaxonomyConfig pointer;
	// downstream code (GenerateTaxonomyPages, ResolveTaxonomyLayout)
	// dereferences the pointer without nil checks, causing a panic.
	// Render defaults to true via ShouldRender() (nil *bool = true).
	for name, tc := range cfg.Taxonomies {
		if tc == nil {
			cfg.Taxonomies[name] = &TaxonomyConfig{}
		}
	}
}

// DetectConfigFile finds the config file in the given directory.
func DetectConfigFile(dir string) (string, error) {
	candidates := []string{
		"alloy.config.yaml",
		"alloy.config.yml",
		"alloy.config.toml",
		"alloy.config.json",
	}
	for _, name := range candidates {
		path := filepath.Join(dir, name)
		if _, err := os.Stat(path); err == nil {
			return path, nil
		}
	}
	return "", fmt.Errorf("no config file found in %s (expected alloy.config.yaml, .yml, .toml, or .json)", dir)
}

// MergeFlags merges CLI flag values into a loaded config.
// Flag values override config file values. Flags not present in the
// map leave config values unchanged.
func MergeFlags(cfg *Config, flags map[string]interface{}) {
	if v, ok := flags["output"]; ok {
		if s, ok := v.(string); ok {
			cfg.Build.Output = s
		}
	}
	if v, ok := flags["verbose"]; ok {
		if b, ok := v.(bool); ok {
			cfg.Verbose = b
		}
	}
	if v, ok := flags["quiet"]; ok {
		if b, ok := v.(bool); ok {
			cfg.Quiet = b
		}
	}
	if v, ok := flags["refetch"]; ok {
		if b, ok := v.(bool); ok {
			cfg.Refetch = b
		}
	}
	if v, ok := flags["root"]; ok {
		if s, ok := v.(string); ok && s != "" {
			abs, err := filepath.Abs(s)
			if err == nil {
				cfg.ProjectRoot = abs
			}
		}
	}
}

// Validate checks a loaded config for semantic errors: missing required
// fields, invalid values (e.g., negative timeout), and constraint
// violations. Returns nil if the config is valid.
func Validate(cfg *Config) error {
	if cfg.Title == "" {
		return fmt.Errorf("validation error: title must not be empty")
	}
	if cfg.BaseURL == "" {
		return fmt.Errorf("validation error: baseURL must not be empty")
	}
	if !strings.HasPrefix(cfg.BaseURL, "http://") && !strings.HasPrefix(cfg.BaseURL, "https://") {
		return fmt.Errorf("validation error: baseURL must be a valid URL (starts with http:// or https://)")
	}
	if cfg.Plugins.Timeout < 0 {
		return fmt.Errorf("validation error: plugins timeout must not be negative (got %d)", cfg.Plugins.Timeout)
	}
	switch cfg.Templates.Engine {
	case "liquid", "gotemplate", "":
	default:
		return fmt.Errorf("validation error: unknown templates.engine %q (expected \"liquid\" or \"gotemplate\")", cfg.Templates.Engine)
	}

	structDir := func(configured, fallback string) string {
		if configured != "" {
			return strings.TrimRight(configured, "/\\")
		}
		return fallback
	}
	baseDirs := map[string]bool{
		structDir(cfg.Structure.Content, "content"): true,
		structDir(cfg.Structure.Layouts, "layouts"): true,
		structDir(cfg.Structure.Data, "data"):       true,
		structDir(cfg.Structure.Assets, "assets"):   true,
		structDir(cfg.Structure.Static, "static"):   true,
		structDir(cfg.Structure.Plugins, "plugins"):       true,
		structDir(cfg.Structure.Components, "components"): true,
	}
	seen := make(map[string]bool)
	for i := range cfg.Watch {
		from := strings.TrimRight(cfg.Watch[i].From, "/\\")
		if from == "" {
			return fmt.Errorf("validation error: watch[%d].from must not be empty", i)
		}

		switch cfg.Watch[i].Type {
		case "content", "layout", "data":
		default:
			return fmt.Errorf("validation error: watch[%d].type must be content, layout, or data", i)
		}

		if seen[from] {
			return fmt.Errorf("validation error: duplicate watch from: %q", from)
		}
		seen[from] = true

		for baseDir := range baseDirs {
			if from == baseDir || strings.HasPrefix(from, baseDir+"/") || strings.HasPrefix(from, baseDir+"\\") {
				return fmt.Errorf("validation error: watch from: %q overlaps base structure directory %q", from, baseDir)
			}
		}

		isGlob := strings.ContainsAny(from, "*?[{")
		statTarget := from
		if isGlob {
			if idx := strings.IndexAny(from, "*?[{"); idx > 0 {
				statTarget = filepath.Dir(from[:idx])
			} else {
				return fmt.Errorf("validation error: watch[%d].from %q has no directory prefix before glob — would watch the entire project root", i, from)
			}
		}
		if statTarget != "" && statTarget != "." {
			statPath := statTarget
			if cfg.ProjectRoot != "" {
				statPath = filepath.Join(cfg.ProjectRoot, statTarget)
			}
			info, err := os.Stat(statPath)
			if err != nil {
				if os.IsNotExist(err) {
					if isGlob {
						return fmt.Errorf("validation error: watch from: %q glob root %q does not exist", from, statTarget)
					}
					return fmt.Errorf("validation error: watch from: %q directory does not exist", from)
				}
				return fmt.Errorf("validation error: watch from: %q: %w", from, err)
			}
			if !info.IsDir() {
				if isGlob {
					return fmt.Errorf("validation error: watch from: %q glob root %q is not a directory", from, statTarget)
				}
				return fmt.Errorf("validation error: watch from: %q is not a directory", from)
			}
		}
	}

	return nil
}
