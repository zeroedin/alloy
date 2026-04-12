package data

import "errors"

// ErrNotImplemented is returned by all stub functions.
var ErrNotImplemented = errors.New("not implemented")

// LoadFile loads a single data file (YAML, TOML, JSON) and returns its contents.
func LoadFile(path string) (map[string]interface{}, error) {
	return nil, ErrNotImplemented
}

// LoadDirectory loads all data files from a directory, keyed by filename.
func LoadDirectory(dir string) (map[string]interface{}, error) {
	return nil, ErrNotImplemented
}

// LoadCSV loads a CSV file and returns an array of maps (header row = keys).
func LoadCSV(path string) ([]map[string]string, error) {
	return nil, ErrNotImplemented
}
