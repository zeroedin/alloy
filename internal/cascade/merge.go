package cascade

import (
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// DeepMerge merges overlay into base following Alloy's rules:
// - Objects are deep-merged (nested keys merge recursively)
// - Arrays are replaced (not concatenated)
func DeepMerge(base, overlay map[string]interface{}) map[string]interface{} {
	if base == nil && overlay == nil {
		return nil
	}
	if base == nil {
		return copyMap(overlay)
	}
	if overlay == nil || len(overlay) == 0 {
		return copyMap(base)
	}

	result := copyMap(base)
	for k, ov := range overlay {
		bv, exists := result[k]
		if !exists {
			result[k] = ov
			continue
		}
		// If both are maps, deep merge recursively
		bMap, bIsMap := bv.(map[string]interface{})
		oMap, oIsMap := ov.(map[string]interface{})
		if bIsMap && oIsMap {
			result[k] = DeepMerge(bMap, oMap)
		} else {
			// Arrays and scalars: overlay wins (replaces)
			result[k] = ov
		}
	}
	return result
}

// copyMap creates a shallow copy of a map.
func copyMap(m map[string]interface{}) map[string]interface{} {
	if m == nil {
		return nil
	}
	result := make(map[string]interface{}, len(m))
	for k, v := range m {
		result[k] = v
	}
	return result
}

// LoadDirectoryCascade loads and merges _data.yaml files from root through
// nested directories, producing the cumulative data at each level.
// Returns a map of directory path -> merged data at that level.
//
// Uses a two-pass approach: first collects raw _data.yaml contents, then
// accumulates in depth order so that (a) parent data is always available
// before children regardless of WalkDir ordering, and (b) gaps in the
// _data.yaml chain are bridged by walking up to the nearest ancestor.
func LoadDirectoryCascade(contentDir string) (map[string]map[string]interface{}, error) {
	raw := make(map[string]map[string]interface{})

	contentDir = filepath.Clean(contentDir)
	baseName := filepath.Base(contentDir)

	err := filepath.WalkDir(contentDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		name := d.Name()
		if name != "_data.yaml" && name != "_data.yml" {
			return nil
		}

		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		var data map[string]interface{}
		if err := yaml.Unmarshal(b, &data); err != nil {
			return err
		}

		dir := filepath.Dir(path)
		rel, err := filepath.Rel(filepath.Dir(contentDir), dir)
		if err != nil {
			return err
		}
		key := strings.ReplaceAll(rel, string(filepath.Separator), "/")
		if !strings.HasSuffix(key, "/") {
			key += "/"
		}

		raw[key] = data
		return nil
	})
	if err != nil {
		return nil, err
	}

	keys := make([]string, 0, len(raw))
	for k := range raw {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		return strings.Count(keys[i], "/") < strings.Count(keys[j], "/")
	})

	result := make(map[string]map[string]interface{}, len(raw))
	for _, key := range keys {
		data := raw[key]
		parentKey := key
		for {
			next := findParentKey(parentKey, baseName)
			if next == parentKey {
				break
			}
			parentKey = next
			if parentData, ok := result[parentKey]; ok {
				data = DeepMerge(parentData, data)
				break
			}
		}
		result[key] = data
	}

	return result, nil
}

// FindCascadeData walks up the directory tree from a page's directory to find
// the nearest ancestor with cascade data. Returns nil when no ancestor has data.
// LoadDirectoryCascade already accumulates ancestor data into each directory
// entry, so returning the nearest match is sufficient.
func FindCascadeData(cascadeData map[string]map[string]interface{}, contentBase, relPath string) map[string]interface{} {
	dir := filepath.Dir(relPath)
	for {
		var key string
		if dir == "." {
			key = contentBase + "/"
		} else {
			key = contentBase + "/" + filepath.ToSlash(dir) + "/"
		}
		if data, ok := cascadeData[key]; ok {
			return data
		}
		if dir == "." {
			break
		}
		dir = filepath.Dir(dir)
	}
	return nil
}

// findParentKey finds the parent cascade key for a given directory key.
func findParentKey(key, baseName string) string {
	// Remove trailing slash, get parent dir, add trailing slash
	trimmed := strings.TrimSuffix(key, "/")
	parent := filepath.Dir(trimmed)
	parent = strings.ReplaceAll(parent, string(filepath.Separator), "/")
	if !strings.HasSuffix(parent, "/") {
		parent += "/"
	}
	return parent
}
