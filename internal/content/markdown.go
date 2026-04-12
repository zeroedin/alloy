package content

import (
	"bytes"
	"fmt"
	"regexp"
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/renderer"
	"github.com/yuin/goldmark/renderer/html"
)

// MarkdownOptions controls goldmark rendering behavior.
type MarkdownOptions struct {
	Unsafe       bool
	Typographer  bool
	TemplateTags bool
}

// templateTagPattern matches {{ ... }} and {% ... %} template expressions.
var templateTagPattern = regexp.MustCompile(`(\{\{.*?\}\}|\{%.*?%\})`)

// RenderMarkdown converts Markdown source to HTML.
func RenderMarkdown(source []byte, opts MarkdownOptions) ([]byte, error) {
	src := source

	// Template tag preservation: replace {{ }} and {% %} with placeholders
	// before goldmark processing, then restore them after.
	var placeholders []string
	if opts.TemplateTags {
		src, placeholders = protectTemplateTags(src)
	} else {
		// When template tags are disabled, escape braces so they don't
		// pass through as literal template syntax.
		src = escapeTemplateTags(src)
	}

	// Build goldmark with extensions
	extensions := []goldmark.Extender{
		extension.Table,
		extension.TaskList,
		extension.NewFootnote(),
	}
	if opts.Typographer {
		extensions = append(extensions, extension.NewTypographer())
	}

	rendererOpts := []renderer.Option{}
	if opts.Unsafe {
		rendererOpts = append(rendererOpts, html.WithUnsafe())
	}

	md := goldmark.New(
		goldmark.WithExtensions(extensions...),
		goldmark.WithRendererOptions(rendererOpts...),
	)

	var buf bytes.Buffer
	if err := md.Convert(src, &buf); err != nil {
		return nil, fmt.Errorf("markdown render error: %w", err)
	}

	result := buf.Bytes()

	// Restore template tags from placeholders
	if opts.TemplateTags && len(placeholders) > 0 {
		result = restoreTemplateTags(result, placeholders)
	}

	return result, nil
}

// protectTemplateTags replaces template tags with unique placeholders.
// It handles both inline code and fenced code blocks by processing
// the entire source — tags inside code spans/blocks get placeholders too,
// which goldmark will render inside <code> elements, and then we restore them.
func protectTemplateTags(src []byte) ([]byte, []string) {
	var placeholders []string
	result := templateTagPattern.ReplaceAllFunc(src, func(match []byte) []byte {
		idx := len(placeholders)
		placeholders = append(placeholders, string(match))
		placeholder := fmt.Sprintf("ALLOY_TPL_%d_ELPMT", idx)
		return []byte(placeholder)
	})
	return result, placeholders
}

// restoreTemplateTags replaces placeholders back with the original template tags.
func restoreTemplateTags(html []byte, placeholders []string) []byte {
	result := string(html)
	for i, original := range placeholders {
		placeholder := fmt.Sprintf("ALLOY_TPL_%d_ELPMT", i)
		result = strings.ReplaceAll(result, placeholder, original)
	}
	return []byte(result)
}

// escapeTemplateTags inserts zero-width spaces between consecutive braces
// so they don't survive as literal template syntax when preservation is disabled.
func escapeTemplateTags(src []byte) []byte {
	result := templateTagPattern.ReplaceAllFunc(src, func(match []byte) []byte {
		s := string(match)
		// Insert zero-width space between {{ and }} / {% and %}
		// so the template engine won't recognize them.
		s = strings.ReplaceAll(s, "{{", "{\u200B{")
		s = strings.ReplaceAll(s, "}}", "}\u200B}")
		s = strings.ReplaceAll(s, "{%", "{\u200B%")
		s = strings.ReplaceAll(s, "%}", "%\u200B}")
		return []byte(s)
	})
	return result
}

// RenderText wraps plain text content in <pre> tags for .txt files.
func RenderText(source []byte) ([]byte, error) {
	var buf bytes.Buffer
	buf.WriteString("<pre>")
	buf.Write(source)
	buf.WriteString("</pre>")
	return buf.Bytes(), nil
}
