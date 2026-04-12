package output

import "github.com/zeroedin/alloy/internal/content"

// ResolveOutputFormat determines the output format for a page.
func ResolveOutputFormat(page *content.Page) string {
	return ""
}

// ResolveFormatLayout finds the layout file for a specific output format.
func ResolveFormatLayout(page *content.Page, format string, layoutsDir string, engine string) (string, error) {
	return "", ErrNotImplemented
}
