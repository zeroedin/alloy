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
