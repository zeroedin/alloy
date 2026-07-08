package output

import (
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

