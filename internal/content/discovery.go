package content

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ErrNotImplemented is returned by all stub functions.
var ErrNotImplemented = errors.New("not implemented")

// Discover walks the content directory and returns all content pages.
func Discover(contentDir string) ([]*Page, error) {
	return DiscoverWithFormats(contentDir, []string{"md", "html", "txt"})
}

// DiscoverWithFormats walks the content directory and returns only pages
// whose file extension matches one of the allowed formats.
func DiscoverWithFormats(contentDir string, formats []string) ([]*Page, error) {
	contentDir = filepath.Clean(contentDir)

	info, err := os.Stat(contentDir)
	if err != nil {
		return nil, fmt.Errorf("content discovery error: %s: %w", contentDir, err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("content discovery error: %s is not a directory", contentDir)
	}

	formatSet := make(map[string]bool)
	for _, f := range formats {
		formatSet["."+f] = true
	}

	// First pass: find all index.md/index.html files to identify bundles
	bundleDirs := make(map[string]bool)
	_ = filepath.Walk(contentDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		if info.Name() == "index.md" || info.Name() == "index.html" {
			dir := filepath.Dir(path)
			rel, _ := filepath.Rel(contentDir, dir)
			if rel != "." {
				bundleDirs[dir] = true
			}
		}
		return nil
	})

	var pages []*Page

	err = filepath.Walk(contentDir, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if info.IsDir() {
			return nil
		}

		name := info.Name()

		// Ignore _data.yaml and _data.yml
		if name == "_data.yaml" || name == "_data.yml" {
			return nil
		}

		ext := filepath.Ext(name)
		if !formatSet[ext] {
			return nil
		}

		rel, err := filepath.Rel(contentDir, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)

		// Read file and build page
		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}

		page, err := BuildPage(rel, raw)
		if err != nil {
			return err
		}

		page.SourcePath = path

		// Set section from first directory segment
		parts := strings.SplitN(rel, "/", 2)
		if len(parts) > 1 {
			page.Section = parts[0]
		}

		// Check if this is a bundle index file
		dir := filepath.Dir(path)
		if bundleDirs[dir] && (name == "index.md" || name == "index.html") {
			page.Bundle = true
			// Collect co-located assets
			entries, err := os.ReadDir(dir)
			if err == nil {
				for _, entry := range entries {
					if entry.IsDir() {
						continue
					}
					entryName := entry.Name()
					if entryName == name {
						continue
					}
					if entryName == "_data.yaml" || entryName == "_data.yml" {
						continue
					}
					page.BundleAssets = append(page.BundleAssets, entryName)
				}
			}
		}

		pages = append(pages, page)
		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("content discovery error: %s: %w", contentDir, err)
	}

	return pages, nil
}
