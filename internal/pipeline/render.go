package pipeline

import (
	"bytes"
	"fmt"
	"html"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/zeroedin/alloy/internal/config"
	"github.com/zeroedin/alloy/internal/content"
	"github.com/zeroedin/alloy/internal/i18n"
	"github.com/zeroedin/alloy/internal/pagination"
	tmpl "github.com/zeroedin/alloy/internal/template"
)

// renderPages renders all pages through the markdown and template pipeline.
// When engine is non-nil, it is used for template rendering (with custom filters).
// When engine is nil (incremental/SSR-on-demand paths), the standalone
// RenderTemplate is used with strict filters.
func renderPages(pages []*content.Page, rc *RenderContext, reporter ProgressReporter) ([]string, error) {
	cfg := rc.Cfg
	mdOpts := content.MarkdownOptions{
		Unsafe:         cfg.Content.Markdown.Goldmark.UnsafeValue(),
		Typographer:    cfg.Content.Markdown.Goldmark.Typographer,
		TemplateTags:   cfg.Content.Markdown.Goldmark.TemplateTagsValue(),
		AutoHeadingID:  cfg.Content.Markdown.Goldmark.AutoHeadingIDValue(),
		CustomElements: cfg.Content.Markdown.Goldmark.CustomElementsValue(),
	}

	layoutsDir := resolveDir(cfg.ProjectRoot, cfg.Structure.Layouts)
	engineName := cfg.Templates.Engine

	hooks, err := tmpl.DiscoverRenderHooks(layoutsDir, engineName)
	if err != nil {
		return nil, fmt.Errorf("render hook discovery: %w", err)
	}
	if len(hooks) > 0 && rc.Engine != nil {
		mdOpts.Hooks = hooks
		mdOpts.HookRenderer = func(source string, ctx map[string]interface{}) (string, error) {
			tpl, err := rc.Engine.Parse("_markup/hook", []byte(source))
			if err != nil {
				return "", err
			}
			out, err := tpl.Render(ctx)
			if err != nil {
				return "", err
			}
			return string(out), nil
		}
	}

	if rc.Goldmark == nil {
		mdOptsForGoldmark := mdOpts
		mdOptsForGoldmark.AutoHeadingID = true
		rc.Goldmark = content.CreateGoldmark(mdOptsForGoldmark)
	}
	md := rc.Goldmark

	var rendered []string

	for i, page := range pages {
		var pageStart time.Time
		if reporter != nil {
			pageStart = time.Now()
		}
		body := page.Body

		ext := filepath.Ext(page.RelPath)
		var html []byte
		var err error
		switch ext {
		case ".md":
			var toc []content.TOCEntry
			html, toc, err = content.RenderMarkdown(body, md)
			if err == nil && cfg.Content.Markdown.TOCValue() {
				page.TOC = toc
			}
		case ".txt":
			html, err = content.RenderText(body)
		default:
			html = body
		}
		if err != nil {
			return nil, fmt.Errorf("content transformation: %s: %w", page.RelPath, err)
		}

		if ext == ".md" {
			html = escapeTemplateTagsInCode(html)
		}

		if hasTemplateSyntax(html) {
			tc := tmpl.BuildTemplateContext(page, combinedSiteDataForPage(cfg, rc.SiteData, rc.LangContexts, page), rc.Pages, rc.CollectionsCtx, rc.TaxonomiesCtx, nil, "")
			tc.Content = string(html)
			ctx := tc.ToMap()
			if page.SourcePath != "" {
				ctx["_contentDir"] = filepath.Dir(page.SourcePath)
				contentRoot := resolveDir(cfg.ProjectRoot, cfg.Structure.Content)
				ctx["_contentRoot"] = contentRoot
			}
			if rc.Engine != nil {
				tpl, err := rc.Engine.Parse(page.RelPath, html)
				if err != nil {
					return nil, fmt.Errorf("template rendering: %s", err.Error())
				}
				rendered, err := tpl.Render(ctx)
				if err != nil {
					return nil, fmt.Errorf("template rendering: %s", err.Error())
				}
				html = rendered
			} else {
				result, err := tmpl.RenderTemplate(string(html), page.RelPath, ctx)
				if err != nil {
					return nil, fmt.Errorf("template rendering: %s", err.Error())
				}
				html = []byte(result)
			}
		}

		page.SetRenderedBody(html)
		rendered = append(rendered, page.RelPath)
		if reporter != nil {
			reportUpdate(reporter, i+1, page.RelPath, time.Since(pageStart))
		}
	}

	return rendered, nil
}

// renderedContentKey returns the key to use in RenderedContent for a page.
// Regular pages use RelPath; generated pages (taxonomy, paginated) use URL.
func renderedContentKey(page *content.Page) string {
	_, hasPaginationCtx := page.FrontMatter["_paginationCtx"]
	_, hasPagination := page.FrontMatter["pagination"]
	if (hasPaginationCtx || hasPagination) && page.URL != "" {
		return page.URL
	}
	if page.RelPath == "" && page.URL != "" {
		return page.URL
	}
	return page.RelPath
}

// copyFrontMatter creates a shallow copy of front matter.
func copyFrontMatter(fm map[string]interface{}) map[string]interface{} {
	result := make(map[string]interface{}, len(fm))
	for k, v := range fm {
		result[k] = v
	}
	return result
}

// interpolateFrontMatter resolves template tags in string-valued front matter
// fields for single-item paginated virtual pages.
func interpolateFrontMatter(vp *content.Page, asVar string, item interface{}, engine tmpl.TemplateEngine) {
	skipKeys := map[string]bool{
		"permalink": true, "layout": true, "pagination": true,
	}
	ctx := map[string]interface{}{asVar: item}
	for k, v := range vp.FrontMatter {
		s, ok := v.(string)
		if !ok || (!strings.Contains(s, "{{") && !strings.Contains(s, "{%")) {
			continue
		}
		if skipKeys[k] || strings.HasPrefix(k, "_pagination") {
			continue
		}
		if engine != nil {
			tpl, err := engine.Parse("_fm_"+k, []byte(s))
			if err != nil {
				log.Printf("warning: front matter interpolation for %s.%s: %v", vp.RelPath, k, err)
				continue
			}
			out, err := tpl.Render(ctx)
			if err != nil {
				log.Printf("warning: front matter interpolation for %s.%s: %v", vp.RelPath, k, err)
				continue
			}
			vp.FrontMatter[k] = html.UnescapeString(strings.TrimSpace(string(out)))
		} else {
			vp.FrontMatter[k] = strings.TrimSpace(pagination.RenderSimpleLiquid(s, asVar, item))
		}
	}
}

// codeBlockPattern matches <code> elements (including those with attributes).
// The non-greedy .*? matches to the first </code>, so nested <code> tags would
// not be handled correctly. This is fine because goldmark does not produce
// nested <code> elements — inline code and fenced code blocks each emit a
// single <code>…</code> pair.
var codeBlockPattern = regexp.MustCompile(`(?s)<code[^>]*>.*?</code>`)

// escapeTemplateTagsInCode replaces {{ }}, {% %} inside <code> elements with
// HTML entities so Liquid won't process them. This preserves template syntax
// examples in code fences for display purposes.
func escapeTemplateTagsInCode(html []byte) []byte {
	return codeBlockPattern.ReplaceAllFunc(html, func(match []byte) []byte {
		s := string(match)
		s = strings.ReplaceAll(s, "{{", "&#123;&#123;")
		s = strings.ReplaceAll(s, "}}", "&#125;&#125;")
		s = strings.ReplaceAll(s, "{%", "&#123;%")
		s = strings.ReplaceAll(s, "%}", "%&#125;")
		return []byte(s)
	})
}

// hasTemplateSyntax checks if content contains Liquid template tags.
func hasTemplateSyntax(body []byte) bool {
	return bytes.Contains(body, []byte("{{")) || bytes.Contains(body, []byte("{%"))
}

// combinedSiteData builds the site data map expected by BuildTemplateContext,
// combining config-level fields (title, baseURL) with data/ directory files.
func combinedSiteData(cfg *config.Config, siteData map[string]interface{}) map[string]interface{} {
	m := map[string]interface{}{
		"title":   cfg.Title,
		"baseURL": cfg.BaseURL,
	}
	if siteData != nil {
		m["data"] = siteData
	}
	return m
}

// combinedSiteDataForPage returns site data with language-specific overrides
// when i18n is active. Falls back to combinedSiteData for single-language builds.
func combinedSiteDataForPage(cfg *config.Config, siteData map[string]interface{}, langContexts []i18n.LanguageContext, page *content.Page) map[string]interface{} {
	m := combinedSiteData(cfg, siteData)
	if len(langContexts) == 0 || page == nil {
		return m
	}
	langCode, _ := page.FrontMatter["lang"].(string)
	if langCode == "" {
		return m
	}
	for _, lc := range langContexts {
		if lc.Code == langCode {
			m["language"] = i18n.LanguageData(lc)
			m["title"] = i18n.LanguageSiteTitle(cfg.Title, cfg.Languages[langCode])
			break
		}
	}
	return m
}

// formatOutputPath computes the output path for a non-HTML format by replacing
// the .html extension with the format extension (e.g., "blog/post/index.json").
func formatOutputPath(htmlPath string, format string) string {
	return strings.TrimSuffix(htmlPath, ".html") + "." + format
}

// renderPageFormats renders additional output formats for a page (spec §1e).
// For each non-HTML format in page.Outputs, resolves a format-specific layout,
// renders through it, and stores the result in page.FormatBodies.
// Returns a build error if a declared format has no matching layout.
func renderPageFormats(page *content.Page, layoutsDir, engineName string, rc *RenderContext) error {
	if len(page.Outputs) <= 1 {
		return nil
	}
	page.FormatBodies = make(map[string][]byte)
	for _, format := range page.Outputs {
		if format == "html" {
			continue
		}
		fmtLayoutPath, err := tmpl.ResolveLayoutForFormat(page, layoutsDir, engineName, format)
		if err != nil {
			return fmt.Errorf("no %s layout found for %s: %w", format, page.RelPath, err)
		}
		fmtTpl, err := parseLayout(fmtLayoutPath, rc.Engine, rc.LayoutCache)
		if err != nil {
			return fmt.Errorf("format layout: %w", err)
		}
		fmtCtx := tmpl.BuildTemplateContext(page, combinedSiteDataForPage(rc.Cfg, rc.SiteData, rc.LangContexts, page), rc.Pages, rc.CollectionsCtx, rc.TaxonomiesCtx, nil, "").ToMap()
		fmtCtx["content"] = page.HTML()
		fmtResult, err := fmtTpl.Render(fmtCtx)
		if err != nil {
			return fmt.Errorf("rendering format layout %s: %w", fmtLayoutPath, err)
		}
		page.FormatBodies[format] = fmtResult
	}
	return nil
}

// renderPageThroughLayouts resolves the layout chain for a page, strips front
// matter from each layout, and renders inside-out. Updates page.RenderedBody
// and tracks all layouts in templateUsage for cache invalidation.
func renderPageThroughLayouts(page *content.Page, layoutPath, layoutsDir, engineName string, rc *RenderContext) error {
	chain, err := tmpl.ResolveLayoutChain(layoutPath, layoutsDir, engineName)
	if err != nil {
		return fmt.Errorf("layout chain for %s: %w", page.RelPath, err)
	}

	if rc.TemplateUsage != nil {
		for _, lp := range chain {
			trackedLayout := filepath.ToSlash(filepath.Clean(lp))
			if relLayout, relErr := filepath.Rel(rc.Cfg.ProjectRoot, lp); relErr == nil {
				trackedLayout = filepath.ToSlash(relLayout)
			}
			rc.TemplateUsage[page.RelPath] = append(rc.TemplateUsage[page.RelPath], trackedLayout)
		}
	}

	for _, lp := range chain {
		tpl, err := parseLayout(lp, rc.Engine, rc.LayoutCache)
		if err != nil {
			return err
		}

		tc := tmpl.BuildTemplateContext(page, combinedSiteDataForPage(rc.Cfg, rc.SiteData, rc.LangContexts, page), rc.Pages, rc.CollectionsCtx, rc.TaxonomiesCtx, nil, "")
		ctx := tc.ToMap()
		ctx["content"] = page.HTML()
		layoutResult, err := tpl.Render(ctx)
		if err != nil {
			return fmt.Errorf("rendering layout %s: %w", lp, err)
		}
		page.SetRenderedBody(layoutResult)
	}

	return nil
}

func parseLayout(path string, engine tmpl.TemplateEngine, layoutCache map[string]tmpl.Template) (tmpl.Template, error) {
	if layoutCache != nil {
		if cached, ok := layoutCache[path]; ok {
			return cached, nil
		}
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading layout %s: %w", path, err)
	}
	stripped := tmpl.StripLayoutFrontMatter(string(content))
	tpl, err := engine.Parse(path, []byte(stripped))
	if err != nil {
		return nil, fmt.Errorf("parsing layout %s: %w", path, err)
	}
	if layoutCache != nil {
		layoutCache[path] = tpl
	}
	return tpl, nil
}
