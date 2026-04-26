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
	Taxonomies  map[string]interface{}
	Content     string // rendered HTML body
	Pagination  *pagination.PaginationContext
	Custom      map[string]interface{} // dynamic top-level variables (e.g., pagination "as" alias)
}

// SiteContext holds site-wide data available as {{ site.* }}.
type SiteContext struct {
	Title        string
	BaseURL      string
	Language     string
	LanguageData map[string]interface{} // structured language data (code, title, root, strings)
	Data         map[string]interface{}
	Pages        []*content.Page
}

// PageContext holds per-page data available as {{ page.* }}.
type PageContext struct {
	Title        string
	URL          string
	Date         interface{}
	Collection   string
	FrontMatter  map[string]interface{}
	Translations []map[string]interface{} // translation info: url, lang
	TOC          []content.TOCEntry
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
	} else if langData, ok := siteData["language"].(map[string]interface{}); ok {
		ctx.Site.LanguageData = langData
		if code, ok := langData["code"].(string); ok {
			ctx.Site.Language = code
		}
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
	ctx.Page.TOC = page.TOC
	if !page.Date.IsZero() {
		ctx.Page.Date = page.Date
	}

	// Populate translations from linked pages
	if len(page.Translations) > 0 {
		translations := make([]map[string]interface{}, 0, len(page.Translations))
		for _, t := range page.Translations {
			info := map[string]interface{}{
				"url": t.URL,
			}
			if lang, ok := t.FrontMatter["lang"].(string); ok {
				info["lang"] = lang
			}
			if title, ok := t.FrontMatter["title"].(string); ok {
				info["title"] = title
			}
			translations = append(translations, info)
		}
		ctx.Page.Translations = translations
	}

	// Set content from rendered body
	if len(page.RenderedBody) > 0 {
		ctx.Content = string(page.RenderedBody)
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

// ToMap converts the TemplateContext to a raw map[string]interface{} for use
// with template engines (e.g., osteele/liquid) that require a plain map.
// The map shape matches the spec §3 context: page.*, site.*, collections.*,
// with pagination and custom "as" variables hoisted to the top level.
func (tc *TemplateContext) ToMap() map[string]interface{} {
	// Page context: start with all front matter, overlay computed fields.
	pageCtx := make(map[string]interface{}, len(tc.Page.FrontMatter)+5)
	for k, v := range tc.Page.FrontMatter {
		pageCtx[k] = v
	}
	if tc.Page.URL != "" {
		pageCtx["url"] = tc.Page.URL
	}
	if tc.Page.Date != nil {
		pageCtx["date"] = tc.Page.Date
	}
	if tc.Page.Collection != "" {
		pageCtx["collection"] = tc.Page.Collection
	}
	if tc.Content != "" {
		pageCtx["content"] = tc.Content
	}
	if tc.Page.Translations != nil {
		pageCtx["translations"] = tc.Page.Translations
	}
	if len(tc.Page.TOC) > 0 {
		tocList := make([]map[string]interface{}, len(tc.Page.TOC))
		for i, entry := range tc.Page.TOC {
			tocList[i] = tocEntryToMap(entry)
		}
		pageCtx["toc"] = tocList
	}

	// Site context
	site := map[string]interface{}{
		"title":   tc.Site.Title,
		"baseURL": tc.Site.BaseURL,
	}
	if tc.Site.LanguageData != nil {
		site["language"] = tc.Site.LanguageData
	} else if tc.Site.Language != "" {
		site["language"] = tc.Site.Language
	}
	if tc.Site.Data != nil {
		site["data"] = tc.Site.Data
	} else {
		site["data"] = make(map[string]interface{})
	}
	if tc.Site.Pages != nil {
		site["pages"] = tc.Site.Pages
	}
	if tc.Collections != nil {
		site["collections"] = tc.Collections
	}

	ctx := map[string]interface{}{
		"page": pageCtx,
		"site": site,
	}
	if tc.Collections != nil {
		ctx["collections"] = tc.Collections
	}
	if tc.Taxonomies != nil {
		ctx["taxonomies"] = tc.Taxonomies
	}

	// Hoist pagination to top level per spec §1c.
	// Two sources: explicit Pagination field (from BuildTemplateContext params)
	// or _pagination* transport keys in FrontMatter (from processPagination).
	if tc.Pagination != nil {
		ctx["pagination"] = map[string]interface{}{
			"pageNumber":   tc.Pagination.PageNumber,
			"totalPages":   tc.Pagination.TotalPages,
			"previousPage": tc.Pagination.PreviousPage,
			"nextPage":     tc.Pagination.NextPage,
			"first":        tc.Pagination.First,
			"last":         tc.Pagination.Last,
			"items":        tc.Pagination.Items,
		}
	} else if paginationCtx, ok := pageCtx["_paginationCtx"]; ok {
		ctx["pagination"] = paginationCtx
		delete(pageCtx, "_paginationCtx")
	}

	// Hoist custom "as" variables.
	for k, v := range tc.Custom {
		ctx[k] = v
	}
	if asVar, ok := pageCtx["_paginationAs"].(string); ok {
		if data, ok := pageCtx["_paginationData"]; ok {
			ctx[asVar] = data
		}
		delete(pageCtx, "_paginationAs")
		delete(pageCtx, "_paginationData")
	}

	return ctx
}

func tocEntryToMap(entry content.TOCEntry) map[string]interface{} {
	m := map[string]interface{}{
		"id":    entry.ID,
		"text":  entry.Text,
		"level": entry.Level,
	}
	if len(entry.Children) > 0 {
		children := make([]map[string]interface{}, len(entry.Children))
		for i, child := range entry.Children {
			children[i] = tocEntryToMap(child)
		}
		m["children"] = children
	} else {
		m["children"] = []map[string]interface{}{}
	}
	return m
}
