package fetch

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// FetchResult holds fetched data along with cache metadata.
type FetchResult struct {
	Data     interface{}
	CachedAt time.Time
	Source   string
}

// FetchREST fetches data from a REST endpoint and parses the JSON response.
func FetchREST(url string) (interface{}, error) {
	resp, err := http.Get(url)
	if err != nil {
		return nil, fmt.Errorf("fetch request failed for %s: %w", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("fetch HTTP %d from %s", resp.StatusCode, url)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("fetch read error for %s: %w", url, err)
	}

	var result interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("fetch JSON parse error for %s: %w", url, err)
	}

	return result, nil
}

// FetchGraphQL sends a GraphQL query and returns the unwrapped data.
func FetchGraphQL(endpoint, query string) (interface{}, error) {
	reqBody, err := json.Marshal(map[string]string{"query": query})
	if err != nil {
		return nil, fmt.Errorf("graphql request encoding error: %w", err)
	}

	resp, err := http.Post(endpoint, "application/json", bytes.NewReader(reqBody))
	if err != nil {
		return nil, fmt.Errorf("graphql request failed for %s: %w", endpoint, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("graphql read error for %s: %w", endpoint, err)
	}

	var raw map[string]interface{}
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("graphql JSON parse error for %s: %w", endpoint, err)
	}

	return UnwrapGraphQLData(raw)
}

// cacheEntry holds cached data with a timestamp.
type cacheEntry struct {
	Data     interface{} `json:"data"`
	CachedAt time.Time   `json:"cached_at"`
}

// GetCached returns cached data if the TTL has not expired.
func GetCached(name, cacheDir string, ttl int) (interface{}, bool) {
	data, found, _ := GetCachedWithTTL(name, cacheDir, ttl)
	return data, found
}

// SaveCache writes fetched data to the cache directory.
func SaveCache(name, cacheDir string, data interface{}) error {
	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		return fmt.Errorf("cache dir creation failed: %w", err)
	}

	entry := cacheEntry{
		Data:     data,
		CachedAt: time.Now(),
	}

	b, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("cache serialization error: %w", err)
	}

	path := filepath.Join(cacheDir, name+".json")
	return os.WriteFile(path, b, 0644)
}

// ParseXML parses an XML response body into a map structure.
func ParseXML(data []byte) (map[string]interface{}, error) {
	decoder := xml.NewDecoder(bytes.NewReader(data))
	result := make(map[string]interface{})

	var currentKey string
	for {
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("XML parse error: %w", err)
		}

		switch t := token.(type) {
		case xml.StartElement:
			if t.Name.Local != "root" {
				currentKey = t.Name.Local
			}
		case xml.CharData:
			if currentKey != "" {
				result[currentKey] = strings.TrimSpace(string(t))
			}
		case xml.EndElement:
			currentKey = ""
		}
	}

	return result, nil
}

// ParseCSVResponse parses a CSV response body into rows of maps.
// First row is treated as headers.
func ParseCSVResponse(data []byte) ([]map[string]string, error) {
	reader := csv.NewReader(bytes.NewReader(data))

	records, err := reader.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("CSV parse error: %w", err)
	}

	if len(records) < 2 {
		return nil, nil
	}

	headers := records[0]
	var rows []map[string]string
	for _, record := range records[1:] {
		row := make(map[string]string, len(headers))
		for i, header := range headers {
			if i < len(record) {
				row[header] = record[i]
			}
		}
		rows = append(rows, row)
	}

	return rows, nil
}

// UnwrapGraphQLData extracts the "data" field from a GraphQL JSON response.
func UnwrapGraphQLData(raw map[string]interface{}) (interface{}, error) {
	data, ok := raw["data"]
	if !ok {
		return nil, fmt.Errorf("graphql response missing 'data' field")
	}
	return data, nil
}

// CacheDir returns the default cache directory path (.alloy/fetch-cache/).
func CacheDir(projectRoot string) string {
	return projectRoot + "/.alloy/fetch-cache/"
}

// GetCachedWithTTL returns cached data only if it hasn't expired.
func GetCachedWithTTL(name, cacheDir string, ttlSeconds int) (interface{}, bool, error) {
	path := filepath.Join(cacheDir, name+".json")
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, false, nil
	}

	var entry cacheEntry
	if err := json.Unmarshal(b, &entry); err != nil {
		return nil, false, nil
	}

	if ttlSeconds > 0 && time.Since(entry.CachedAt) > time.Duration(ttlSeconds)*time.Second {
		return nil, false, nil
	}

	// TTL of 0 means already expired
	if ttlSeconds == 0 {
		return nil, false, nil
	}

	return entry.Data, true, nil
}

// FetchRESTWithRefetch fetches from a REST endpoint, bypassing cache when refetch is true.
func FetchRESTWithRefetch(url string, cacheDir string, refetch bool) (interface{}, error) {
	if !refetch {
		data, found := GetCached(url, cacheDir, 3600)
		if found {
			return data, nil
		}
	}

	return FetchREST(url)
}

// PluginSourceHandler is a function provided by a plugin to fetch data.
type PluginSourceHandler func(config map[string]interface{}) (interface{}, error)

// pluginSources holds registered plugin source handlers.
var pluginSources = make(map[string]PluginSourceHandler)

// RegisterPluginSource registers a named plugin source handler.
func RegisterPluginSource(name string, handler PluginSourceHandler) {
	pluginSources[name] = handler
}

// FetchPluginSource invokes the registered plugin source handler by name.
func FetchPluginSource(name string, config map[string]interface{}) (interface{}, error) {
	handler, ok := pluginSources[name]
	if !ok {
		return nil, fmt.Errorf("plugin source %q not registered", name)
	}
	return handler(config)
}

// RegisteredPluginSources returns the names of all registered plugin source handlers.
func RegisteredPluginSources() []string {
	names := make([]string, 0, len(pluginSources))
	for name := range pluginSources {
		names = append(names, name)
	}
	return names
}
