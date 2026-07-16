package pipeline

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/zeroedin/alloy/internal/config"
	"github.com/zeroedin/alloy/internal/ordered"
)

// validateOnConfigPath checks that a plugin-supplied path is safe to use as a
// project-relative directory. It rejects absolute paths, empty strings, ".",
// paths whose cleaned form starts with ".." (traversing above the project root),
// and non-local paths (Windows reserved device names, volume-relative paths).
// Valid paths are returned in cleaned form (e.g. "subdir/../dist" → "dist").
func validateOnConfigPath(field, value string) (string, error) {
	cleaned := filepath.Clean(value)

	if filepath.IsAbs(cleaned) {
		return "", fmt.Errorf("onConfig: %s: absolute path %q is not allowed", field, value)
	}

	if cleaned == "." {
		return "", fmt.Errorf("onConfig: %s: path %q resolves to the project root", field, value)
	}

	if cleaned == ".." || strings.HasPrefix(cleaned, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("onConfig: %s: path %q traverses above the project root", field, value)
	}

	if !filepath.IsLocal(cleaned) {
		return "", fmt.Errorf("onConfig: %s: path %q is not a valid local path", field, value)
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

	// Validate all path fields before applying any to prevent partial config
	// mutation on validation failure (e.g. build.output written but
	// structure.content rejected would leave cfg in a half-mutated state).
	type pathField struct {
		field   string
		cleaned string
	}
	var validatedPaths []pathField

	if build, ok := m["build"].(map[string]interface{}); ok {
		if output, ok := build["output"].(string); ok {
			cleaned, err := validateOnConfigPath("build.output", output)
			if err != nil {
				return err
			}
			validatedPaths = append(validatedPaths, pathField{"build.output", cleaned})
		}
	}

	if structure, ok := m["structure"].(map[string]interface{}); ok {
		for _, entry := range []struct {
			key, field string
		}{
			{"content", "structure.content"},
			{"layouts", "structure.layouts"},
			{"assets", "structure.assets"},
			{"static", "structure.static"},
			{"data", "structure.data"},
		} {
			if v, ok := structure[entry.key].(string); ok {
				cleaned, err := validateOnConfigPath(entry.field, v)
				if err != nil {
					return err
				}
				validatedPaths = append(validatedPaths, pathField{entry.field, cleaned})
			}
		}
	}

	var validatedMappings []config.PassthroughMapping
	hasPassthrough := false
	if passthrough, ok := m["passthrough"].([]interface{}); ok {
		hasPassthrough = true
		validatedMappings = make([]config.PassthroughMapping, 0, len(passthrough))
		for i, item := range passthrough {
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
			cleaned, err := validateOnConfigPath(fmt.Sprintf("passthrough[%d].from", i), mapping.From)
			if err != nil {
				return err
			}
			mapping.From = cleaned

			if to, ok := pm["to"].(string); ok {
				mapping.To = to
			}
			if mapping.To != "" && mapping.To != "." {
				cleaned, err := validateOnConfigPath(fmt.Sprintf("passthrough[%d].to", i), mapping.To)
				if err != nil {
					return err
				}
				mapping.To = cleaned
			}

			if exclude, ok := pm["exclude"].([]interface{}); ok {
				for _, e := range exclude {
					if s, ok := e.(string); ok {
						mapping.Exclude = append(mapping.Exclude, s)
					}
				}
			}
			validatedMappings = append(validatedMappings, mapping)
		}
	}

	// All path validations passed — apply to cfg.
	for _, pf := range validatedPaths {
		switch pf.field {
		case "build.output":
			cfg.Build.Output = pf.cleaned
		case "structure.content":
			cfg.Structure.Content = pf.cleaned
		case "structure.layouts":
			cfg.Structure.Layouts = pf.cleaned
		case "structure.assets":
			cfg.Structure.Assets = pf.cleaned
		case "structure.static":
			cfg.Structure.Static = pf.cleaned
		case "structure.data":
			cfg.Structure.Data = pf.cleaned
		}
	}

	if build, ok := m["build"].(map[string]interface{}); ok {
		if clean, ok := build["clean"].(bool); ok {
			cfg.Build.Clean = &clean
		}
	}

	if hasPassthrough {
		cfg.Passthrough = validatedMappings
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
