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
// blockShortcodeLineRe matches a line that contains only a {% %} tag
// (with optional leading/trailing whitespace). These are block shortcodes
// and must be treated as block-level elements by goldmark.
var blockShortcodeLineRe = regexp.MustCompile(`(?m)^[ \t]*(\{%-?[\s\S]*?-?%\})[ \t]*$`)

func protectTemplateTags(src []byte) ([]byte, []string) {
	var placeholders []string

	// First pass: replace block-level {% %} tags with placeholders surrounded
	// by blank lines so goldmark treats them as separate blocks.
	result := blockShortcodeLineRe.ReplaceAllFunc(src, func(match []byte) []byte {
		trimmed := bytes.TrimSpace(match)
		idx := len(placeholders)
		placeholders = append(placeholders, string(trimmed))
		placeholder := fmt.Sprintf("\n\nALLOY_TPL_%d_ELPMT\n\n", idx)
		return []byte(placeholder)
	})

	// Second pass: replace remaining inline template tags with text placeholders
	result = templateTagPattern.ReplaceAllFunc(result, func(match []byte) []byte {
		if bytes.Contains(match, []byte("ALLOY_TPL_")) {
			return match
		}
		idx := len(placeholders)
		placeholders = append(placeholders, string(match))
		placeholder := fmt.Sprintf("ALLOY_TPL_%d_ELPMT", idx)
		return []byte(placeholder)
	})
	return result, placeholders
}



// restoreTemplateTags replaces placeholders back with the original template tags.
// Block-level placeholders end up in their own <p> tags from goldmark — strip
// the <p> wrapper to leave the raw template tag at block level.
func restoreTemplateTags(html []byte, placeholders []string) []byte {
	result := string(html)
	for i, original := range placeholders {
		placeholder := fmt.Sprintf("ALLOY_TPL_%d_ELPMT", i)
		// Strip <p> wrapper if the placeholder is the sole content of a paragraph
		wrapped := "<p>" + placeholder + "</p>"
		if strings.Contains(result, wrapped) {
			result = strings.ReplaceAll(result, wrapped, original)
		} else {
			result = strings.ReplaceAll(result, placeholder, original)
		}
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
