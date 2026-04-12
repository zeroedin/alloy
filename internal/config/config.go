package config

import "errors"

// ErrNotImplemented is returned by all stub functions.
var ErrNotImplemented = errors.New("not implemented")

// Config represents the full alloy.config.yaml structure.
type Config struct {
	Title       string                       `yaml:"title" toml:"title" json:"title"`
	BaseURL     string                       `yaml:"baseURL" toml:"baseURL" json:"baseURL"`
	Language    string                       `yaml:"language" toml:"language" json:"language"`
	Verbose     bool                         `yaml:"-" toml:"-" json:"-"` // CLI-only, set via MergeFlags
	Quiet       bool                         `yaml:"-" toml:"-" json:"-"` // CLI-only, set via MergeFlags
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
	return nil, ErrNotImplemented
}

// LoadWithDefaults loads a config file and applies default values.
func LoadWithDefaults(path string) (*Config, error) {
	return nil, ErrNotImplemented
}

// DetectConfigFile finds the config file in the given directory.
func DetectConfigFile(dir string) (string, error) {
	return "", ErrNotImplemented
}

// MergeFlags merges CLI flag values into a loaded config.
// Flag values override config file values. Flags not present in the
// map leave config values unchanged.
func MergeFlags(cfg *Config, flags map[string]interface{}) {
	// stub — no-op
}

// Validate checks a loaded config for semantic errors: missing required
// fields, invalid values (e.g., negative timeout), and constraint
// violations. Returns nil if the config is valid.
func Validate(cfg *Config) error {
	return ErrNotImplemented
}
