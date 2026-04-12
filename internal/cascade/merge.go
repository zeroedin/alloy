package cascade

import "errors"

// ErrNotImplemented is returned by all stub functions.
var ErrNotImplemented = errors.New("not implemented")

// DeepMerge merges overlay into base following Alloy's rules:
// - Objects are deep-merged (nested keys merge recursively)
// - Arrays are replaced (not concatenated)
func DeepMerge(base, overlay map[string]interface{}) map[string]interface{} {
	return nil
}

// LoadDirectoryCascade loads and merges _data.yaml files from root through
// nested directories, producing the cumulative data at each level.
// Returns a map of directory path -> merged data at that level.
func LoadDirectoryCascade(contentDir string) (map[string]map[string]interface{}, error) {
	return nil, ErrNotImplemented
}
