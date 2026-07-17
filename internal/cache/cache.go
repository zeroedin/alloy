package cache

import (
	"crypto/sha256"
	"encoding/hex"
)

// Cache manages content hashes and template usage tracking for
// incremental build detection. Passed in-memory between builds.
type Cache struct {
	hashes              map[string]string
	templates           map[string]map[string]bool // template path → set of page paths
	directoryData       map[string]map[string]bool // directory path → set of page paths
	virtualPages        map[string]bool            // RelPaths of pages injected by onPagesReady (issue #970)
	virtualDependencies map[string]map[string]bool // source path → set of virtual page RelPaths (issue #1058)
}

// New creates an empty cache with no entries.
func New() *Cache {
	return &Cache{
		hashes:              make(map[string]string),
		templates:           make(map[string]map[string]bool),
		directoryData:       make(map[string]map[string]bool),
		virtualPages:        make(map[string]bool),
		virtualDependencies: make(map[string]map[string]bool),
	}
}

// SetHash records the content hash for a source file path.
// Empty keys are silently discarded — taxonomy pages have empty
// RelPath and the Build() hash loop would create a spurious entry.
func (c *Cache) SetHash(path, hash string) {
	if path == "" {
		return
	}
	c.hashes[path] = hash
}

// GetHash retrieves the stored content hash for a source file path.
// Returns empty string if no hash is stored.
func (c *Cache) GetHash(path string) string {
	return c.hashes[path]
}

// HasChanged returns true if the given hash differs from the stored
// hash for the path (or if no hash is stored). Used during rebuild
// to decide whether a file needs re-processing.
func (c *Cache) HasChanged(path, currentHash string) bool {
	stored, ok := c.hashes[path]
	if !ok {
		return true
	}
	return stored != currentHash
}

// Entries returns the number of stored content hashes.
func (c *Cache) Entries() int {
	return len(c.hashes)
}

// Clear removes all stored hashes and template tracking data.
func (c *Cache) Clear() {
	c.hashes = make(map[string]string)
	c.templates = make(map[string]map[string]bool)
	c.directoryData = make(map[string]map[string]bool)
	c.virtualPages = make(map[string]bool)
	c.virtualDependencies = make(map[string]map[string]bool)
}

// Clone returns a deep copy of the cache. Used by BuildIncremental to
// carry forward hashes for unchanged pages without re-hashing them.
func (c *Cache) Clone() *Cache {
	cloned := New()
	for k, v := range c.hashes {
		cloned.hashes[k] = v
	}
	for k, v := range c.templates {
		cp := make(map[string]bool, len(v))
		for p, val := range v {
			cp[p] = val
		}
		cloned.templates[k] = cp
	}
	for k, v := range c.directoryData {
		cp := make(map[string]bool, len(v))
		for p, val := range v {
			cp[p] = val
		}
		cloned.directoryData[k] = cp
	}
	for k, v := range c.virtualPages {
		cloned.virtualPages[k] = v
	}
	for k, v := range c.virtualDependencies {
		cp := make(map[string]bool, len(v))
		for p, val := range v {
			cp[p] = val
		}
		cloned.virtualDependencies[k] = cp
	}
	return cloned
}

// HashContent computes a SHA-256 hex digest of the given content bytes.
func HashContent(content []byte) string {
	h := sha256.Sum256(content)
	return hex.EncodeToString(h[:])
}

// TrackTemplateUsage records that a page uses a specific template.
// Called during layout resolution to build the invalidation map.
func (c *Cache) TrackTemplateUsage(pagePath, templatePath string) {
	pages := c.templates[templatePath]
	if pages == nil {
		pages = make(map[string]bool)
		c.templates[templatePath] = pages
	}
	pages[pagePath] = true
}

// InvalidatedPages returns the list of page paths that need rebuilding
// because the given template changed. Returns nil if the template
// is not tracked.
func (c *Cache) InvalidatedPages(changedTemplate string) []string {
	pages, ok := c.templates[changedTemplate]
	if !ok {
		return nil
	}
	result := make([]string, 0, len(pages))
	for p := range pages {
		result = append(result, p)
	}
	return result
}

// ShouldSkipFile returns true if the file content hash matches the stored hash,
// meaning the file has not changed and can be skipped entirely (no re-parse, no re-render).
func (c *Cache) ShouldSkipFile(path string, currentContent []byte) bool {
	currentHash := HashContent(currentContent)
	return !c.HasChanged(path, currentHash)
}

// IsConfigChanged returns true if the config hash differs from the stored hash.
// A config change triggers a full rebuild of all pages.
func (c *Cache) IsConfigChanged(configHash string) bool {
	return c.HasChanged("__config__", configHash)
}

// InvalidatedByGlobalData returns all page paths that need rebuilding when
// global data (data/ directory files) changes.
func (c *Cache) InvalidatedByGlobalData() []string {
	paths := make([]string, 0, len(c.hashes))
	for p := range c.hashes {
		paths = append(paths, p)
	}
	return paths
}

// InvalidatedByDirectoryData returns page paths that need rebuilding when
// a directory's _data.yaml changes. Only pages in that directory are affected.
func (c *Cache) InvalidatedByDirectoryData(dirPath string) []string {
	pages, ok := c.directoryData[dirPath]
	if !ok {
		return nil
	}
	result := make([]string, 0, len(pages))
	for p := range pages {
		result = append(result, p)
	}
	return result
}

// TrackDirectoryData records that pages in a directory depend on that directory's _data.yaml.
func (c *Cache) TrackDirectoryData(pagePath, dirPath string) {
	pages := c.directoryData[dirPath]
	if pages == nil {
		pages = make(map[string]bool)
		c.directoryData[dirPath] = pages
	}
	pages[pagePath] = true
}

// TrackVirtualPage records a RelPath as a virtual page injected by
// onPagesReady. Used by BuildIncremental to pre-populate renderRelPaths
// on the next rebuild so virtual pages aren't filtered out (issue #970).
func (c *Cache) TrackVirtualPage(relPath string) {
	c.virtualPages[relPath] = true
}

// ClearVirtualPages removes all virtual page tracking and dependency
// tracking without affecting content hashes or template tracking.
// Called before re-populating virtual pages after each build so stale
// entries don't persist.
func (c *Cache) ClearVirtualPages() {
	c.virtualPages = make(map[string]bool)
	c.virtualDependencies = make(map[string]map[string]bool)
}

// VirtualPagePaths returns all RelPaths tracked as virtual pages.
// Returns an empty (non-nil) slice when no virtual pages are tracked.
func (c *Cache) VirtualPagePaths() []string {
	result := make([]string, 0, len(c.virtualPages))
	for p := range c.virtualPages {
		result = append(result, p)
	}
	return result
}

// TrackVirtualDependency records that a virtual page depends on a source
// file outside the content directory. Builds a reverse map so
// InvalidatedVirtualPages can return all virtual pages affected by a
// source file change (issue #1058).
func (c *Cache) TrackVirtualDependency(virtualRelPath, sourcePath string) {
	pages := c.virtualDependencies[sourcePath]
	if pages == nil {
		pages = make(map[string]bool)
		c.virtualDependencies[sourcePath] = pages
	}
	pages[virtualRelPath] = true
}

// InvalidatedVirtualPages returns the virtual page RelPaths that depend
// on the given source file. Returns nil if no virtual page tracks the
// given source file as a dependency (issue #1058).
func (c *Cache) InvalidatedVirtualPages(changedFile string) []string {
	pages, ok := c.virtualDependencies[changedFile]
	if !ok {
		return nil
	}
	result := make([]string, 0, len(pages))
	for p := range pages {
		result = append(result, p)
	}
	return result
}
