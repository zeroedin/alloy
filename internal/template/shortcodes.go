package template

import (
	"fmt"
	"regexp"
	"strings"
)

// ShortcodeFunc handles inline shortcodes (no inner content).
type ShortcodeFunc func(args []string) string

// BlockShortcodeFunc handles block shortcodes (with inner content).
type BlockShortcodeFunc func(args []string, content string) string

// shortcode registries
var inlineShortcodes = map[string]ShortcodeFunc{}
var blockShortcodes = map[string]BlockShortcodeFunc{}

// RegisterShortcode registers a shortcode that works in both Liquid and Go template engines.
func RegisterShortcode(name string, fn ShortcodeFunc) error {
	inlineShortcodes[name] = fn
	return nil
}

// RegisterBlockShortcode registers a block shortcode (with inner content).
func RegisterBlockShortcode(name string, fn BlockShortcodeFunc) error {
	blockShortcodes[name] = fn
	return nil
}

// shortcodeArgPattern matches quoted arguments in a shortcode tag.
var shortcodeArgPattern = regexp.MustCompile(`"([^"]*)"`)

// RenderShortcodes processes shortcode tags in template source and expands them.
// Handles both inline {% name "arg" %} and block {% name %}...{% endname %} syntax.
func RenderShortcodes(source string) (string, error) {
	result := source

	// Process block shortcodes first ({% name "args" %}content{% endname %})
	for name, fn := range blockShortcodes {
		pattern := regexp.MustCompile(
			`\{%\s*` + regexp.QuoteMeta(name) + `((?:\s+"[^"]*")*)\s*%\}(.*?)\{%\s*end` + regexp.QuoteMeta(name) + `\s*%\}`,
		)
		result = pattern.ReplaceAllStringFunc(result, func(match string) string {
			submatches := pattern.FindStringSubmatch(match)
			if len(submatches) < 3 {
				return match
			}
			args := parseShortcodeArgs(submatches[1])
			content := submatches[2]
			return fn(args, content)
		})
	}

	// Process inline shortcodes ({% name "args" %})
	for name, fn := range inlineShortcodes {
		pattern := regexp.MustCompile(
			`\{%\s*` + regexp.QuoteMeta(name) + `((?:\s+"[^"]*")*)\s*%\}`,
		)
		// Avoid matching block shortcodes that have already been processed
		result = pattern.ReplaceAllStringFunc(result, func(match string) string {
			// Skip if this is part of an end tag
			if strings.Contains(match, "end"+name) {
				return match
			}
			submatches := pattern.FindStringSubmatch(match)
			if len(submatches) < 2 {
				return match
			}
			args := parseShortcodeArgs(submatches[1])
			return fn(args)
		})
	}

	return result, nil
}

// parseShortcodeArgs extracts quoted arguments from a shortcode tag.
func parseShortcodeArgs(argStr string) []string {
	matches := shortcodeArgPattern.FindAllStringSubmatch(argStr, -1)
	args := make([]string, 0, len(matches))
	for _, m := range matches {
		if len(m) >= 2 {
			args = append(args, m[1])
		}
	}
	return args
}

// ResetShortcodes clears all registered shortcodes (useful for testing).
func ResetShortcodes() {
	inlineShortcodes = map[string]ShortcodeFunc{}
	blockShortcodes = map[string]BlockShortcodeFunc{}
}

// GetShortcode returns a registered inline shortcode by name.
func GetShortcode(name string) (ShortcodeFunc, error) {
	fn, ok := inlineShortcodes[name]
	if !ok {
		return nil, fmt.Errorf("shortcode %q not registered", name)
	}
	return fn, nil
}
