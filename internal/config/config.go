package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
	"gopkg.in/yaml.v3"
)

// ErrNotImplemented is returned by all stub functions.
var ErrNotImplemented = errors.New("not implemented")

// Config represents the full alloy.config.yaml structure.
type Config struct {
	ProjectRoot string                       `yaml:"-" toml:"-" json:"-"` // set by Load; directory containing the config file
	Title       string                       `yaml:"title" toml:"title" json:"title"`
	BaseURL     string                       `yaml:"baseURL" toml:"baseURL" json:"baseURL"`
	Language    string                       `yaml:"language" toml:"language" json:"language"`
	Verbose     bool                         `yaml:"-" toml:"-" json:"-"` // CLI-only, set via MergeFlags
	Quiet       bool                         `yaml:"-" toml:"-" json:"-"` // CLI-only, set via MergeFlags
	Refetch     bool                         `yaml:"-" toml:"-" json:"-"` // CLI-only: --refetch bypasses fetch cache
	Build       BuildConfig                  `yaml:"build" toml:"build" json:"build"`
	Content     ContentConfig                `yaml:"content" toml:"content" json:"content"`
	Templates   TemplatesConfig              `yaml:"templates" toml:"templates" json:"templates"`
	Plugins     PluginsConfig                `yaml:"plugins" toml:"plugins" json:"plugins"`
	Taxonomies  map[string]*TaxonomyConfig   `yaml:"taxonomies" toml:"taxonomies" json:"taxonomies"`
	Permalinks  map[string]string            `yaml:"permalinks" toml:"permalinks" json:"permalinks"`
	Pagination  PaginationConfig             `yaml:"pagination" toml:"pagination" json:"pagination"`
	Passthrough []PassthroughMapping         `yaml:"passthrough" toml:"passthrough" json:"passthrough"`
	Sources     map[string]*SourceConfig     `yaml:"sources" toml:"sources" json:"sources"`
	Sitemap     SitemapConfig                `yaml:"sitemap" toml:"sitemap" json:"sitemap"`
	Structure   StructureConfig              `yaml:"structure" toml:"structure" json:"structure"`
	Languages   map[string]*LanguageConfig   `yaml:"languages" toml:"languages" json:"languages"`
	Collections map[string]*CollectionConfig `yaml:"collections" toml:"collections" json:"collections"`
	SSR         *SSRConfig                   `yaml:"ssr" toml:"ssr" json:"ssr"`
}

// BuildConfig holds output directory and clean settings.
type BuildConfig struct {
	Output string `yaml:"output" toml:"output" json:"output"`
	Clean  bool   `yaml:"clean" toml:"clean" json:"clean"`
}

// StructureConfig holds custom directory paths for project structure.
// All paths are relative to project root. When not specified, defaults apply.
type StructureConfig struct {
	Content string `yaml:"content" toml:"content" json:"content"` // default: "content"
	Layouts string `yaml:"layouts" toml:"layouts" json:"layouts"` // default: "layouts"
	Assets  string `yaml:"assets" toml:"assets" json:"assets"`   // default: "assets"
	Static  string `yaml:"static" toml:"static" json:"static"`   // default: "static"
	Data    string `yaml:"data" toml:"data" json:"data"`         // default: "data"
}

// ContentConfig holds content format and Markdown settings.
type ContentConfig struct {
	Formats  []string       `yaml:"formats" toml:"formats" json:"formats"`
	Markdown MarkdownConfig `yaml:"markdown" toml:"markdown" json:"markdown"`
}

// MarkdownConfig holds goldmark options.
type MarkdownConfig struct {
	Goldmark GoldmarkConfig `yaml:"goldmark" toml:"goldmark" json:"goldmark"`
}

// GoldmarkConfig holds goldmark-specific options.
type GoldmarkConfig struct {
	Unsafe       bool `yaml:"unsafe" toml:"unsafe" json:"unsafe"`
	Typographer  bool `yaml:"typographer" toml:"typographer" json:"typographer"`
	TemplateTags bool `yaml:"templateTags" toml:"templateTags" json:"templateTags"`
}

// TemplatesConfig holds the template engine selection.
type TemplatesConfig struct {
	Engine string `yaml:"engine" toml:"engine" json:"engine"`
}

// PluginsConfig holds plugin system settings.
type PluginsConfig struct {
	Node    bool `yaml:"node" toml:"node" json:"node"`
	Timeout int  `yaml:"timeout" toml:"timeout" json:"timeout"`
}

// TaxonomyConfig holds per-taxonomy settings.
type TaxonomyConfig struct {
	Permalink string `yaml:"permalink" toml:"permalink" json:"permalink"`
	Layout    string `yaml:"layout" toml:"layout" json:"layout"`
}

// PaginationConfig holds pagination path settings.
type PaginationConfig struct {
	Path string `yaml:"path" toml:"path" json:"path"`
}

// PassthroughMapping maps an external directory to an output path.
type PassthroughMapping struct {
	From string `yaml:"from" toml:"from" json:"from"`
	To   string `yaml:"to" toml:"to" json:"to"`
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
	ChangeFreq string  `yaml:"changefreq" toml:"changefreq" json:"changefreq"`
	Priority   float64 `yaml:"priority" toml:"priority" json:"priority"`
}

// FeedConfig holds RSS/Atom feed settings.
type FeedConfig struct {
	Collection string `yaml:"collection" toml:"collection" json:"collection"`
	Limit      int    `yaml:"limit" toml:"limit" json:"limit"`
	Title      string `yaml:"title" toml:"title" json:"title"`
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
	Build string         `yaml:"build" toml:"build" json:"build"`
	Serve SSRServeConfig `yaml:"serve" toml:"serve" json:"serve"`
}

// SSRServeConfig holds SSR serve-mode settings.
type SSRServeConfig struct {
	Cmd      string `yaml:"cmd" toml:"cmd" json:"cmd"`
	Endpoint string `yaml:"endpoint" toml:"endpoint" json:"endpoint"`
	Protocol string `yaml:"protocol" toml:"protocol" json:"protocol"`
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
		if err := json.Unmarshal(b, cfg); err != nil {
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
	if len(cfg.Content.Formats) == 0 {
		cfg.Content.Formats = []string{"md", "html"}
	}
	// TemplateTags defaults to true (zero value is false, so we apply on fresh configs)
	// We need a way to know if it was explicitly set. Since we can't distinguish,
	// for LoadWithDefaults we always set it if the whole markdown section is empty.
	// Actually the spec says default true, so we set it.
	cfg.Content.Markdown.Goldmark.TemplateTags = true
	if cfg.Pagination.Path == "" {
		cfg.Pagination.Path = "page"
	}
	if cfg.Plugins.Timeout == 0 {
		cfg.Plugins.Timeout = 5000
	}
	if cfg.Language == "" {
		cfg.Language = "en"
	}
	// Build.Clean defaults to true
	cfg.Build.Clean = true
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
	return nil
}
