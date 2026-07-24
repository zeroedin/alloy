package plugin

import "github.com/zeroedin/alloy/internal/content"

// HookPagePayload is the outbound representation of a page sent to plugins.
// Separate from the template data path (which uses map[string]interface{} for liquidgo).
type HookPagePayload struct {
	Path        string                 `json:"path"`
	URL         string                 `json:"url"`
	FrontMatter map[string]interface{} `json:"frontMatter"`
	Content     string                 `json:"content,omitempty"`
	HTML        string                 `json:"html,omitempty"`
}

// HookTransformPayload is the outbound payload for onContentTransformed (per-page).
type HookTransformPayload struct {
	Path        string                 `json:"path"`
	URL         string                 `json:"url"`
	FrontMatter map[string]interface{} `json:"frontMatter"`
	HTML        string                 `json:"html"`
	TOC         []content.TOCEntry     `json:"toc,omitempty"`
}

// HookRenderedPayload is the outbound payload for onPageRendered (per-page).
// Only the returned html field is applied back; frontMatter, url, and path are
// read-only context for conditional processing (issue #1095).
type HookRenderedPayload struct {
	HTML        string                 `json:"html"`
	FrontMatter map[string]interface{} `json:"frontMatter"`
	URL         string                 `json:"url"`
	Path        string                 `json:"path"`
}

// HookFormatRenderedPayload is the outbound payload for onFormatRendered (per-format body).
// Only the returned content field is applied back; format, frontMatter, url, and path are
// read-only context for conditional processing (issue #1102).
type HookFormatRenderedPayload struct {
	Format      string                 `json:"format"`
	Content     string                 `json:"content"`
	URL         string                 `json:"url"`
	Path        string                 `json:"path"`
	FrontMatter map[string]interface{} `json:"frontMatter"`
}

// HookPagesReadyPayload is the outbound payload for onPagesReady (per-batch).
type HookPagesReadyPayload struct {
	Pages    []HookPagePayload      `json:"pages"`
	SiteData map[string]interface{} `json:"siteData"`
}

// HookCascadePayload is one entry in the onDataCascadeReady batch payload.
type HookCascadePayload struct {
	Path string                 `json:"path"`
	Data map[string]interface{} `json:"data"`
}

// HookAssetPayload is the outbound payload for per-asset onAssetProcess dispatch.
// Sent once per asset file with the relative path and file content as a string.
// Return value's "content" key replaces the asset content written to the output directory.
type HookAssetPayload struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

// HookBuildCompletePayload is the trimmed onBuildComplete payload sent to plugins.
// Only includes stats + outputDir — no rendered HTML, cache, or site data (issue #1098).
type HookBuildCompletePayload struct {
	PageCount int      `json:"pageCount"`
	Duration  string   `json:"duration"`
	Errors    []string `json:"errors"`
	OutputDir string   `json:"outputDir"`
}
