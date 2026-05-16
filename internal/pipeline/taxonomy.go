package pipeline

import (
	"fmt"
	"strings"

	"github.com/zeroedin/alloy/internal/collection"
	"github.com/zeroedin/alloy/internal/content"
	"github.com/zeroedin/alloy/internal/i18n"
	tmpl "github.com/zeroedin/alloy/internal/template"
)

func generateTaxonomyPages(taxonomies map[string]*collection.TaxonomyCollection, layoutsDir, engineName string, rc *RenderContext, langCtx *i18n.LanguageContext) ([]*content.Page, error) {
	cfg := rc.Cfg
	for taxName, tc := range taxonomies {
		taxCfg := cfg.Taxonomies[taxName]
		if taxCfg != nil && !taxCfg.ShouldRender() {
			continue
		}
		dupes := collection.DetectDuplicateTermSlugs(tc)
		if len(dupes) > 0 {
			return nil, fmt.Errorf("taxonomy %q has duplicate term slugs: %v", taxName, dupes)
		}
	}

	var urlPrefix string
	var langCode string
	if langCtx != nil {
		urlPrefix = i18n.OutputPrefix(langCtx.Code, langCtx.Root)
		langCode = langCtx.Code
	}

	var result []*content.Page
	for taxName, tc := range taxonomies {
		taxCfg := cfg.Taxonomies[taxName]
		if !taxCfg.ShouldRender() {
			continue
		}
		layoutPath, err := tmpl.ResolveTaxonomyLayout(taxName, taxCfg.Layout, layoutsDir, engineName)
		if err != nil {
			return nil, fmt.Errorf("taxonomy %q layout: %w", taxName, err)
		}
		tpl, err := parseLayout(layoutPath, rc.Engine, rc.LayoutCache)
		if err != nil {
			return nil, fmt.Errorf("taxonomy %q layout: %w", taxName, err)
		}

		taxPages := collection.GenerateTaxonomyPages(tc, taxCfg)
		for _, taxPage := range taxPages {
			if langCode != "" {
				taxPage.FrontMatter["lang"] = langCode
				taxPage.URL = "/" + urlPrefix + strings.TrimPrefix(taxPage.URL, "/")
			}

			ctx := tmpl.BuildTemplateContext(taxPage, combinedSiteDataForPage(cfg, rc.SiteData, rc.LangContexts, taxPage), rc.Pages, rc.CollectionsCtx, rc.TaxonomiesCtx, nil, "").ToMap()
			term := ""
			if taxPage.Kind == "taxonomy_term" {
				if t, ok := taxPage.FrontMatter["title"].(string); ok {
					term = t
				}
			}
			ctx["taxonomy"] = collection.BuildTaxonomyPageContext(tc, term).ToMap()
			ctx["content"] = ""
			out, err := tpl.Render(ctx)
			if err != nil {
				return nil, fmt.Errorf("rendering taxonomy page %s: %w", taxPage.URL, err)
			}
			taxPage.SetRenderedBody(out)
			if err := renderPageFormats(taxPage, layoutsDir, engineName, rc); err != nil {
				return nil, err
			}
			result = append(result, taxPage)
		}
	}
	return result, nil
}
