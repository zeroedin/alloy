package template

import (
	"github.com/zeroedin/alloy/internal/content"
	"github.com/zeroedin/alloy/internal/pagination"
)

// TemplateContext is the complete context object passed to templates during rendering.
type TemplateContext struct {
	Site        SiteContext
	Page        PageContext
	Collections map[string]interface{}
	Content     string // rendered HTML body
	Pagination  *pagination.PaginationContext
	Custom      map[string]interface{} // dynamic top-level variables (e.g., pagination "as" alias)
}

// SiteContext holds site-wide data available as {{ site.* }}.
type SiteContext struct {
	Title    string
	BaseURL  string
	Language string
	Data     map[string]interface{}
	Pages    []*content.Page
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
// For paginated pages, pass the PaginationContext and the "as" variable name.
// For non-paginated pages, pass nil and "".
func BuildTemplateContext(page *content.Page, siteData map[string]interface{}, allPages []*content.Page, collections map[string]interface{}, pagCtx *pagination.PaginationContext, asName string) *TemplateContext {
	ctx := &TemplateContext{
		Collections: collections,
	}
	ctx.Site.Pages = allPages

	// Populate site context
	if title, ok := siteData["title"].(string); ok {
		ctx.Site.Title = title
	}
	if baseURL, ok := siteData["baseURL"].(string); ok {
		ctx.Site.BaseURL = baseURL
	}
	if language, ok := siteData["language"].(string); ok {
		ctx.Site.Language = language
	}
	if data, ok := siteData["data"].(map[string]interface{}); ok {
		ctx.Site.Data = data
	}
	if ctx.Site.Data == nil {
		ctx.Site.Data = make(map[string]interface{})
	}

	// Populate page context
	if title, ok := page.FrontMatter["title"].(string); ok {
		ctx.Page.Title = title
	}
	ctx.Page.URL = page.URL
	ctx.Page.Collection = page.Collection
	ctx.Page.FrontMatter = page.FrontMatter
	if !page.Date.IsZero() {
		ctx.Page.Date = page.Date
	}

	// Set content from rendered body
	if len(page.RenderedBody) > 0 {
		ctx.Content = string(page.RenderedBody)
		ctx.Page.Content = string(page.RenderedBody)
	}

	// Inject pagination context and "as" alias for paginated pages
	if pagCtx != nil {
		ctx.Pagination = pagCtx
		if asName != "" {
			ctx.Custom = map[string]interface{}{
				asName: pagCtx.Items,
			}
		}
	}

	return ctx
}
