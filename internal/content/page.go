package content

import (
	"sync/atomic"
	"time"
)

// Page represents a single content file with its metadata and rendered output.
type Page struct {
	SourcePath   string
	RelPath      string
	Content      []byte
	FrontMatter  map[string]interface{}
	Body         []byte
	BodyLine     int // 1-based line in Content where Body starts (front matter offset)
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
	Dependencies []string // nil = untracked (always re-render); non-nil = tracked source file deps
	renderedStr  string
}

// SetRenderedBody updates RenderedBody and clears the cached HTML string.
func (p *Page) SetRenderedBody(b []byte) {
	p.RenderedBody = b
	p.renderedStr = ""
}

// SetRenderedHTML stores both RenderedBody (as []byte) and the cached HTML
// string simultaneously, avoiding the string→[]byte→string round-trip that
// SetRenderedBody([]byte(html)) triggers. The pipeline uses this when applying
// hook results back, where the source is already a Go string from QuickJS
// GetPropertyStr().String() (issue #1185).
func (p *Page) SetRenderedHTML(s string) {
	p.RenderedBody = []byte(s)
	p.renderedStr = s
}

// ReleaseRenderedBody frees all rendered output data to reduce peak memory.
// Nils RenderedBody, clears the cached HTML string, and nils FormatBodies.
// Safe to call multiple times and on pages with nil fields (issue #1107).
func (p *Page) ReleaseRenderedBody() {
	p.RenderedBody = nil
	p.renderedStr = ""
	p.FormatBodies = nil
}

// HTML returns the string conversion of RenderedBody, caching the result
// so subsequent calls avoid redundant []byte→string allocations.
func (p *Page) HTML() string {
	if p.renderedStr == "" && len(p.RenderedBody) > 0 {
		p.renderedStr = string(p.RenderedBody)
	}
	return p.renderedStr
}

// TrustedHTML is HTML Alloy produced or vouches for: a page's rendered body
// and the summary in its own front matter. It is marked where the value is
// created, and the Go template engine prints only this type unescaped — a
// string is never trusted because of the key it sits under (issue #1267).
type TrustedHTML string

// ToLiquid hands Liquid a plain string. Liquid builds never see the marker
// (see MarkTrustedHTML); this covers a TrustedHTML reaching Liquid any other
// way, since liquidgo calls ToLiquid on lookup.
func (t TrustedHTML) ToLiquid() interface{} { return string(t) }

// trustMarkers is set for a build that renders with Go templates.
var trustMarkers atomic.Bool

// SetTrustMarkers turns TrustedHTML marking on for a build that renders with
// Go templates, and off for Liquid. liquidgo tests for string in property
// lookups ({{ page.summary.size }}) and filters such as sum, so a Liquid build
// gets the plain strings it always had (issue #1267).
func SetTrustMarkers(on bool) { trustMarkers.Store(on) }

// MarkTrustedHTML returns s as TrustedHTML in a Go-template build and as a
// plain string otherwise.
func MarkTrustedHTML(s string) interface{} {
	if trustMarkers.Load() {
		return TrustedHTML(s)
	}
	return s
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
	// Summary is the page's own; a summary inherited through the data
	// cascade stays a plain string and is escaped (issue #1267).
	if p.Summary != "" {
		m["summary"] = MarkTrustedHTML(p.Summary)
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
