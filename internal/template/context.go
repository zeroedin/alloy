package template

import (
	"github.com/zeroedin/alloy/internal/content"
)

// TemplateContext is the complete context object passed to templates during rendering.
type TemplateContext struct {
	Site        SiteContext
	Page        PageContext
	Pages       []*content.Page
	Collections map[string]interface{}
	Content     string // rendered HTML body
}

// SiteContext holds site-wide data available as {{ site.* }}.
type SiteContext struct {
	Title    string
	BaseURL  string
	Language string
	Data     map[string]interface{}
}

// PageContext holds per-page data available as {{ page.* }}.
type PageContext struct {
	Title       string
	URL         string
	Content     string
	Date        interface{}
	Collection  string
	FrontMatter map[string]interface{}
}

// BuildTemplateContext assembles the complete template context for rendering a page.
func BuildTemplateContext(page *content.Page, siteData map[string]interface{}, allPages []*content.Page, collections map[string]interface{}) *TemplateContext {
	return nil
}
