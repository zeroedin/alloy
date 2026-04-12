package fetch

import (
	"errors"
	"time"
)

// ErrNotImplemented is returned by all stub functions.
var ErrNotImplemented = errors.New("not implemented")

// FetchResult holds fetched data along with cache metadata.
type FetchResult struct {
	Data     interface{}
	CachedAt time.Time
	Source   string
}

// FetchREST fetches data from a REST endpoint and parses the response.
func FetchREST(url string) (interface{}, error) {
	return nil, ErrNotImplemented
}

// FetchGraphQL sends a GraphQL query and returns the unwrapped data.
func FetchGraphQL(endpoint, query string) (interface{}, error) {
	return nil, ErrNotImplemented
}

// GetCached returns cached data if the TTL has not expired.
func GetCached(name, cacheDir string, ttl int) (interface{}, bool) {
	return nil, false
}

// SaveCache writes fetched data to the cache directory.
func SaveCache(name, cacheDir string, data interface{}) error {
	return ErrNotImplemented
}

// ParseXML parses an XML response body into a map structure.
func ParseXML(data []byte) (map[string]interface{}, error) {
	return nil, ErrNotImplemented
}

// ParseCSVResponse parses a CSV response body into rows of maps.
func ParseCSVResponse(data []byte) ([]map[string]string, error) {
	return nil, ErrNotImplemented
}

// UnwrapGraphQLData extracts the "data" field from a GraphQL JSON response.
// GraphQL responses wrap results in {"data": {...}, "errors": [...]}.
func UnwrapGraphQLData(raw map[string]interface{}) (interface{}, error) {
	return nil, ErrNotImplemented
}

// CacheDir returns the default cache directory path (.alloy/fetch-cache/).
func CacheDir(projectRoot string) string {
	return ""
}

// GetCachedWithTTL returns cached data only if it hasn't expired.
// In build mode, expired cache must not be used.
func GetCachedWithTTL(name, cacheDir string, ttlSeconds int) (interface{}, bool, error) {
	return nil, false, ErrNotImplemented
}

// FetchRESTWithRefetch fetches from a REST endpoint, bypassing cache when refetch is true.
func FetchRESTWithRefetch(url string, cacheDir string, refetch bool) (interface{}, error) {
	return nil, ErrNotImplemented
}

// PluginSourceHandler is a function provided by a plugin to fetch data.
type PluginSourceHandler func(config map[string]interface{}) (interface{}, error)

// RegisterPluginSource registers a named plugin source handler.
func RegisterPluginSource(name string, handler PluginSourceHandler) {
	// stub — no-op
}

// FetchPluginSource invokes the registered plugin source handler by name.
func FetchPluginSource(name string, config map[string]interface{}) (interface{}, error) {
	return nil, ErrNotImplemented
}

// RegisteredPluginSources returns the names of all registered plugin source handlers.
func RegisteredPluginSources() []string {
	return nil
}
