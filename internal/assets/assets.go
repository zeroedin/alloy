package assets

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/zeroedin/alloy/internal/fileutil"
)

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
	_, err := os.Stat(assetsDir)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}

	return filepath.WalkDir(assetsDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		rel, err := filepath.Rel(assetsDir, path)
		if err != nil {
			return err
		}

		dest := filepath.Join(outputDir, rel)

		if d.IsDir() {
			return os.MkdirAll(dest, 0o755)
		}

		return fileutil.CopyFile(path, dest)
	})
}

// ProcessAssets copies files from assetsDir to outputDir, calling hookFn for
// each file before writing. The hook may modify the AssetFile content.
// If hookFn is nil, files are copied unchanged (same as CopyAssets).
func ProcessAssets(assetsDir, outputDir string, hookFn func(AssetFile) (AssetFile, error)) error {
	_, err := os.Stat(assetsDir)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}

	return filepath.WalkDir(assetsDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}

		rel, err := filepath.Rel(assetsDir, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)

		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}

		asset := AssetFile{
			Path:    rel,
			Content: content,
		}

		// Apply hook if provided — only content may change; path is fixed.
		if hookFn != nil {
			result, err := hookFn(asset)
			if err != nil {
				return err
			}
			asset.Content = result.Content
		}

		// Write to output using the original relative path (path changes in
		// hook return values are ignored per spec).
		dest := filepath.Join(outputDir, filepath.FromSlash(rel))
		dir := filepath.Dir(dest)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}

		return os.WriteFile(dest, asset.Content, 0o644)
	})
}

// ResolveURL resolves an asset path relative to baseURL.
func ResolveURL(path, baseURL string) string {
	baseURL = strings.TrimRight(baseURL, "/")
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	return baseURL + path
}
