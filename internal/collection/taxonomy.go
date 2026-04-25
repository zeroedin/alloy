package collection

import (
	"regexp"
	"sort"
	"strings"
	"unicode"

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
	Pages []*content.Page
}

// BuildTaxonomies creates taxonomy collections from page front matter.
// Only taxonomy keys declared in taxonomyCfg are collected.
func BuildTaxonomies(pages []*content.Page, taxonomyCfg map[string]*config.TaxonomyConfig) map[string]*TaxonomyCollection {
	taxonomies := make(map[string]*TaxonomyCollection)

	for taxName := range taxonomyCfg {
		tc := &TaxonomyCollection{
			Name:  taxName,
			Terms: make(map[string][]*content.Page),
		}

		for _, page := range pages {
			vals, ok := page.FrontMatter[taxName]
			if !ok {
				continue
			}
			arr, ok := vals.([]interface{})
			if !ok {
				continue
			}
			for _, v := range arr {
				term, ok := v.(string)
				if !ok {
					continue
				}
				tc.Terms[term] = append(tc.Terms[term], page)
			}
		}

		taxonomies[taxName] = tc
	}

	return taxonomies
}

// GenerateTaxonomyPages creates the virtual index and term pages for a taxonomy.
func GenerateTaxonomyPages(taxonomy *TaxonomyCollection, cfg *config.TaxonomyConfig) []*content.Page {
	if !cfg.Render {
		return nil
	}

	var pages []*content.Page

	// Determine permalink pattern
	permalinkPattern := cfg.Permalink
	if permalinkPattern == "" {
		// Default: /<taxonomy-name>/:slug/
		permalinkPattern = "/" + taxonomy.Name + "/:slug/"
	}

	// Index page at /<taxonomy-name>/
	indexURL := "/" + taxonomy.Name + "/"
	indexPage := &content.Page{
		URL:  indexURL,
		Kind: "taxonomy",
		FrontMatter: map[string]interface{}{
			"title": taxonomy.Name,
		},
	}
	if cfg.Layout != "" {
		indexPage.Layout = cfg.Layout
	}
	pages = append(pages, indexPage)

	// Per-term pages
	for term, termPages := range taxonomy.Terms {
		termSlug := slugify(term)
		termURL := strings.ReplaceAll(permalinkPattern, ":slug", termSlug)

		termPage := &content.Page{
			URL:  termURL,
			Kind: "taxonomy_term",
			FrontMatter: map[string]interface{}{
				"title": term,
			},
		}
		if cfg.Layout != "" {
			termPage.Layout = cfg.Layout
		}
		_ = termPages // pages are accessible via BuildTaxonomyPageContext
		pages = append(pages, termPage)
	}

	return pages
}

// TaxonomyPageContext holds the template context for taxonomy page rendering.
// The template receives this as the "taxonomy" object.
type TaxonomyPageContext struct {
	// Term is the current term being rendered (empty for index pages).
	Term string
	// Terms is the list of all terms (for index pages).
	Terms []TaxonomyTerm
	// Pages is the list of pages for the current term (for term pages).
	Pages []*content.Page
}

// ToMap converts the context to a map with lowercase keys for reliable
// template access (avoids liquidgo acronym-mapping issues with struct fields).
func (ctx *TaxonomyPageContext) ToMap() map[string]interface{} {
	m := map[string]interface{}{
		"term": ctx.Term,
	}
	if ctx.Terms != nil {
		terms := make([]map[string]interface{}, len(ctx.Terms))
		for i, t := range ctx.Terms {
			termPages := make([]interface{}, len(t.Pages))
			for j, p := range t.Pages {
				termPages[j] = p.ToTemplateMap()
			}
			terms[i] = map[string]interface{}{
				"name":  t.Name,
				"slug":  t.Slug,
				"url":   t.URL,
				"pages": termPages,
			}
		}
		m["terms"] = terms
	}
	if ctx.Pages != nil {
		ctxPages := make([]interface{}, len(ctx.Pages))
		for i, p := range ctx.Pages {
			ctxPages[i] = p.ToTemplateMap()
		}
		m["pages"] = ctxPages
	}
	return m
}

// BuildTaxonomyPageContext creates the template context for a taxonomy page.
// For index pages (term=""), it provides Terms. For term pages, it provides Pages.
func BuildTaxonomyPageContext(taxonomy *TaxonomyCollection, term string) *TaxonomyPageContext {
	if term == "" {
		// Index page: provide all terms
		var terms []TaxonomyTerm
		for termName, termPages := range taxonomy.Terms {
			terms = append(terms, TaxonomyTerm{
				Name:  termName,
				Slug:  slugify(termName),
				URL:   "/" + taxonomy.Name + "/" + slugify(termName) + "/",
				Pages: termPages,
			})
		}
		// Sort terms by name for deterministic output
		sort.Slice(terms, func(i, j int) bool {
			return terms[i].Name < terms[j].Name
		})
		return &TaxonomyPageContext{
			Term:  "",
			Terms: terms,
		}
	}

	// Term page: provide pages for the specific term
	termPages := taxonomy.Terms[term]
	return &TaxonomyPageContext{
		Term:  term,
		Pages: termPages,
	}
}

// DetectDuplicateTermSlugs checks for taxonomy terms that produce the same
// URL slug from different source values (e.g., "C++" and "c--" both → "c").
// Also detects terms whose slugs are prefixes of each other, which would
// cause URL routing conflicts.
func DetectDuplicateTermSlugs(taxonomy *TaxonomyCollection) []string {
	// Collect all term slugs
	type termSlug struct {
		term string
		slug string
	}
	var slugs []termSlug
	for term := range taxonomy.Terms {
		slugs = append(slugs, termSlug{term: term, slug: slugify(term)})
	}

	// Check for exact duplicates
	slugToTerms := make(map[string][]string)
	for _, ts := range slugs {
		slugToTerms[ts.slug] = append(slugToTerms[ts.slug], ts.term)
	}

	duplicateSet := make(map[string]bool)
	for slug, terms := range slugToTerms {
		if len(terms) > 1 {
			duplicateSet[slug] = true
		}
	}

	// Check for prefix collisions: if slug A is a prefix of slug B,
	// the generated URLs could conflict in routing
	for i := 0; i < len(slugs); i++ {
		for j := i + 1; j < len(slugs); j++ {
			a, b := slugs[i].slug, slugs[j].slug
			if a != b && (strings.HasPrefix(b, a) || strings.HasPrefix(a, b)) {
				// Report the shorter slug as the conflicting one
				if len(a) < len(b) {
					duplicateSet[a] = true
				} else {
					duplicateSet[b] = true
				}
			}
		}
	}

	var duplicates []string
	for slug := range duplicateSet {
		duplicates = append(duplicates, slug)
	}

	return duplicates
}

// nonAlphaNum matches any character that is not alphanumeric or a hyphen.
var nonAlphaNumTax = regexp.MustCompile(`-{2,}`)

// slugify converts a string to a URL-safe slug.
func slugify(s string) string {
	s = strings.ToLower(s)
	s = strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '-' || r == ' ' {
			return r
		}
		return -1
	}, s)
	s = strings.ReplaceAll(s, " ", "-")
	s = nonAlphaNumTax.ReplaceAllString(s, "-")
	s = strings.Trim(s, "-")
	return s
}
