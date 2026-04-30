package collection

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/zeroedin/alloy/internal/content"
)

// Collection represents a group of related pages (e.g., blog posts).
type Collection struct {
	Name   string
	Pages  []*content.Page
	frozen bool
}

// BuildCollections creates section collections from pages with date-based permalink patterns.
// Only sections whose permalink pattern contains date tokens (:year, :month, :day)
// produce collections. Draft pages are excluded.
func BuildCollections(pages []*content.Page, permalinkCfg map[string]string) map[string]*Collection {
	// Determine which sections have date-based patterns
	dateSections := make(map[string]bool)
	for section, pattern := range permalinkCfg {
		if section == "default" {
			continue
		}
		if containsDateToken(pattern) {
			dateSections[section] = true
		}
	}

	collections := make(map[string]*Collection)
	for _, page := range pages {
		if page.Draft {
			continue
		}
		if !dateSections[page.Section] {
			continue
		}
		if isSectionIndex(page.RelPath, page.Section) {
			continue
		}
		c, ok := collections[page.Section]
		if !ok {
			c = &Collection{Name: page.Section}
			collections[page.Section] = c
		}
		c.Pages = append(c.Pages, page)
	}

	return collections
}

// isSectionIndex returns true if the page is a section-level index file
// (e.g., blog/index.md) as opposed to a page bundle index file
// (e.g., blog/second-post/index.md). Section indexes are containers,
// not collection members. Page bundles ARE collection members.
func isSectionIndex(relPath string, section string) bool {
	base := filepath.Base(relPath)
	name := strings.TrimSuffix(base, filepath.Ext(base))
	return name == "index" && filepath.Dir(relPath) == section
}

// containsDateToken checks if a pattern has :year, :month, or :day.
func containsDateToken(pattern string) bool {
	return strings.Contains(pattern, ":year") ||
		strings.Contains(pattern, ":month") ||
		strings.Contains(pattern, ":day")
}

// SortPages sorts a slice of pages by the given field and order.
func SortPages(pages []*content.Page, sortBy string, order string) []*content.Page {
	result := make([]*content.Page, len(pages))
	copy(result, pages)

	sort.SliceStable(result, func(i, j int) bool {
		a, b := result[i], result[j]

		switch sortBy {
		case "date":
			aHasDate := !a.Date.IsZero()
			bHasDate := !b.Date.IsZero()

			// Dateless pages always sort after dated pages
			if aHasDate && !bHasDate {
				return true
			}
			if !aHasDate && bHasDate {
				return false
			}
			if !aHasDate && !bHasDate {
				// Both dateless: sort by filename ascending
				return a.RelPath < b.RelPath
			}

			// Both have dates
			if !a.Date.Equal(b.Date) {
				if order == "asc" {
					return a.Date.Before(b.Date)
				}
				return a.Date.After(b.Date)
			}

			// Same date: filename ascending tiebreaker
			return a.RelPath < b.RelPath
		}

		return false
	})

	return result
}

// SortByFrontMatter sorts pages by a front matter field value.
// Used for custom sort fields like "weight", "title", etc.
func SortByFrontMatter(pages []*content.Page, field string, order string) []*content.Page {
	result := make([]*content.Page, len(pages))
	copy(result, pages)

	sort.SliceStable(result, func(i, j int) bool {
		aVal := result[i].FrontMatter[field]
		bVal := result[j].FrontMatter[field]

		cmp := compareValues(aVal, bVal)
		if order == "desc" {
			return cmp > 0
		}
		return cmp < 0
	})

	return result
}

// compareValues compares two interface values for sorting.
func compareValues(a, b interface{}) int {
	// Handle nil
	if a == nil && b == nil {
		return 0
	}
	if a == nil {
		return 1
	}
	if b == nil {
		return -1
	}

	// Try numeric comparison
	aNum, aOk := toFloat64(a)
	bNum, bOk := toFloat64(b)
	if aOk && bOk {
		if aNum < bNum {
			return -1
		}
		if aNum > bNum {
			return 1
		}
		return 0
	}

	// Fall back to string comparison
	aStr := fmt.Sprintf("%v", a)
	bStr := fmt.Sprintf("%v", b)
	if aStr < bStr {
		return -1
	}
	if aStr > bStr {
		return 1
	}
	return 0
}

func toFloat64(v interface{}) (float64, bool) {
	switch n := v.(type) {
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	case float64:
		return n, true
	case float32:
		return float64(n), true
	}
	return 0, false
}

// Freeze marks a collection as read-only. After freezing, any attempt
// to modify Pages should return an error.
func (c *Collection) Freeze() {
	c.frozen = true
}

// IsFrozen returns whether the collection has been frozen.
func (c *Collection) IsFrozen() bool {
	return c.frozen
}

// AddPage appends a page to the collection. Returns error if frozen.
func (c *Collection) AddPage(page *content.Page) error {
	if c.frozen {
		return fmt.Errorf("cannot add page to frozen collection %q", c.Name)
	}
	c.Pages = append(c.Pages, page)
	return nil
}

// BuildCollectionsWithMode builds collections with lifecycle filtering based on mode.
// devMode=true includes drafts; devMode=false excludes them.
func BuildCollectionsWithMode(pages []*content.Page, permalinkCfg map[string]string, devMode bool) map[string]*Collection {
	now := time.Now()
	filtered := content.FilterByLifecycle(pages, now, devMode)
	return buildCollectionsIncludeAll(filtered, permalinkCfg)
}

// buildCollectionsIncludeAll creates section collections from pre-filtered pages.
// Unlike BuildCollections, this does not re-filter drafts since lifecycle filtering
// has already been applied.
func buildCollectionsIncludeAll(pages []*content.Page, permalinkCfg map[string]string) map[string]*Collection {
	dateSections := make(map[string]bool)
	for section, pattern := range permalinkCfg {
		if section == "default" {
			continue
		}
		if containsDateToken(pattern) {
			dateSections[section] = true
		}
	}

	collections := make(map[string]*Collection)
	for _, page := range pages {
		if !dateSections[page.Section] {
			continue
		}
		if isSectionIndex(page.RelPath, page.Section) {
			continue
		}
		c, ok := collections[page.Section]
		if !ok {
			c = &Collection{Name: page.Section}
			collections[page.Section] = c
		}
		c.Pages = append(c.Pages, page)
	}

	return collections
}
