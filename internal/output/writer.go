package output

import "errors"

// ErrNotImplemented is returned by all stub functions.
var ErrNotImplemented = errors.New("not implemented")

// WriteFile writes content to the output directory at the given relative path.
func WriteFile(outputDir, relPath string, content []byte) error {
	return ErrNotImplemented
}

// CleanOutputDir removes all files from the output directory.
func CleanOutputDir(dir string) error {
	return ErrNotImplemented
}

// ComputeOutputPath computes the output file path for a given permalink.
// Pretty URLs: /about/ → about/index.html, /blog/post/ → blog/post/index.html
func ComputeOutputPath(permalink string) string {
	return ""
}

// WritePageFormats writes a page's content in multiple output formats.
// For example, a page with outputs: ["html", "json"] produces both
// about/index.html and about/index.json.
func WritePageFormats(outputDir string, permalink string, formats map[string][]byte) error {
	return ErrNotImplemented
}

// ShouldWrite returns false for pages with permalink: false (data-only pages
// that should be processed but not written to disk).
func ShouldWrite(permalink string) bool {
	return false
}

// WriteAliases writes the same rendered content to all alias output paths.
// These are additional output locations, not redirects.
func WriteAliases(outputDir string, aliases []string, content []byte) error {
	return ErrNotImplemented
}
