package content

import "time"

// Page represents a single content file with its metadata and rendered output.
type Page struct {
	SourcePath   string
	RelPath      string
	Content      []byte
	FrontMatter  map[string]interface{}
	Body         []byte
	RenderedBody []byte
	OutputPath   string
	URL          string
	Date         time.Time
	Draft        bool
	PublishDate  *time.Time
	ExpiryDate   *time.Time
	Summary      string
	Layout       string
	Outputs      []string
	Aliases      []string
	Permalink    string
	Slug         string
	Section      string
	Kind         string // "page", "taxonomy", "taxonomy_term", "home", "section"
	Collection   string
	Translations []*Page
	Pagination   *PaginationFrontMatter
	Bundle       bool
	BundleAssets []string
}

// PaginationFrontMatter holds pagination settings from front matter.
type PaginationFrontMatter struct {
	Data    string `yaml:"data" json:"data"`
	PerPage int    `yaml:"perPage" json:"perPage"`
	As      string `yaml:"as" json:"as"`
}
