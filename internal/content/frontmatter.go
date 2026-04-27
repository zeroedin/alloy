package content

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
	"gopkg.in/yaml.v3"
)

// HasFrontMatter returns true if raw content starts with a recognized
// front matter delimiter (---, +++, or {).
func HasFrontMatter(raw []byte) bool {
	s := strings.TrimLeft(string(raw), " \t\r\n")
	return strings.HasPrefix(s, "---") || strings.HasPrefix(s, "+++") || strings.HasPrefix(s, "{")
}

// ParseFrontMatter extracts front matter and body from raw content.
// Detects YAML (---), TOML (+++), or JSON ({) delimiters.
func ParseFrontMatter(raw []byte) (map[string]interface{}, []byte, error) {
	s := string(raw)

	// Detect delimiter type
	if strings.HasPrefix(s, "---") {
		return parseYAMLFrontMatter(s)
	}
	if strings.HasPrefix(s, "+++") {
		return parseTOMLFrontMatter(s)
	}
	if strings.HasPrefix(s, "{") {
		return parseJSONFrontMatter(s)
	}

	return nil, nil, fmt.Errorf("missing front matter delimiters: content files require front matter. Add empty front matter with ---\\n--- if no metadata is needed")
}

func parseYAMLFrontMatter(s string) (map[string]interface{}, []byte, error) {
	// Find the closing ---
	rest := s[3:] // skip opening ---
	// Skip newline after opening ---
	if len(rest) > 0 && rest[0] == '\n' {
		rest = rest[1:]
	}

	idx := strings.Index(rest, "\n---")
	if idx < 0 {
		// Check if the whole thing is just ---\n---
		if strings.TrimSpace(rest) == "---" || rest == "---\n" || rest == "---" {
			return map[string]interface{}{}, nil, nil
		}
		// The closing --- might be at the very start (empty front matter)
		if strings.HasPrefix(rest, "---") {
			body := strings.TrimPrefix(rest, "---")
			if len(body) > 0 && body[0] == '\n' {
				body = body[1:]
			}
			return map[string]interface{}{}, []byte(body), nil
		}
		return nil, nil, fmt.Errorf("missing closing front matter delimiter (---)")
	}

	fmContent := rest[:idx]
	body := rest[idx+4:] // skip \n---
	if len(body) > 0 && body[0] == '\n' {
		body = body[1:]
	}

	fm := make(map[string]interface{})
	if strings.TrimSpace(fmContent) == "" {
		return fm, []byte(body), nil
	}

	if err := yaml.Unmarshal([]byte(fmContent), &fm); err != nil {
		return nil, nil, fmt.Errorf("yaml parse error: %w", err)
	}

	// Convert yaml-specific types
	fm = normalizeYAMLMap(fm)

	return fm, []byte(body), nil
}

func parseTOMLFrontMatter(s string) (map[string]interface{}, []byte, error) {
	rest := s[3:] // skip opening +++
	if len(rest) > 0 && rest[0] == '\n' {
		rest = rest[1:]
	}

	idx := strings.Index(rest, "\n+++")
	if idx < 0 {
		if strings.HasPrefix(rest, "+++") {
			body := strings.TrimPrefix(rest, "+++")
			if len(body) > 0 && body[0] == '\n' {
				body = body[1:]
			}
			return map[string]interface{}{}, []byte(body), nil
		}
		return nil, nil, fmt.Errorf("missing closing front matter delimiter (+++)")
	}

	fmContent := rest[:idx]
	body := rest[idx+4:] // skip \n+++
	if len(body) > 0 && body[0] == '\n' {
		body = body[1:]
	}

	fm := make(map[string]interface{})
	if err := toml.Unmarshal([]byte(fmContent), &fm); err != nil {
		return nil, nil, fmt.Errorf("toml parse error: %w", err)
	}

	return fm, []byte(body), nil
}

func parseJSONFrontMatter(s string) (map[string]interface{}, []byte, error) {
	// Find the closing }
	// We need to handle nested braces
	depth := 0
	endIdx := -1
	for i, ch := range s {
		if ch == '{' {
			depth++
		} else if ch == '}' {
			depth--
			if depth == 0 {
				endIdx = i
				break
			}
		}
	}
	if endIdx < 0 {
		return nil, nil, fmt.Errorf("missing closing front matter delimiter (})")
	}

	jsonContent := s[:endIdx+1]
	body := s[endIdx+1:]
	if len(body) > 0 && body[0] == '\n' {
		body = body[1:]
	}

	fm := make(map[string]interface{})
	if err := json.Unmarshal([]byte(jsonContent), &fm); err != nil {
		return nil, nil, fmt.Errorf("json parse error: %w", err)
	}

	return fm, []byte(body), nil
}

// normalizeYAMLMap converts yaml-specific types to standard Go types.
func normalizeYAMLMap(m map[string]interface{}) map[string]interface{} {
	for k, v := range m {
		m[k] = normalizeYAMLValue(v)
	}
	return m
}

func normalizeYAMLValue(v interface{}) interface{} {
	switch val := v.(type) {
	case map[string]interface{}:
		return normalizeYAMLMap(val)
	case map[interface{}]interface{}:
		result := make(map[string]interface{})
		for k, v := range val {
			result[fmt.Sprintf("%v", k)] = normalizeYAMLValue(v)
		}
		return result
	case []interface{}:
		for i, item := range val {
			val[i] = normalizeYAMLValue(item)
		}
		return val
	default:
		return v
	}
}

// BuildPage parses raw content bytes and returns a fully populated Page.
func BuildPage(relPath string, raw []byte) (*Page, error) {
	fm, body, err := ParseFrontMatter(raw)
	if err != nil {
		// .txt files may not have front matter — treat entire content as body
		if strings.HasSuffix(relPath, ".txt") {
			fm = map[string]interface{}{}
			body = raw
		} else {
			return nil, fmt.Errorf("%s: %w", relPath, err)
		}
	}

	page := &Page{
		RelPath:     relPath,
		FrontMatter: fm,
		Body:        body,
		Content:     raw,
	}

	// Populate fields from front matter
	if title, ok := fm["title"].(string); ok {
		_ = title // stored in FrontMatter
	}
	if summary, ok := fm["summary"].(string); ok {
		page.Summary = summary
	}
	if draft, ok := fm["draft"].(bool); ok {
		page.Draft = draft
	}
	if layout, ok := fm["layout"].(string); ok {
		page.Layout = layout
	}
	if permalink, ok := fm["permalink"].(string); ok {
		page.Permalink = permalink
	}
	if slug, ok := fm["slug"].(string); ok {
		page.Slug = slug
	}

	// Parse date
	if dateVal, ok := fm["date"]; ok {
		page.Date = parseDate(dateVal)
	}

	// Parse publishDate
	if pd, ok := fm["publishDate"]; ok {
		t := parseDate(pd)
		if !t.IsZero() {
			page.PublishDate = &t
		}
	}

	// Parse expiryDate
	if ed, ok := fm["expiryDate"]; ok {
		t := parseDate(ed)
		if !t.IsZero() {
			page.ExpiryDate = &t
		}
	}

	// Parse aliases
	if aliases, ok := fm["aliases"]; ok {
		if arr, ok := aliases.([]interface{}); ok {
			for _, a := range arr {
				if s, ok := a.(string); ok {
					page.Aliases = append(page.Aliases, s)
				}
			}
		}
	}

	// Parse outputs
	if outputs, ok := fm["outputs"]; ok {
		if arr, ok := outputs.([]interface{}); ok {
			for _, o := range arr {
				if s, ok := o.(string); ok {
					page.Outputs = append(page.Outputs, s)
				}
			}
		}
	}

	// Parse pagination
	if pagination, ok := fm["pagination"].(map[string]interface{}); ok {
		pfm := &PaginationFrontMatter{}
		if d, ok := pagination["data"].(string); ok {
			pfm.Data = d
		}
		if pp, ok := pagination["perPage"]; ok {
			pfm.PerPage = toInt(pp)
		}
		if as, ok := pagination["as"].(string); ok {
			pfm.As = as
		}
		page.Pagination = pfm
	}

	return page, nil
}

func parseDate(v interface{}) time.Time {
	switch val := v.(type) {
	case time.Time:
		return val
	case string:
		layouts := []string{
			time.RFC3339,
			"2006-01-02T15:04:05Z07:00",
			"2006-01-02 15:04:05",
			"2006-01-02",
		}
		for _, layout := range layouts {
			t, err := time.Parse(layout, val)
			if err == nil {
				return t
			}
		}
	}
	return time.Time{}
}

func toInt(v interface{}) int {
	switch n := v.(type) {
	case int:
		return n
	case int64:
		return int(n)
	case float64:
		return int(n)
	}
	return 0
}

// SplitAtDelimiter splits raw bytes at a delimiter line, returning content before and after.
func SplitAtDelimiter(raw []byte, delim string) (before, after []byte, found bool) {
	delimBytes := []byte(delim)
	idx := bytes.Index(raw, delimBytes)
	if idx < 0 {
		return raw, nil, false
	}
	return raw[:idx], raw[idx+len(delimBytes):], true
}
