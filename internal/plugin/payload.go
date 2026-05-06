package plugin

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
	TOC         []TOCEntry             `json:"toc,omitempty"`
}

// TOCEntry replaces the dynamic map serialization in serializeTOC/deserializeTOC.
type TOCEntry struct {
	ID       string     `json:"id"`
	Text     string     `json:"text"`
	Level    int        `json:"level"`
	Children []TOCEntry `json:"children,omitempty"`
}

// HookPagesReadyPayload is the outbound payload for onPagesReady (per-batch).
type HookPagesReadyPayload struct {
	Pages    []HookPagePayload      `json:"pages"`
	SiteData map[string]interface{} `json:"siteData"`
}

// HookCascadePayload is the target outbound payload for onDataCascadeReady (per-page).
// Not yet wired — the pipeline currently sends []HookPagePayload to this hook.
// Defined here for spec compliance; will be wired when the cascade hook is refactored.
type HookCascadePayload struct {
	Path string                 `json:"path"`
	Data map[string]interface{} `json:"data"`
}

// HookAssetPayload is the target outbound payload for per-asset onAssetProcess dispatch.
// Not yet wired — the pipeline currently sends directory-level info to this hook.
// Defined here for spec compliance; will be wired when per-asset dispatch is implemented.
type HookAssetPayload struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}
