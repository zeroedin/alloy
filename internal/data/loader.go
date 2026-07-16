package data

import (
	"encoding/csv"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/zeroedin/alloy/internal/ordered"
	"gopkg.in/yaml.v3"
)

// LoadFile loads a single data file (YAML, TOML, JSON) and returns its contents as a map.
// For files with root-level arrays, use LoadFileAny instead.
func LoadFile(path string) (map[string]interface{}, error) {
	v, err := LoadFileAny(path)
	if err != nil {
		return nil, err
	}
	switch val := v.(type) {
	case map[string]interface{}:
		return val, nil
	case *ordered.Map:
		return val.ToGoMap(), nil
	default:
		return nil, fmt.Errorf("parsing %s: expected map at root level, got %T", path, v)
	}
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
		result, err := ordered.UnmarshalJSONValue(b)
		if err != nil {
			return nil, fmt.Errorf("parsing JSON %s: %w", path, err)
		}
		return result, nil
	default:
		return nil, fmt.Errorf("unsupported data file format: %s", ext)
	}
}

// LoadDirectory loads all data files from a directory, keyed by filename stem.
// Subdirectories are recursed into and produce nested namespace maps:
// data/nav/main.yaml → result["nav"]["main"].
// Returns an error if two files share a stem name (e.g., team.csv and team.yaml)
// or if a file and subdirectory share the same stem (e.g., nav.yaml and nav/).
func LoadDirectory(dir string) (map[string]interface{}, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	result := make(map[string]interface{})
	seen := make(map[string]string) // stem → first filename or dirname

	// Process files first, then directories, so collision detection
	// catches file-vs-directory conflicts regardless of readdir order.
	var dirs []os.DirEntry
	for _, entry := range entries {
		if entry.IsDir() {
			dirs = append(dirs, entry)
			continue
		}
		ext := strings.ToLower(filepath.Ext(entry.Name()))
		name := strings.TrimSuffix(entry.Name(), filepath.Ext(entry.Name()))
		path := filepath.Join(dir, entry.Name())
		switch ext {
		case ".yaml", ".yml", ".toml", ".json":
			if prev, ok := seen[name]; ok {
				return nil, fmt.Errorf("data file stem conflict: %q and %q both produce key %q", prev, entry.Name(), name)
			}
			seen[name] = entry.Name()
			d, err := LoadFileAny(path)
			if err != nil {
				return nil, err
			}
			result[name] = d
		case ".csv":
			if prev, ok := seen[name]; ok {
				return nil, fmt.Errorf("data file stem conflict: %q and %q both produce key %q", prev, entry.Name(), name)
			}
			seen[name] = entry.Name()
			rows, err := LoadCSV(path)
			if err != nil {
				return nil, err
			}
			result[name] = rows
		}
	}

	for _, entry := range dirs {
		name := entry.Name()
		if prev, ok := seen[name]; ok {
			return nil, fmt.Errorf("data file stem conflict: %q and directory %q both produce key %q", prev, name+"/", name)
		}
		subDir := filepath.Join(dir, name)
		sub, err := LoadDirectory(subDir)
		if err != nil {
			return nil, err
		}
		if len(sub) == 0 {
			continue
		}
		seen[name] = name + "/"
		result[name] = sub
	}

	return result, nil
}

// LoadExternalFiles loads individual data files by key→path mapping.
// Paths must be relative and are resolved against projectRoot. Keys are
// processed in sorted order for deterministic error reporting.
func LoadExternalFiles(files map[string]string, projectRoot string) (map[string]interface{}, error) {
	if len(files) == 0 {
		return nil, nil
	}
	keys := make([]string, 0, len(files))
	for k := range files {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	result := make(map[string]interface{}, len(files))
	for _, key := range keys {
		relPath := files[key]
		if key == "" || relPath == "" {
			return nil, fmt.Errorf("data.files: empty key or path (key=%q, path=%q)", key, relPath)
		}
		if filepath.IsAbs(relPath) {
			return nil, fmt.Errorf("data.files %q: path must be relative, got %q", key, relPath)
		}
		absPath := filepath.Join(projectRoot, relPath)
		v, err := LoadFileAny(absPath)
		if err != nil {
			return nil, fmt.Errorf("loading data file %q (%s): %w", key, relPath, err)
		}
		result[key] = v
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
