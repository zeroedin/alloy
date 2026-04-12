package output

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
)

// ErrNotImplemented is returned by all stub functions.
var ErrNotImplemented = errors.New("not implemented")

// WriteFile writes content to the output directory at the given relative path.
func WriteFile(outputDir, relPath string, content []byte) error {
	fullPath := filepath.Join(outputDir, relPath)

	// Create intermediate directories
	dir := filepath.Dir(fullPath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}

	return os.WriteFile(fullPath, content, 0o644)
}

// CleanOutputDir removes all files from the output directory.
func CleanOutputDir(dir string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		path := filepath.Join(dir, entry.Name())
		if err := os.RemoveAll(path); err != nil {
			return err
		}
	}
	return nil
}

// ComputeOutputPath computes the output file path for a given permalink.
// Pretty URLs: /about/ → about/index.html, / → index.html
func ComputeOutputPath(permalink string) string {
	// Remove leading slash
	p := strings.TrimPrefix(permalink, "/")

	// Root
	if p == "" {
		return "index.html"
	}

	// Trailing slash = directory with index.html
	if strings.HasSuffix(p, "/") {
		return p + "index.html"
	}

	// Already has extension
	if filepath.Ext(p) != "" {
		return p
	}

	return p + "/index.html"
}

// WritePageFormats writes a page's content in multiple output formats.
func WritePageFormats(outputDir string, permalink string, formats map[string][]byte) error {
	basePath := ComputeOutputPath(permalink)

	for format, content := range formats {
		outputPath := basePath
		if format != "html" {
			// Replace .html extension with the format extension
			outputPath = strings.TrimSuffix(basePath, ".html") + "." + format
		}
		if err := WriteFile(outputDir, outputPath, content); err != nil {
			return err
		}
	}

	return nil
}

// ShouldWrite returns false for pages with permalink: false (data-only pages
// that should be processed but not written to disk).
func ShouldWrite(permalink string) bool {
	return permalink != ""
}

// WriteAliases writes the same rendered content to all alias output paths.
func WriteAliases(outputDir string, aliases []string, content []byte) error {
	for _, alias := range aliases {
		outputPath := ComputeOutputPath(alias)
		if err := WriteFile(outputDir, outputPath, content); err != nil {
			return err
		}
	}
	return nil
}
