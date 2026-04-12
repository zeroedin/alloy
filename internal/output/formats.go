package output

import (
	"path/filepath"

	"github.com/zeroedin/alloy/internal/content"
)

// ResolveOutputFormat determines the output format for a page.
// Returns "html" as default; if the page has explicit outputs, returns the first one.
func ResolveOutputFormat(page *content.Page) string {
	if len(page.Outputs) > 0 {
		return page.Outputs[0]
	}
	return "html"
}

// ResolveFormatLayout finds the layout file for a specific output format.
// Lookup: layouts/<layout>.<format>.<engine-ext> → layouts/<layout>.<format>
func ResolveFormatLayout(page *content.Page, format string, layoutsDir string, engine string) (string, error) {
	ext := ".liquid"
	if engine == "gotemplate" {
		ext = ".html"
	}

	layout := page.Layout
	if layout == "" {
		layout = "single"
	}

	// Primary: <layout>.<format>.<engine-ext>
	primary := filepath.Join(layoutsDir, layout+"."+format+ext)
	return primary, nil
}
