package template

import "github.com/zeroedin/alloy/internal/content"

// ResolveLayout finds the correct layout file for a page following the lookup order.
func ResolveLayout(page *content.Page, layoutsDir string, engine string, permalinkCfg map[string]string) (string, error) {
	return "", ErrNotImplemented
}

// ResolveLayoutForFormat finds the correct layout for a specific output format.
// For example, a page requesting "json" output with the Liquid engine looks for
// "layouts/single.json.liquid" first, then "layouts/single.json".
func ResolveLayoutForFormat(page *content.Page, layoutsDir string, engine string, format string) (string, error) {
	return "", ErrNotImplemented
}

// ResolveTaxonomyLayout finds the layout for a taxonomy page following the
// taxonomy-specific lookup order: layouts/taxonomies/<name>.liquid → layouts/<name>.liquid.
// Returns an error if no layout is found (build must abort).
func ResolveTaxonomyLayout(taxonomyName string, layoutOverride string, layoutsDir string, engine string) (string, error) {
	return "", ErrNotImplemented
}

// DetectCircularLayouts checks for circular references in layout chains.
// Returns an error naming the cycle if one is found.
func DetectCircularLayouts(layoutsDir string) error {
	return ErrNotImplemented
}

// ResolveLayoutWithCascade resolves the layout for a page, considering both
// front matter and _data.yaml cascade data. Front matter takes priority.
func ResolveLayoutWithCascade(page *content.Page, layoutsDir, engine string, permalinkCfg map[string]string, cascadeData map[string]interface{}) (string, error) {
	return "", ErrNotImplemented
}
