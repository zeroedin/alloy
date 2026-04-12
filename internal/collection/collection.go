package collection

import (
	"errors"

	"github.com/zeroedin/alloy/internal/content"
)

// ErrNotImplemented is returned by all stub functions.
var ErrNotImplemented = errors.New("not implemented")

// Collection represents a group of related pages (e.g., blog posts).
type Collection struct {
	Name  string
	Pages []*content.Page
}

// BuildCollections creates section collections from pages with date-based permalink patterns.
func BuildCollections(pages []*content.Page, permalinkCfg map[string]string) map[string]*Collection {
	return nil
}

// SortPages sorts a slice of pages by the given field and order.
func SortPages(pages []*content.Page, sortBy string, order string) []*content.Page {
	return nil
}

// SortByFrontMatter sorts pages by a front matter field value.
// Used for custom sort fields like "weight", "title", etc.
func SortByFrontMatter(pages []*content.Page, field string, order string) []*content.Page {
	return nil
}

// Freeze marks a collection as read-only. After freezing, any attempt
// to modify Pages should return an error or panic.
func (c *Collection) Freeze() {
	// stub: not implemented
}

// IsFrozen returns whether the collection has been frozen.
func (c *Collection) IsFrozen() bool {
	return false
}

// AddPage appends a page to the collection. Returns error if frozen.
func (c *Collection) AddPage(page *content.Page) error {
	return ErrNotImplemented
}

// BuildCollectionsWithMode builds collections with lifecycle filtering based on mode.
// devMode=true includes drafts; devMode=false excludes them.
func BuildCollectionsWithMode(pages []*content.Page, permalinkCfg map[string]string, devMode bool) map[string]*Collection {
	return nil
}
