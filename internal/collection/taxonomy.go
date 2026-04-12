package collection

import (
	"github.com/zeroedin/alloy/internal/config"
	"github.com/zeroedin/alloy/internal/content"
)

// TaxonomyCollection represents a taxonomy with its terms and pages.
type TaxonomyCollection struct {
	Name  string
	Terms map[string][]*content.Page
}

// TaxonomyTerm represents a single taxonomy term with metadata.
type TaxonomyTerm struct {
	Name  string
	Slug  string
	URL   string
	Items []*content.Page
}

// BuildTaxonomies creates taxonomy collections from page front matter.
func BuildTaxonomies(pages []*content.Page, taxonomyCfg map[string]*config.TaxonomyConfig) map[string]*TaxonomyCollection {
	return nil
}

// GenerateTaxonomyPages creates the virtual index and term pages for a taxonomy.
func GenerateTaxonomyPages(taxonomy *TaxonomyCollection, cfg *config.TaxonomyConfig) []*content.Page {
	return nil
}

// TaxonomyPageContext holds the template context for taxonomy page rendering.
// The template receives this as the "taxonomy" object.
type TaxonomyPageContext struct {
	// Term is the current term being rendered (empty for index pages).
	Term string
	// Terms is the list of all terms (for index pages).
	Terms []TaxonomyTerm
	// Items is the list of pages for the current term (for term pages).
	Items []*content.Page
}

// BuildTaxonomyPageContext creates the template context for a taxonomy page.
// For index pages (term=""), it provides Terms. For term pages, it provides Items.
func BuildTaxonomyPageContext(taxonomy *TaxonomyCollection, term string) *TaxonomyPageContext {
	return nil
}

// DetectDuplicateTermSlugs checks for taxonomy terms that produce the same
// URL slug from different source values (e.g., "C++" and "c--" both → "c").
func DetectDuplicateTermSlugs(taxonomy *TaxonomyCollection) []string {
	return nil
}
