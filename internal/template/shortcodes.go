package template

// ShortcodeFunc handles inline shortcodes (no inner content).
type ShortcodeFunc func(args []string) string

// BlockShortcodeFunc handles block shortcodes (with inner content).
type BlockShortcodeFunc func(args []string, content string) string

// RegisterShortcode registers a shortcode that works in both Liquid and Go template engines.
func RegisterShortcode(name string, fn ShortcodeFunc) error {
	return ErrNotImplemented
}

// RegisterBlockShortcode registers a block shortcode (with inner content).
func RegisterBlockShortcode(name string, fn BlockShortcodeFunc) error {
	return ErrNotImplemented
}

// RenderShortcodes processes shortcode tags in template source and expands them.
// Handles both inline {% name "arg" %} and block {% name %}...{% endname %} syntax.
func RenderShortcodes(source string) (string, error) {
	return "", ErrNotImplemented
}
