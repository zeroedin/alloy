package pipeline

import (
	"errors"
	"fmt"
	"io/fs"
	"sort"
	"strings"

	"github.com/zeroedin/alloy/internal/collection"
	"github.com/zeroedin/alloy/internal/config"
	"github.com/zeroedin/alloy/internal/content"
	"github.com/zeroedin/alloy/internal/data"
	"github.com/zeroedin/alloy/internal/ordered"
)

// loadSiteData loads data files from the configured data directory and
// merges any external file mappings from data.files config.
// Returns an error if an external file is missing or collides with a
// data directory entry.
func loadSiteData(cfg *config.Config) (map[string]interface{}, error) {
	var result map[string]interface{}

	dataDir := resolveDir(cfg.ProjectRoot, cfg.Structure.Data)
	if dataDir != "" {
		loaded, err := data.LoadDirectory(dataDir)
		if err != nil {
			if !errors.Is(err, fs.ErrNotExist) {
				return nil, fmt.Errorf("data directory %s: %w", dataDir, err)
			}
		} else {
			result = loaded
		}
	}

	if len(cfg.Data.Files) > 0 {
		external, err := data.LoadExternalFiles(cfg.Data.Files, cfg.ProjectRoot)
		if err != nil {
			return nil, fmt.Errorf("external data files: %w", err)
		}
		if len(external) > 0 {
			if result == nil {
				result = make(map[string]interface{})
			}
			sortedKeys := make([]string, 0, len(external))
			for k := range external {
				sortedKeys = append(sortedKeys, k)
			}
			sort.Strings(sortedKeys)
			for _, k := range sortedKeys {
				if _, exists := result[k]; exists {
					return nil, fmt.Errorf("external data files: key %q collides with data directory entry", k)
				}
				result[k] = external[k]
			}
		}
	}

	return result, nil
}

// listMap pairs a page with the map a list of pages holds for it, so the
// map can be filled again after the content hooks (issue #1385).
type listMap struct {
	page *content.Page
	m    map[string]interface{}
}

// buildCollectionsContext builds section collections (directory-based),
// returning them as a template-friendly map. Taxonomies are handled
// separately via buildTaxonomiesContext. Each collection is sorted by its
// entry in collCfg (issue #1418), and each page's map is added to maps
// (issue #1385).
func buildCollectionsContext(pages []*content.Page, permalinkCfg map[string]string, collectionNames []string, collCfg map[string]*config.CollectionConfig, maps *[]listMap) map[string]interface{} {
	result := make(map[string]interface{})

	// Section collections — convert pages to template-friendly maps so
	// Liquid can access fields like {{ post.title }} and {{ post.url }}.
	colls := collection.BuildCollections(pages, permalinkCfg, collectionNames)
	for name, coll := range colls {
		var sortBy, order string
		if c := collCfg[name]; c != nil {
			sortBy, order = c.SortBy, c.Order
		}
		coll.Pages = collection.Sort(coll.Pages, sortBy, order)
		items := make([]interface{}, len(coll.Pages))
		for i, p := range coll.Pages {
			m := p.ToTemplateMap()
			*maps = append(*maps, listMap{page: p, m: m})
			items[i] = m
		}
		result[name] = items
	}

	if len(result) == 0 {
		return nil
	}
	return result
}

// collectionNames extracts section names from cfg.Collections for explicit
// collection membership. Returns nil when no collections are configured.
func collectionNames(cfg *config.Config) []string {
	if len(cfg.Collections) == 0 {
		return nil
	}
	names := make([]string, 0, len(cfg.Collections))
	for name := range cfg.Collections {
		names = append(names, name)
	}
	return names
}

// buildTaxonomiesContext converts taxonomy collections into a template-friendly
// map for the top-level taxonomies.* namespace. Each term's pages are converted
// via ToTemplateMap() so Liquid can access fields like {{ p.title }}. Each
// page's map is added to maps.
func buildTaxonomiesContext(taxonomies map[string]*collection.TaxonomyCollection, maps *[]listMap) map[string]interface{} {
	if len(taxonomies) == 0 {
		return nil
	}
	taxMap := make(map[string]interface{})
	for name, tc := range taxonomies {
		termMap := make(map[string]interface{})
		for term, termPages := range tc.Terms {
			items := make([]interface{}, len(termPages))
			for i, p := range termPages {
				m := p.ToTemplateMap()
				*maps = append(*maps, listMap{page: p, m: m})
				items[i] = m
			}
			termMap[term] = items
		}
		taxMap[name] = termMap
	}
	if len(taxMap) == 0 {
		return nil
	}
	return taxMap
}

// pagedItems pairs a page that paginates a collection with the items
// pagination gave it: the collection's own maps (issue #1385). single says
// the page's as: name holds one item rather than the list (perPage 1,
// issue #1426).
type pagedItems struct {
	page   *content.Page
	items  []interface{}
	single bool
}

// collectionPagination records the items of each page that paginates a
// collection. Call it before the content hooks: they send and take back a
// page's whole front matter, _paginationCtx included, so afterwards its items
// are copies. Recording the items themselves, not a key such as url, keeps
// a page with no url, or two pages with the same one, on its own map.
func collectionPagination(pages []*content.Page) []pagedItems {
	var out []pagedItems
	for _, p := range pages {
		if data, _ := fieldOf(p.FrontMatter["pagination"], "data").(string); !strings.HasPrefix(data, "collections.") {
			continue
		}
		items, ok := fieldOf(p.FrontMatter["_paginationCtx"], "items").([]interface{})
		if !ok {
			continue
		}
		_, list := p.FrontMatter["_paginationData"].([]interface{})
		out = append(out, pagedItems{page: p, items: items, single: !list})
	}
	return out
}

// refreshListMaps fills each map a collection or taxonomy list holds again
// from its page, once onContentLoaded and onDataCascadeReady have run, so
// the lists show what those hooks added. Each map keeps its identity, so the
// lists keep the pages and order they were built with before the hooks.
// Each paginated page gets back the items collectionPagination recorded,
// which are those maps (issue #1385), in pagination.items and under its as:
// name (issue #1426).
func refreshListMaps(maps []listMap, paged []pagedItems) {
	for _, lm := range maps {
		fresh := lm.page.ToTemplateMap()
		for k := range lm.m {
			delete(lm.m, k)
		}
		for k, v := range fresh {
			lm.m[k] = v
		}
	}
	for _, pi := range paged {
		items := append([]interface{}(nil), pi.items...)
		switch c := pi.page.FrontMatter["_paginationCtx"].(type) {
		case map[string]interface{}:
			c["items"] = items
		case *ordered.Map:
			c.Set("items", items)
		}
		// The as: name, as processPagination sets it: the one item with
		// perPage 1, else the list.
		if pi.single && len(items) == 1 {
			pi.page.FrontMatter["_paginationData"] = convertOrderedValue(items[0])
		} else {
			pi.page.FrontMatter["_paginationData"] = convertOrderedValue(items)
		}
	}
}

// fieldOf returns key's value in v, a Go map or an ordered map, or nil.
func fieldOf(v interface{}, key string) interface{} {
	switch m := v.(type) {
	case map[string]interface{}:
		return m[key]
	case *ordered.Map:
		return m.Get(key)
	}
	return nil
}

// convertedFrontMatter replaces convertOrderedMaps(page.FrontMatter) at call
// sites. Converts *ordered.Map values to map[string]interface{} in-place and
// stores the result back on page.FrontMatter so subsequent calls (across hook
// types for the same page) skip the deep walk (issue #1185).
//
// Thread-safety: mutates page.FrontMatter in-place. Safe because the pipeline
// processes pages sequentially per hook stage — no concurrent readers during
// conversion. If page-level parallelism is added, this must be revisited.
func convertedFrontMatter(page *content.Page) map[string]interface{} {
	if page.FrontMatter == nil {
		return nil
	}
	if !needsOrderedMapConversion(page.FrontMatter) {
		return page.FrontMatter
	}
	for k, v := range page.FrontMatter {
		page.FrontMatter[k] = convertOrderedValue(v)
	}
	return page.FrontMatter
}

// convertOrderedMaps recursively converts *ordered.Map values to
// map[string]interface{} for fast JSON serialization in hook payloads.
func convertOrderedMaps(m map[string]interface{}) map[string]interface{} {
	if m == nil || !needsOrderedMapConversion(m) {
		return m
	}
	out := make(map[string]interface{}, len(m))
	for k, v := range m {
		out[k] = convertOrderedValue(v)
	}
	return out
}

func needsOrderedMapConversion(m map[string]interface{}) bool {
	for _, v := range m {
		if needsConversion(v) {
			return true
		}
	}
	return false
}

func needsConversion(v interface{}) bool {
	switch val := v.(type) {
	case *ordered.Map:
		return true
	case map[string]interface{}:
		return needsOrderedMapConversion(val)
	case []interface{}:
		for _, item := range val {
			if needsConversion(item) {
				return true
			}
		}
	}
	return false
}

func convertOrderedValue(v interface{}) interface{} {
	switch val := v.(type) {
	case *ordered.Map:
		return val.ToGoMap()
	case map[string]interface{}:
		return convertOrderedMaps(val)
	case []interface{}:
		result := make([]interface{}, len(val))
		for i, item := range val {
			result[i] = convertOrderedValue(item)
		}
		return result
	default:
		return v
	}
}

// toGoMap coerces a value to map[string]interface{}, accepting either
// map[string]interface{} directly or *ordered.Map (from JSON round-trip).
func toGoMap(v interface{}) (map[string]interface{}, bool) {
	switch val := v.(type) {
	case map[string]interface{}:
		return val, true
	case *ordered.Map:
		return val.ToGoMap(), true
	default:
		return nil, false
	}
}
