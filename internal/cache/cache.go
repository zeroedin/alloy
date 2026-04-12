package cache

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
)

// Cache manages content hashes and template usage tracking for
// incremental build detection. Stored on disk as .alloy/cache.json.
type Cache struct {
	hashes        map[string]string
	templates     map[string][]string // template path → list of page paths
	directoryData map[string][]string // directory path → list of page paths
}

// cacheJSON is the serialization format for cache persistence.
type cacheJSON struct {
	Hashes        map[string]string   `json:"hashes"`
	Templates     map[string][]string `json:"templates"`
	DirectoryData map[string][]string `json:"directoryData"`
}

// New creates an empty cache with no entries.
func New() *Cache {
	return &Cache{
		hashes:        make(map[string]string),
		templates:     make(map[string][]string),
		directoryData: make(map[string][]string),
	}
}

// SetHash records the content hash for a source file path.
func (c *Cache) SetHash(path, hash string) {
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
	c.templates = make(map[string][]string)
	c.directoryData = make(map[string][]string)
}

// SaveTo writes cache.json to the given directory (typically .alloy/).
// Creates the directory if it does not exist.
func (c *Cache) SaveTo(dir string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	data := cacheJSON{
		Hashes:        c.hashes,
		Templates:     c.templates,
		DirectoryData: c.directoryData,
	}
	b, err := json.Marshal(data)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "cache.json"), b, 0o644)
}

// LoadFrom reads cache.json from the given directory and restores
// all entries. Returns an empty cache if the file does not exist
// (fresh build).
func LoadFrom(dir string) (*Cache, error) {
	b, err := os.ReadFile(filepath.Join(dir, "cache.json"))
	if err != nil {
		if os.IsNotExist(err) {
			return New(), nil
		}
		return nil, err
	}
	var data cacheJSON
	if err := json.Unmarshal(b, &data); err != nil {
		return nil, err
	}
	c := New()
	if data.Hashes != nil {
		c.hashes = data.Hashes
	}
	if data.Templates != nil {
		c.templates = data.Templates
	}
	if data.DirectoryData != nil {
		c.directoryData = data.DirectoryData
	}
	return c, nil
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
	for _, p := range pages {
		if p == pagePath {
			return
		}
	}
	c.templates[templatePath] = append(c.templates[templatePath], pagePath)
}

// InvalidatedPages returns the list of page paths that need rebuilding
// because the given template changed. Returns nil if the template
// is not tracked.
func (c *Cache) InvalidatedPages(changedTemplate string) []string {
	pages, ok := c.templates[changedTemplate]
	if !ok {
		return nil
	}
	return pages
}

// InvalidateAll returns a signal that all pages need rebuilding
// (e.g., config change). Returns true if a full rebuild is required.
func (c *Cache) InvalidateAll() bool {
	return true
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
	return pages
}

// TrackDirectoryData records that pages in a directory depend on that directory's _data.yaml.
func (c *Cache) TrackDirectoryData(pagePath, dirPath string) {
	pages := c.directoryData[dirPath]
	for _, p := range pages {
		if p == pagePath {
			return
		}
	}
	c.directoryData[dirPath] = append(c.directoryData[dirPath], pagePath)
}
