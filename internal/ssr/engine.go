package ssr

// SSREngine is the interface for server-side rendering engines.
type SSREngine interface {
	Render(html string) (string, error)
}
