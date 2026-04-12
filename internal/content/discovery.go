package content

import "errors"

// ErrNotImplemented is returned by all stub functions.
var ErrNotImplemented = errors.New("not implemented")

// Discover walks the content directory and returns all content pages.
func Discover(contentDir string) ([]*Page, error) {
	return nil, ErrNotImplemented
}

// DiscoverWithFormats walks the content directory and returns only pages
// whose file extension matches one of the allowed formats (e.g., ["md", "html"]).
// Files with extensions not in the list are ignored.
func DiscoverWithFormats(contentDir string, formats []string) ([]*Page, error) {
	return nil, ErrNotImplemented
}
