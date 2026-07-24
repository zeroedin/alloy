package plugin

import "fmt"

// FileChangedResult holds the structured return value from an onFileChanged
// hook. Nil when the plugin returns nil, non-map, or a map with no
// recognized keys (backward-compatible no-op).
type FileChangedResult struct {
	InvalidateByDependency []string
	Restart                bool
	Warnings               []string
}

// ParseFileChangedResult extracts the structured return value from an
// onFileChanged hook result. Returns nil for nil, non-map, or maps
// without recognized keys (issue #1100).
func ParseFileChangedResult(result interface{}) *FileChangedResult {
	if result == nil {
		return nil
	}
	m, ok := result.(map[string]interface{})
	if !ok {
		return nil
	}

	hasRecognized := false
	parsed := &FileChangedResult{}

	if raw, exists := m["invalidateByDependency"]; exists {
		hasRecognized = true
		if arr, ok := raw.([]interface{}); ok {
			deps := make([]string, 0, len(arr))
			for _, entry := range arr {
				if s, ok := entry.(string); ok {
					deps = append(deps, s)
				} else {
					parsed.Warnings = append(parsed.Warnings,
						fmt.Sprintf("non-string entry in invalidateByDependency: %T", entry))
				}
			}
			parsed.InvalidateByDependency = deps
		} else {
			parsed.Warnings = append(parsed.Warnings,
				fmt.Sprintf("invalidateByDependency must be an array, got %T", raw))
		}
	}

	if raw, exists := m["restart"]; exists {
		hasRecognized = true
		if b, ok := raw.(bool); ok {
			parsed.Restart = b
		} else {
			parsed.Warnings = append(parsed.Warnings,
				fmt.Sprintf("restart must be a boolean, got %T — value dropped", raw))
		}
	}

	if !hasRecognized {
		return nil
	}
	return parsed
}
