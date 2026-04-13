package i18n

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/zeroedin/alloy/internal/config"
	"github.com/zeroedin/alloy/internal/content"
)

// LanguageContext holds the active language state during a build.
type LanguageContext struct {
	Code    string
	Title   string
	Strings map[string]string
	Root    bool
}

// BuildLanguageContexts creates a LanguageContext for each declared language.
// Returns contexts sorted by weight (lowest first = default language).
func BuildLanguageContexts(cfg map[string]*config.LanguageConfig) ([]LanguageContext, error) {
	if cfg == nil || len(cfg) == 0 {
		return nil, fmt.Errorf("no languages configured: at least one language must be declared")
	}

	contexts := make([]LanguageContext, 0, len(cfg))
	for code, langCfg := range cfg {
		contexts = append(contexts, LanguageContext{
			Code:    code,
			Title:   langCfg.Title,
			Strings: langCfg.Strings,
			Root:    langCfg.Root,
		})
	}

	// Sort by weight (lowest first)
	sort.Slice(contexts, func(i, j int) bool {
		wi := 0
		wj := 0
		if c, ok := cfg[contexts[i].Code]; ok {
			wi = c.Weight
		}
		if c, ok := cfg[contexts[j].Code]; ok {
			wj = c.Weight
		}
		return wi < wj
	})

	return contexts, nil
}

// LinkTranslations connects pages across languages by matching relative paths.
func LinkTranslations(pages []*content.Page, languages []string) error {
	// Group pages by RelPath
	byPath := make(map[string][]*content.Page)
	for _, page := range pages {
		byPath[page.RelPath] = append(byPath[page.RelPath], page)
	}

	// For each group with multiple pages, link them as translations
	for _, group := range byPath {
		if len(group) < 2 {
			continue
		}
		for i, page := range group {
			for j, other := range group {
				if i != j {
					page.Translations = append(page.Translations, other)
				}
			}
		}
	}

	return nil
}

// OutputPrefix returns the output directory prefix for a language.
// Root languages output at "/" (empty prefix); non-root get "lang/" prefix.
func OutputPrefix(langCode string, isRoot bool) string {
	if isRoot {
		return ""
	}
	return langCode + "/"
}

// ContentTreeRoute determines the content directory path for a language.
// Returns "content/<lang>/" for language-specific content trees.
func ContentTreeRoute(langCode string) string {
	return "content/" + langCode + "/"
}

// LanguageData returns the site.language data cascade entry for a language context.
func LanguageData(ctx LanguageContext) map[string]interface{} {
	data := map[string]interface{}{
		"code":  ctx.Code,
		"title": ctx.Title,
		"root":  ctx.Root,
	}
	if ctx.Strings != nil {
		data["strings"] = ctx.Strings
	}
	return data
}

// FilterByLanguage filters a page slice to only include pages for a given language.
func FilterByLanguage(pages []*content.Page, langCode string) []*content.Page {
	var result []*content.Page
	for _, page := range pages {
		if lang, ok := page.FrontMatter["lang"].(string); ok && lang == langCode {
			result = append(result, page)
		}
	}
	return result
}

// BuildTaxonomiesForLanguage generates taxonomy pages scoped to a specific language.
func BuildTaxonomiesForLanguage(langCode string, pages []*content.Page) map[string]interface{} {
	taxonomies := make(map[string]interface{})

	for _, page := range pages {
		fm := page.FrontMatter
		// Check common taxonomy fields (tags, categories)
		for _, field := range []string{"tags", "categories"} {
			if vals, ok := fm[field]; ok {
				if items, ok := vals.([]interface{}); ok {
					termMap, exists := taxonomies[field]
					if !exists {
						termMap = make(map[string][]*content.Page)
						taxonomies[field] = termMap
					}
					tm := termMap.(map[string][]*content.Page)
					for _, item := range items {
						if term, ok := item.(string); ok {
							tm[term] = append(tm[term], page)
						}
					}
				}
			}
		}
	}

	return taxonomies
}

// LanguageSiteTitle returns the site title for a specific language,
// using the language-specific override if available, falling back to global title.
func LanguageSiteTitle(globalTitle string, langCfg *config.LanguageConfig) string {
	if langCfg != nil && langCfg.Title != "" {
		return langCfg.Title
	}
	return globalTitle
}

// TranslationInfo holds URL and language code for a page translation.
type TranslationInfo struct {
	URL      string
	LangCode string
}

// GetTranslations returns translation info for a page's linked translations.
func GetTranslations(page *content.Page) []TranslationInfo {
	var translations []TranslationInfo
	for _, t := range page.Translations {
		langCode := ""
		if lang, ok := t.FrontMatter["lang"].(string); ok {
			langCode = lang
		} else {
			// Extract language code from path (e.g., "fr/about.md" → "fr")
			parts := strings.SplitN(filepath.ToSlash(t.RelPath), "/", 2)
			if len(parts) > 1 {
				langCode = parts[0]
			}
		}
		translations = append(translations, TranslationInfo{
			URL:      t.URL,
			LangCode: langCode,
		})
	}
	return translations
}
