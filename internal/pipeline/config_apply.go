package pipeline

import (
	"fmt"

	"github.com/zeroedin/alloy/internal/config"
	"github.com/zeroedin/alloy/internal/ordered"
)

// applyOnConfigResult applies the return value of an onConfig hook to the
// pipeline config. Only fields in the mutable allowlist are applied; all
// other fields are silently preserved. Returns an error if the result is
// not a map[string]interface{} or *ordered.Map (i.e., non-object return from JS).
func applyOnConfigResult(cfg *config.Config, result interface{}) error {
	if result == nil {
		return fmt.Errorf("onConfig hook must return an object, got nil")
	}

	var m map[string]interface{}
	switch v := result.(type) {
	case map[string]interface{}:
		m = v
	case *ordered.Map:
		m = v.ToGoMap()
	default:
		return fmt.Errorf("onConfig hook must return an object, got %T", result)
	}

	if build, ok := m["build"].(map[string]interface{}); ok {
		if output, ok := build["output"].(string); ok {
			cfg.Build.Output = output
		}
		if clean, ok := build["clean"].(bool); ok {
			cfg.Build.Clean = &clean
		}
	}

	if structure, ok := m["structure"].(map[string]interface{}); ok {
		if v, ok := structure["content"].(string); ok {
			cfg.Structure.Content = v
		}
		if v, ok := structure["layouts"].(string); ok {
			cfg.Structure.Layouts = v
		}
		if v, ok := structure["assets"].(string); ok {
			cfg.Structure.Assets = v
		}
		if v, ok := structure["static"].(string); ok {
			cfg.Structure.Static = v
		}
		if v, ok := structure["data"].(string); ok {
			cfg.Structure.Data = v
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
		if timeout, ok := plugins["timeout"].(float64); ok {
			cfg.Plugins.Timeout = int(timeout)
		}
	}

	return nil
}
