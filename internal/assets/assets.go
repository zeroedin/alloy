package assets

import "errors"

// ErrNotImplemented is returned by all stub functions.
var ErrNotImplemented = errors.New("not implemented")

// AssetFile represents a single asset file passed through the onAssetProcess
// hook. Plugins receive this struct and may modify Content before it is written.
type AssetFile struct {
	Path    string // Relative path within the assets directory (e.g., "css/main.css")
	Content []byte // Raw file content
}

// CopyAssets copies all files from assetsDir to outputDir, preserving directory
// structure. No transformation is applied — files are copied verbatim.
// Returns nil without error if assetsDir does not exist.
func CopyAssets(assetsDir, outputDir string) error {
	return ErrNotImplemented
}

// ProcessAssets copies files from assetsDir to outputDir, calling hookFn for
// each file before writing. The hook may modify the AssetFile content.
// If hookFn is nil, files are copied unchanged (same as CopyAssets).
func ProcessAssets(assetsDir, outputDir string, hookFn func(AssetFile) (AssetFile, error)) error {
	return ErrNotImplemented
}

// ResolveURL resolves an asset path relative to baseURL.
// Used by the `url` template filter: {{ 'css/main.css' | url }}
func ResolveURL(path, baseURL string) string {
	return ""
}
