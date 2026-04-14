package cascade

import (
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// resolveDir resolves a directory path. If the path is relative and doesn't
// exist in the current directory, it walks up to find the module root (go.mod)
// and resolves from there. This handles `go test` running from package dirs.
func resolveDir(dir string) string {
	if filepath.IsAbs(dir) {
		return dir
	}
	// If the path exists relative to CWD, use it as-is
	if _, err := os.Stat(dir); err == nil {
		return dir
	}
	// Walk up to find the module root
	wd, err := os.Getwd()
	if err != nil {
		return dir
	}
	current := wd
	for {
		if _, err := os.Stat(filepath.Join(current, "go.mod")); err == nil {
			candidate := filepath.Join(current, dir)
			if _, err := os.Stat(candidate); err == nil {
				return candidate
			}
			return filepath.Join(current, dir)
		}
		parent := filepath.Dir(current)
		if parent == current {
			break
		}
		current = parent
	}
	return dir
}

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
func LoadDirectoryCascade(contentDir string) (map[string]map[string]interface{}, error) {
	result := make(map[string]map[string]interface{})

	// Resolve relative paths (handles go test running from package dirs)
	contentDir = resolveDir(contentDir)

	// Normalize the contentDir to get its base name for prefix keys
	contentDir = filepath.Clean(contentDir)
	baseName := filepath.Base(contentDir)

	err := filepath.Walk(contentDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		name := info.Name()
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

		// Compute the relative directory key
		dir := filepath.Dir(path)
		rel, err := filepath.Rel(filepath.Dir(contentDir), dir)
		if err != nil {
			return err
		}
		// Normalize to forward slashes with trailing slash
		key := strings.ReplaceAll(rel, string(filepath.Separator), "/")
		if !strings.HasSuffix(key, "/") {
			key += "/"
		}

		// Find the parent directory's accumulated data
		parentKey := findParentKey(key, baseName)
		if parentData, ok := result[parentKey]; ok {
			data = DeepMerge(parentData, data)
		}

		result[key] = data
		return nil
	})
	if err != nil {
		return nil, err
	}

	return result, nil
}

// FindCascadeData walks up the directory tree from a page's directory to find
// the nearest ancestor with cascade data. Returns nil when no ancestor has data.
// This must be used instead of exact key lookup so pages in directories without
// _data.yaml inherit from ancestors per spec §3.
func FindCascadeData(cascadeData map[string]map[string]interface{}, contentBase, relPath string) map[string]interface{} {
	return nil // stub — not implemented
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
