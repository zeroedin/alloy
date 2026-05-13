package template

import (
	"fmt"
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
