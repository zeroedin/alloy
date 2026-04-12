package cache

import "errors"

// ErrNotImplemented is returned by all stub functions.
var ErrNotImplemented = errors.New("not implemented")

// Cache manages content hashes and template usage tracking for
// incremental build detection. Stored on disk as .alloy/cache.json.
type Cache struct {
	hashes    map[string]string
	templates map[string][]string // template path → list of page paths
}

// New creates an empty cache with no entries.
func New() *Cache {
	return &Cache{}
}

// SetHash records the content hash for a source file path.
func (c *Cache) SetHash(path, hash string) {
	// stub — no-op
}

// GetHash retrieves the stored content hash for a source file path.
// Returns empty string if no hash is stored.
func (c *Cache) GetHash(path string) string {
	return ""
}

// HasChanged returns true if the given hash differs from the stored
// hash for the path (or if no hash is stored). Used during rebuild
// to decide whether a file needs re-processing.
func (c *Cache) HasChanged(path, currentHash string) bool {
	return false
}

// Entries returns the number of stored content hashes.
func (c *Cache) Entries() int {
	return 0
}

// Clear removes all stored hashes and template tracking data.
func (c *Cache) Clear() {
	// stub — no-op
}

// SaveTo writes cache.json to the given directory (typically .alloy/).
// Creates the directory if it does not exist.
func (c *Cache) SaveTo(dir string) error {
	return ErrNotImplemented
}

// LoadFrom reads cache.json from the given directory and restores
// all entries. Returns an empty cache if the file does not exist
// (fresh build).
func LoadFrom(dir string) (*Cache, error) {
	return nil, ErrNotImplemented
}

// HashContent computes a SHA-256 hex digest of the given content bytes.
func HashContent(content []byte) string {
	return ""
}

// TrackTemplateUsage records that a page uses a specific template.
// Called during layout resolution to build the invalidation map.
func (c *Cache) TrackTemplateUsage(pagePath, templatePath string) {
	// stub — no-op
}

// InvalidatedPages returns the list of page paths that need rebuilding
// because the given template changed. Returns nil if the template
// is not tracked.
func (c *Cache) InvalidatedPages(changedTemplate string) []string {
	return nil
}

// InvalidateAll returns a signal that all pages need rebuilding
// (e.g., config change). Returns true if a full rebuild is required.
func (c *Cache) InvalidateAll() bool {
	return false
}

// ShouldSkipFile returns true if the file content hash matches the stored hash,
// meaning the file has not changed and can be skipped entirely (no re-parse, no re-render).
func (c *Cache) ShouldSkipFile(path string, currentContent []byte) bool {
	return false
}

// IsConfigChanged returns true if the config hash differs from the stored hash.
// A config change triggers a full rebuild of all pages.
func (c *Cache) IsConfigChanged(configHash string) bool {
	return false
}

// InvalidatedByGlobalData returns all page paths that need rebuilding when
// global data (data/ directory files) changes.
func (c *Cache) InvalidatedByGlobalData() []string {
	return nil
}

// InvalidatedByDirectoryData returns page paths that need rebuilding when
// a directory's _data.yaml changes. Only pages in that directory are affected.
func (c *Cache) InvalidatedByDirectoryData(dirPath string) []string {
	return nil
}

// TrackDirectoryData records that pages in a directory depend on that directory's _data.yaml.
func (c *Cache) TrackDirectoryData(pagePath, dirPath string) {
	// stub — no-op
}
