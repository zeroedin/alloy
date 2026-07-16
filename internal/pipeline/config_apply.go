package pipeline

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/zeroedin/alloy/internal/config"
	"github.com/zeroedin/alloy/internal/ordered"
)

// validateConfigPath checks that a plugin-supplied path is safe to use as a
// project-relative directory. It rejects absolute paths, empty strings, ".",
// and paths whose cleaned form starts with ".." (traversing above the project root).
// Valid paths are returned in cleaned form (e.g. "subdir/../dist" → "dist").
func validateConfigPath(field, value string) (string, error) {
	cleaned := filepath.Clean(value)

	if filepath.IsAbs(cleaned) {
		return "", fmt.Errorf("%s: absolute path %q is not allowed", field, value)
	}

	if cleaned == "." {
		return "", fmt.Errorf("%s: path %q resolves to the project root", field, value)
	}

	if cleaned == ".." || strings.HasPrefix(cleaned, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("%s: path %q traverses above the project root", field, value)
	}

	return cleaned, nil
}

// applyOnConfigResult applies the return value of an onConfig hook to the
// pipeline config. Only fields in the mutable allowlist are applied; all
// other fields are silently preserved. Returns an error if the result is
// not a map[string]interface{} or *ordered.Map (i.e., non-object return from JS).
//
// When RunWithTimeout times out, it returns the original *config.Config payload.
// That case is treated as a no-op (the timed-out hook's mutations are discarded).
func applyOnConfigResult(cfg *config.Config, result interface{}) error {
	if result == nil {
		return fmt.Errorf("hook must return an object, got nil")
	}

	var m map[string]interface{}
	switch v := result.(type) {
	case *config.Config:
		return nil
	case map[string]interface{}:
		m = v
	case *ordered.Map:
		m = v.ToGoMap()
	default:
		return fmt.Errorf("hook must return an object, got %T", result)
	}

	if build, ok := m["build"].(map[string]interface{}); ok {
		if output, ok := build["output"].(string); ok {
			cleaned, err := validateConfigPath("build.output", output)
			if err != nil {
				return err
			}
			cfg.Build.Output = cleaned
		}
		if clean, ok := build["clean"].(bool); ok {
			cfg.Build.Clean = &clean
		}
	}

	if structure, ok := m["structure"].(map[string]interface{}); ok {
		if v, ok := structure["content"].(string); ok {
			cleaned, err := validateConfigPath("structure.content", v)
			if err != nil {
				return err
			}
			cfg.Structure.Content = cleaned
		}
		if v, ok := structure["layouts"].(string); ok {
			cleaned, err := validateConfigPath("structure.layouts", v)
			if err != nil {
				return err
			}
			cfg.Structure.Layouts = cleaned
		}
		if v, ok := structure["assets"].(string); ok {
			cleaned, err := validateConfigPath("structure.assets", v)
			if err != nil {
				return err
			}
			cfg.Structure.Assets = cleaned
		}
		if v, ok := structure["static"].(string); ok {
			cleaned, err := validateConfigPath("structure.static", v)
			if err != nil {
				return err
			}
			cfg.Structure.Static = cleaned
		}
		if v, ok := structure["data"].(string); ok {
			cleaned, err := validateConfigPath("structure.data", v)
			if err != nil {
				return err
			}
			cfg.Structure.Data = cleaned
		}
	}

	if passthrough, ok := m["passthrough"].([]interface{}); ok {
		mappings := make([]config.PassthroughMapping, 0, len(passthrough))
		for _, item := range passthrough {
			pm, ok := item.(map[string]interface{})
			if !ok {
				continue
			}
			mapping := config.PassthroughMapping{}
			if from, ok := pm["from"].(string); ok {
				mapping.From = from
			}
			if mapping.From == "" {
				continue
			}
			if to, ok := pm["to"].(string); ok {
				mapping.To = to
			}
			if exclude, ok := pm["exclude"].([]interface{}); ok {
				for _, e := range exclude {
					if s, ok := e.(string); ok {
						mapping.Exclude = append(mapping.Exclude, s)
					}
				}
			}
			mappings = append(mappings, mapping)
		}
		cfg.Passthrough = mappings
	}

	if plugins, ok := m["plugins"].(map[string]interface{}); ok {
		if workers, exists := plugins["workers"]; exists {
			cfg.Plugins.Workers = workers
		}
		if raw, exists := plugins["timeout"]; exists {
			var timeout int
			switch v := raw.(type) {
			case float64:
				timeout = int(v)
			case int:
				timeout = v
			}
			if timeout > 0 {
				cfg.Plugins.Timeout = timeout
			}
		}
	}

	return nil
}
