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
	FormatBodies map[string][]byte // alternate format rendered bodies (e.g., "json" → rendered JSON)
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
	TOC          []TOCEntry
	renderedStr  string
}

// SetRenderedBody updates RenderedBody and clears the cached HTML string.
func (p *Page) SetRenderedBody(b []byte) {
	p.RenderedBody = b
	p.renderedStr = ""
}

// HTML returns the string conversion of RenderedBody, caching the result
// so subsequent calls avoid redundant []byte→string allocations.
func (p *Page) HTML() string {
	if p.renderedStr == "" && len(p.RenderedBody) > 0 {
		p.renderedStr = string(p.RenderedBody)
	}
	return p.renderedStr
}

// ToTemplateMap converts a Page to a map with lowercase keys for reliable
// Liquid template access. Struct fields (URL, Date, etc.) are merged on top
// of FrontMatter so templates can use {{ page.title }}, {{ page.url }}, etc.
func (p *Page) ToTemplateMap() map[string]interface{} {
	m := make(map[string]interface{}, len(p.FrontMatter)+6)
	for k, v := range p.FrontMatter {
		m[k] = v
	}
	if p.URL != "" {
		m["url"] = p.URL
	}
	if !p.Date.IsZero() {
		m["date"] = p.Date
	}
	if p.Summary != "" {
		m["summary"] = p.Summary
	}
	if p.Section != "" {
		m["collection"] = p.Section
	}
	if p.Slug != "" {
		m["slug"] = p.Slug
	}
	return m
}

// PaginationFrontMatter holds pagination settings from front matter.
type PaginationFrontMatter struct {
	Data    string `yaml:"data" json:"data"`
	PerPage int    `yaml:"perPage" json:"perPage"`
	As      string `yaml:"as" json:"as"`
}
