package data

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
	"gopkg.in/yaml.v3"
)

// LoadFile loads a single data file (YAML, TOML, JSON) and returns its contents as a map.
// For files with root-level arrays, use LoadFileAny instead.
func LoadFile(path string) (map[string]interface{}, error) {
	v, err := LoadFileAny(path)
	if err != nil {
		return nil, err
	}
	m, ok := v.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("parsing %s: expected map at root level, got %T", path, v)
	}
	return m, nil
}

// LoadFileAny loads a single data file (YAML, TOML, JSON) and returns its contents.
// Unlike LoadFile, it supports both root-level maps and root-level arrays.
func LoadFileAny(path string) (interface{}, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".yaml", ".yml":
		var result interface{}
		if err := yaml.Unmarshal(b, &result); err != nil {
			return nil, fmt.Errorf("parsing YAML %s: %w", path, err)
		}
		return result, nil
	case ".toml":
		var result map[string]interface{}
		if err := toml.Unmarshal(b, &result); err != nil {
			return nil, fmt.Errorf("parsing TOML %s: %w", path, err)
		}
		return result, nil
	case ".json":
		var result interface{}
		if err := json.Unmarshal(b, &result); err != nil {
			return nil, fmt.Errorf("parsing JSON %s: %w", path, err)
		}
		return result, nil
	default:
		return nil, fmt.Errorf("unsupported data file format: %s", ext)
	}
}

// LoadDirectory loads all data files from a directory, keyed by filename.
func LoadDirectory(dir string) (map[string]interface{}, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	result := make(map[string]interface{})
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		ext := strings.ToLower(filepath.Ext(entry.Name()))
		name := strings.TrimSuffix(entry.Name(), filepath.Ext(entry.Name()))
		path := filepath.Join(dir, entry.Name())
		switch ext {
		case ".yaml", ".yml", ".toml", ".json":
			d, err := LoadFileAny(path)
			if err != nil {
				return nil, err
			}
			result[name] = d
		case ".csv":
			rows, err := LoadCSV(path)
			if err != nil {
				return nil, err
			}
			result[name] = rows
		}
	}
	return result, nil
}

// LoadCSV loads a CSV file and returns an array of maps (header row = keys).
func LoadCSV(path string) ([]map[string]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	r := csv.NewReader(f)
	records, err := r.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("parsing CSV %s: %w", path, err)
	}
	if len(records) < 2 {
		return nil, nil
	}
	headers := records[0]
	var rows []map[string]string
	for _, record := range records[1:] {
		row := make(map[string]string)
		for i, header := range headers {
			if i < len(record) {
				row[header] = record[i]
			}
		}
		rows = append(rows, row)
	}
	return rows, nil
}
