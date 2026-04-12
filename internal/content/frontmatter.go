package content

// ParseFrontMatter extracts front matter and body from raw content.
// Detects YAML (---), TOML (+++), or JSON ({) delimiters.
func ParseFrontMatter(raw []byte) (map[string]interface{}, []byte, error) {
	return nil, nil, ErrNotImplemented
}

// BuildPage parses raw content bytes and returns a fully populated Page.
// Extracts front matter, populates Summary from front matter "summary" field,
// and sets other Page fields from front matter values.
func BuildPage(relPath string, raw []byte) (*Page, error) {
	return nil, ErrNotImplemented
}
