package content

// MarkdownOptions controls goldmark rendering behavior.
type MarkdownOptions struct {
	Unsafe       bool
	Typographer  bool
	TemplateTags bool
}

// RenderMarkdown converts Markdown source to HTML.
func RenderMarkdown(source []byte, opts MarkdownOptions) ([]byte, error) {
	return nil, ErrNotImplemented
}

// RenderText wraps plain text content in <pre> tags for .txt files.
func RenderText(source []byte) ([]byte, error) {
	return nil, ErrNotImplemented
}
