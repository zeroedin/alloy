package template

import (
	"fmt"
	"regexp"
	"strings"
)

var (
	blockShortcodeOpenRe  = regexp.MustCompile(`\{\{%\s+([a-zA-Z][a-zA-Z0-9_-]*)((?:\s+"[^"]*")*)\s*%\}\}`)
	blockShortcodeCloseRe = regexp.MustCompile(`\{\{%\s*/([a-zA-Z][a-zA-Z0-9_-]*)\s*%\}\}`)
	blockShortcodeArgRe   = regexp.MustCompile(`"([^"]*)"`)
	preRegionRe           = regexp.MustCompile(`(?si)<pre[^>]*>.*?</pre>`)
	codeRegionRe          = regexp.MustCompile(`(?si)<code[^>]*>.*?</code>`)
)

// BlockShortcodeCallback receives the shortcode name, quoted arguments, and
// inner HTML content. It returns the rendered replacement HTML.
type BlockShortcodeCallback func(name string, args []string, content string) (string, error)

// ProcessBlockShortcodes scans post-Goldmark HTML for Go template block
// shortcode pairs ({{% tag "args" %}}...{{% /tag %}}) and replaces each with
// the callback's output. Nesting is resolved innermost-first. Tags inside
// <pre> and <code> elements are treated as literal text and left untouched.
func ProcessBlockShortcodes(input []byte, callback BlockShortcodeCallback) ([]byte, error) {
	s := string(input)

	if !strings.Contains(s, "{{%") {
		return input, nil
	}

	for {
		excluded := findCodeRegions(s)

		// Find the first (leftmost) closing tag not inside code
		closeMatches := blockShortcodeCloseRe.FindAllStringSubmatchIndex(s, -1)
		var firstClose []int
		var closeName string
		for _, m := range closeMatches {
			if !inCodeRegion(m[0], excluded) {
				firstClose = m
				closeName = s[m[2]:m[3]]
				break
			}
		}

		if firstClose == nil {
			openMatches := blockShortcodeOpenRe.FindAllStringSubmatchIndex(s, -1)
			for _, m := range openMatches {
				if !inCodeRegion(m[0], excluded) {
					return nil, fmt.Errorf("unclosed block shortcode {%% %s %%}", s[m[2]:m[3]])
				}
			}
			break
		}

		// Find the nearest opening tag before the closing tag
		openMatches := blockShortcodeOpenRe.FindAllStringSubmatchIndex(s[:firstClose[0]], -1)
		var nearestOpen []int
		var openName string
		for i := len(openMatches) - 1; i >= 0; i-- {
			if !inCodeRegion(openMatches[i][0], excluded) {
				nearestOpen = openMatches[i]
				openName = s[nearestOpen[2]:nearestOpen[3]]
				break
			}
		}

		if nearestOpen == nil {
			return nil, fmt.Errorf("unexpected closing tag {%% /%s %%}", closeName)
		}
		if openName != closeName {
			return nil, fmt.Errorf("mismatched block shortcode: opened {%% %s %%} but closed with {%% /%s %%}", openName, closeName)
		}

		innerContent := s[nearestOpen[1]:firstClose[0]]

		var argsStr string
		if nearestOpen[4] != nearestOpen[5] {
			argsStr = s[nearestOpen[4]:nearestOpen[5]]
		}
		args := parseShortcodeArgs(argsStr)

		output, err := callback(openName, args, innerContent)
		if err != nil {
			return nil, fmt.Errorf("shortcode %q: %w", openName, err)
		}

		s = s[:nearestOpen[0]] + output + s[firstClose[1]:]
	}

	return []byte(s), nil
}

func parseShortcodeArgs(s string) []string {
	matches := blockShortcodeArgRe.FindAllStringSubmatch(s, -1)
	if matches == nil {
		return nil
	}
	args := make([]string, len(matches))
	for i, m := range matches {
		args[i] = m[1]
	}
	return args
}

func findCodeRegions(s string) [][2]int {
	var regions [][2]int
	for _, m := range preRegionRe.FindAllStringIndex(s, -1) {
		regions = append(regions, [2]int{m[0], m[1]})
	}
	for _, m := range codeRegionRe.FindAllStringIndex(s, -1) {
		regions = append(regions, [2]int{m[0], m[1]})
	}
	return regions
}

func inCodeRegion(pos int, regions [][2]int) bool {
	for _, r := range regions {
		if pos >= r[0] && pos < r[1] {
			return true
		}
	}
	return false
}
