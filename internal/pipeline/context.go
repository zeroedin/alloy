package pipeline

import (
	"errors"
	"fmt"
	"io/fs"
	"sort"

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

// buildCollectionsContext builds section collections (directory-based),
// returning them as a template-friendly map. Taxonomies are handled
// separately via buildTaxonomiesContext.
func buildCollectionsContext(pages []*content.Page, permalinkCfg map[string]string, collectionNames []string) map[string]interface{} {
	result := make(map[string]interface{})

	// Section collections — convert pages to template-friendly maps so
	// Liquid can access fields like {{ post.title }} and {{ post.url }}.
	colls := collection.BuildCollections(pages, permalinkCfg, collectionNames)
	for name, coll := range colls {
		items := make([]interface{}, len(coll.Pages))
		for i, p := range coll.Pages {
			items[i] = p.ToTemplateMap()
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
// via ToTemplateMap() so Liquid can access fields like {{ p.title }}.
func buildTaxonomiesContext(taxonomies map[string]*collection.TaxonomyCollection) map[string]interface{} {
	if len(taxonomies) == 0 {
		return nil
	}
	taxMap := make(map[string]interface{})
	for name, tc := range taxonomies {
		termMap := make(map[string]interface{})
		for term, termPages := range tc.Terms {
			items := make([]interface{}, len(termPages))
			for i, p := range termPages {
				items[i] = p.ToTemplateMap()
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
