package output

// ResolveFeedTemplates scans the layouts directory for feed templates.
// Template placement determines the feed's output path and context:
//   - layouts/feed.xml → /feed.xml (site-wide feed)
//   - layouts/blog/feed.xml → /blog/feed.xml (section feed)
//   - layouts/taxonomies/tags/feed.xml → /tags/:slug/feed.xml (per-term)
func ResolveFeedTemplates(layoutsDir string) ([]FeedTemplate, error) {
	return nil, ErrNotImplemented
}

// FeedTemplate represents a discovered feed template and its output context.
type FeedTemplate struct {
	TemplatePath string // Path to the feed template file
	OutputPath   string // Computed output path
	Section      string // Section scope (empty = site-wide)
	Taxonomy     string // Taxonomy scope (empty = not taxonomy)
	Term         string // Taxonomy term (empty = all terms)
}

// RenderFeedTemplate renders a feed template with the appropriate context.
// Site-wide feeds receive all pages; section feeds receive section pages;
// per-term feeds receive pages tagged with that term.
func RenderFeedTemplate(tmpl FeedTemplate, context map[string]interface{}) ([]byte, error) {
	return nil, ErrNotImplemented
}
