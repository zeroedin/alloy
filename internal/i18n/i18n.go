package i18n

import (
	"errors"

	"github.com/zeroedin/alloy/internal/config"
	"github.com/zeroedin/alloy/internal/content"
)

// ErrNotImplemented is returned by all stub functions.
var ErrNotImplemented = errors.New("not implemented")

// LanguageContext holds the active language state during a build.
type LanguageContext struct {
	Code    string
	Title   string
	Strings map[string]string
	Root    bool
}

// BuildLanguageContexts creates a LanguageContext for each declared language.
func BuildLanguageContexts(cfg map[string]*config.LanguageConfig) ([]LanguageContext, error) {
	return nil, ErrNotImplemented
}

// LinkTranslations connects pages across languages by matching relative paths.
func LinkTranslations(pages []*content.Page, languages []string) error {
	return ErrNotImplemented
}

// OutputPrefix returns the output directory prefix for a language.
func OutputPrefix(langCode string, isRoot bool) string {
	return ""
}

// ContentTreeRoute determines the content directory path for a language.
// Returns "content/en/" for language "en" (content tree routing).
func ContentTreeRoute(langCode string) string {
	return ""
}

// LanguageData returns the site.language data cascade entry for a language context.
func LanguageData(ctx LanguageContext) map[string]interface{} {
	return nil
}

// FilterByLanguage filters a page slice to only include pages for a given language.
func FilterByLanguage(pages []*content.Page, langCode string) []*content.Page {
	return nil
}

// BuildTaxonomiesForLanguage generates taxonomy pages scoped to a specific language.
func BuildTaxonomiesForLanguage(langCode string, pages []*content.Page) map[string]interface{} {
	return nil
}

// LanguageSiteTitle returns the site title for a specific language,
// using the language-specific override if available, falling back to global title.
func LanguageSiteTitle(globalTitle string, langCfg *config.LanguageConfig) string {
	return ""
}

// TranslationInfo holds URL and language code for a page translation.
type TranslationInfo struct {
	URL      string
	LangCode string
}

// GetTranslations returns translation info for a page's linked translations.
func GetTranslations(page *content.Page) []TranslationInfo {
	return nil
}
