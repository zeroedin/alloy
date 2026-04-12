package template

import "errors"

// ErrNotImplemented is returned by all stub functions.
var ErrNotImplemented = errors.New("not implemented")

// FilterFunc is the signature for template filter functions.
type FilterFunc func(input interface{}, args ...interface{}) interface{}

// TagFunc is the signature for custom template tag/shortcode functions.
type TagFunc func(args []string, content string) string

// TemplateEngine is the abstraction over Liquid and Go template engines.
type TemplateEngine interface {
	Parse(name string, content []byte) (Template, error)
	AddFilter(name string, fn FilterFunc) error
	AddTag(name string, fn TagFunc) error
}

// Template is a parsed template that can be rendered with a context.
type Template interface {
	Render(ctx map[string]interface{}) ([]byte, error)
}
