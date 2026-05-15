package content

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// Discover walks the content directory and returns all content pages.
func Discover(contentDir string) ([]*Page, error) {
	return DiscoverWithFormats(contentDir, []string{"md", "html", "txt"})
}

// DiscoverWithFormats walks the content directory and returns only pages
// whose file extension matches one of the allowed formats.
func DiscoverWithFormats(contentDir string, formats []string) ([]*Page, error) {
	pages, _, err := discoverInternal(contentDir, formats, false)
	return pages, err
}

// DiscoverWithPassthrough walks the content directory and returns content pages
// (files matching formats) and passthrough file paths (everything else).
// Excludes _data.yaml, _data.yml, and dot-prefixed files from passthrough.
func DiscoverWithPassthrough(contentDir string, formats []string) ([]*Page, []string, error) {
	return discoverInternal(contentDir, formats, true)
}

// discoverInternal is the shared walk logic for content discovery.
// When collectPassthrough is true, non-format files are collected as
// passthrough paths instead of being silently skipped.
func discoverInternal(contentDir string, formats []string, collectPassthrough bool) ([]*Page, []string, error) {
	contentDir = filepath.Clean(contentDir)

	info, err := os.Stat(contentDir)
	if err != nil {
		return nil, nil, fmt.Errorf("content discovery error: %s: %w", contentDir, err)
	}
	if !info.IsDir() {
		return nil, nil, fmt.Errorf("content discovery error: %s is not a directory", contentDir)
	}

	formatSet := make(map[string]bool)
	for _, f := range formats {
		formatSet["."+f] = true
	}

	type fileEntry struct {
		path string
		rel  string
		raw  []byte
		name string
		ext  string
	}

	bundleDirs := make(map[string]bool)
	var entries []fileEntry
	var passthroughs []string

	// Single walk: collect all files and identify bundles simultaneously
	err = filepath.WalkDir(contentDir, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}

		name := d.Name()

		if name == "_data.yaml" || name == "_data.yml" {
			return nil
		}

		// Track bundle dirs as we discover index files
		if name == "index.md" || name == "index.html" {
			dir := filepath.Dir(path)
			rel, _ := filepath.Rel(contentDir, dir)
			if rel != "." {
				bundleDirs[dir] = true
			}
		}

		ext := filepath.Ext(name)
		if !formatSet[ext] {
			if collectPassthrough {
				if strings.HasPrefix(name, ".") {
					return nil
				}
				rel, err := filepath.Rel(contentDir, path)
				if err != nil {
					return err
				}
				passthroughs = append(passthroughs, filepath.ToSlash(rel))
			}
			return nil
		}

		rel, err := filepath.Rel(contentDir, path)
		if err != nil {
			return err
		}

		raw, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}

		entries = append(entries, fileEntry{
			path: path,
			rel:  filepath.ToSlash(rel),
			raw:  raw,
			name: name,
			ext:  ext,
		})
		return nil
	})
	if err != nil {
		return nil, nil, fmt.Errorf("content discovery error: %s: %w", contentDir, err)
	}

	// Build pages using fully populated bundleDirs
	var pages []*Page
	for _, e := range entries {
		if !hasFrontMatter(e.raw) && e.ext != ".md" {
			if e.ext == ".html" && isFullHTMLDocument(e.raw) {
				if collectPassthrough {
					passthroughs = append(passthroughs, e.rel)
				}
				continue
			}
			page := &Page{
				RelPath:     e.rel,
				FrontMatter: map[string]interface{}{},
				Body:        e.raw,
				Content:     e.raw,
			}
			page.SourcePath = e.path
			parts := strings.SplitN(e.rel, "/", 2)
			if len(parts) > 1 {
				page.Section = parts[0]
			}
			dir := filepath.Dir(e.path)
			if bundleDirs[dir] && (e.name == "index.md" || e.name == "index.html") {
				page.Bundle = true
				dirEntries, err := os.ReadDir(dir)
				if err == nil {
					for _, de := range dirEntries {
						if de.IsDir() {
							continue
						}
						entryName := de.Name()
						if entryName == e.name || entryName == "_data.yaml" || entryName == "_data.yml" {
							continue
						}
						page.BundleAssets = append(page.BundleAssets, entryName)
					}
				}
			}
			pages = append(pages, page)
			continue
		}

		page, err := BuildPage(e.rel, e.raw)
		if err != nil {
			return nil, nil, fmt.Errorf("content discovery error: %s: %w", contentDir, err)
		}

		page.SourcePath = e.path

		parts := strings.SplitN(e.rel, "/", 2)
		if len(parts) > 1 {
			page.Section = parts[0]
		}

		dir := filepath.Dir(e.path)
		if bundleDirs[dir] && (e.name == "index.md" || e.name == "index.html") {
			page.Bundle = true
			dirEntries, err := os.ReadDir(dir)
			if err == nil {
				for _, de := range dirEntries {
					if de.IsDir() {
						continue
					}
					entryName := de.Name()
					if entryName == e.name || entryName == "_data.yaml" || entryName == "_data.yml" {
						continue
					}
					page.BundleAssets = append(page.BundleAssets, entryName)
				}
			}
		}

		pages = append(pages, page)
	}

	return pages, passthroughs, nil
}
