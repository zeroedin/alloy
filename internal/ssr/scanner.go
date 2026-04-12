package ssr

import "errors"

// ErrNotImplemented is returned by all stub functions.
var ErrNotImplemented = errors.New("not implemented")

// ComponentInstance represents a discovered custom element in HTML.
type ComponentInstance struct {
	Tag   string
	Attrs map[string]string
	Hash  string
}

// ScanComponents parses HTML and finds all custom element tags (tags with hyphens).
func ScanComponents(html string) []ComponentInstance {
	return nil
}

// DeduplicateInstances removes duplicate component instances by hash(tag + attributes).
func DeduplicateInstances(instances []ComponentInstance) []ComponentInstance {
	return nil
}

// InsertMarkers wraps component instances with <!--alloy-ssr:hash--> comment markers.
func InsertMarkers(html string, instances []ComponentInstance) string {
	return ""
}

// StampBack replaces marker regions with SSR'd output containing Declarative Shadow DOM.
func StampBack(html string, ssrResults map[string]string) string {
	return ""
}

// SSRConfig holds SSR pipeline configuration parsed from alloy.config.
type SSRConfig struct {
	BuildCmd      string // e.g. "golit render _site/**/*.html"
	ServeCMD      string // e.g. "golit serve"
	ServeEndpoint string // e.g. "http://localhost:6274"
}

// ParseSSRConfig extracts and validates SSR configuration from a config map.
func ParseSSRConfig(raw map[string]interface{}) (*SSRConfig, error) {
	return nil, ErrNotImplemented
}

// RenderViaHTTP sends HTML to an SSR endpoint via POST and returns SSR'd HTML.
func RenderViaHTTP(endpoint string, html string) (string, error) {
	return "", ErrNotImplemented
}

// RenderViaStdio sends HTML to an SSR process via stdin and reads SSR'd HTML from stdout.
// HTML is NUL-terminated on stdin/stdout.
func RenderViaStdio(cmd string, html string) (string, error) {
	return "", ErrNotImplemented
}

// ComponentCacheKey computes the cache key for a component instance:
// hash(tag + sorted_attributes + component_definition_hash).
func ComponentCacheKey(instance ComponentInstance, definitionHash string) string {
	return ""
}

// HashOutput computes a content hash for Phase 2 output comparison.
// If the hash matches the cached hash, SSR can be skipped for that page.
func HashOutput(html string) string {
	return ""
}
