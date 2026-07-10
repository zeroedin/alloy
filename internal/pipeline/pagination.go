package pipeline

import (
	"log"
	"strings"

	"github.com/zeroedin/alloy/internal/config"
	"github.com/zeroedin/alloy/internal/content"
	"github.com/zeroedin/alloy/internal/pagination"
	"github.com/zeroedin/alloy/internal/permalink"
	tmpl "github.com/zeroedin/alloy/internal/template"
)

// processPagination detects pages with pagination: front matter, resolves
// data sources, and generates virtual or paginated pages. Original paginated
// pages are replaced by their expanded set.
func processPagination(pages []*content.Page, cfg *config.Config, siteData map[string]interface{}, collectionsCtx map[string]interface{}, engine tmpl.TemplateEngine) []*content.Page {
	var result []*content.Page
	for _, page := range pages {
		paginationRaw, ok := page.FrontMatter["pagination"]
		if !ok {
			result = append(result, page)
			continue
		}
		paginationMap, ok := paginationRaw.(map[string]interface{})
		if !ok {
			result = append(result, page)
			continue
		}

		dataRef, _ := paginationMap["data"].(string)
		if dataRef == "" {
			result = append(result, page)
			continue
		}

		// Resolve data source — siteData is already the raw data map
		resolved, err := pagination.ResolveDataSource(dataRef, siteData, collectionsCtx)
		if err != nil {
			log.Printf("warning: pagination data source %q: %v", dataRef, err)
			result = append(result, page)
			continue
		}

		perPage := 1
		if pp, ok := paginationMap["perPage"].(int); ok && pp > 0 {
			perPage = pp
		} else if pp, ok := paginationMap["perPage"].(float64); ok && int(pp) > 0 {
			perPage = int(pp)
		}
		asVar, _ := paginationMap["as"].(string)
		if asVar == "" {
			asVar = "item"
		}

		// Check if the page has a template permalink (virtual page generation)
		permalinkStr, _ := page.FrontMatter["permalink"].(string)
		useTemplatePermalink := permalinkStr != "" && strings.Contains(permalinkStr, "{{")

		var contexts []pagination.PaginationContext
		var paths []string

		if useTemplatePermalink && perPage == 1 {
			var renderer pagination.TemplateRenderer
			if engine != nil {
				renderer = func(source string, ctx map[string]interface{}) (string, error) {
					// Convert *ordered.Map values to map[string]interface{}
					// so Go templates can access fields via .member.slug syntax.
					// Liquid handles *ordered.Map via LiquidMethodMissing but
					// Go templates require native map types.
					converted := convertOrderedMaps(ctx)
					tpl, err := engine.Parse("_permalink", []byte(source))
					if err != nil {
						return "", err
					}
					out, err := tpl.Render(converted)
					if err != nil {
						return "", err
					}
					return string(out), nil
				}
			} else {
				renderer = func(source string, ctx map[string]interface{}) (string, error) {
					return tmpl.RenderTemplate(source, "_permalink", ctx)
				}
			}
			contexts, paths, err = pagination.PaginateWithTemplatePermalink(resolved, permalinkStr, asVar, renderer)
		} else {
			basePath := page.URL
			if basePath == "" {
				basePath = permalink.DefaultFromPath(page.RelPath)
			}
			pathSegment := cfg.Pagination.Path
			contexts, paths, err = pagination.Paginate(resolved, perPage, basePath, pathSegment)
		}
		if err != nil {
			log.Printf("warning: pagination for %s: %v", page.RelPath, err)
			result = append(result, page)
			continue
		}

		// Generate virtual pages from pagination contexts
		for i, pctx := range contexts {
			vp := &content.Page{
				RelPath:     page.RelPath,
				Body:        page.Body,
				FrontMatter: copyFrontMatter(page.FrontMatter),
				Section:     page.Section,
				URL:         paths[i],
				Layout:      page.Layout,
				Kind:        "page",
			}
			// Store pagination context for top-level template injection.
			// Keys prefixed with "_pagination" are hoisted by buildTemplateContext
			// to the top level (not nested under page.*) per spec §1c.
			vp.FrontMatter["_paginationCtx"] = map[string]interface{}{
				"pageNumber":   pctx.PageNumber,
				"totalPages":   pctx.TotalPages,
				"previousPage": pctx.PreviousPage,
				"nextPage":     pctx.NextPage,
				"first":        pctx.First,
				"last":         pctx.Last,
				"items":        pctx.Items,
			}
			vp.FrontMatter["_paginationAs"] = asVar
			// Make the data items available under the 'as' variable name.
			// Convert *ordered.Map items to map[string]interface{} so both
			// Liquid and Go template engines can access fields via dot syntax.
			if perPage == 1 && len(pctx.Items) == 1 {
				item := convertOrderedValue(pctx.Items[0])
				vp.FrontMatter["_paginationData"] = item
				interpolateFrontMatter(vp, asVar, item, engine)
			} else {
				converted := make([]interface{}, len(pctx.Items))
				for ci, item := range pctx.Items {
					converted[ci] = convertOrderedValue(item)
				}
				vp.FrontMatter["_paginationData"] = converted
			}
			result = append(result, vp)
		}
	}
	return result
}

