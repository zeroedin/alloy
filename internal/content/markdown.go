package content

import (
	"bytes"
	"fmt"
	"regexp"
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/renderer"
	"github.com/yuin/goldmark/renderer/html"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
)

// HookRenderer renders a hook template with the given context variables.
// source is the template source and ctx contains the template variables
// (e.g. {"markup": {...}}).
type HookRenderer func(source string, ctx map[string]interface{}) (string, error)

// MarkdownOptions controls goldmark rendering behavior.
type MarkdownOptions struct {
	Unsafe        bool
	Typographer   bool
	TemplateTags  bool
	AutoHeadingID bool
	Hooks         map[string]string
	HookRenderer  HookRenderer
}

// templateTagPattern matches {{ ... }} and {% ... %} template expressions,
// including those containing newlines (e.g., {{ "hello\nworld" | filter }}).
var templateTagPattern = regexp.MustCompile(`(?s)(\{\{.*?\}\}|\{%.*?%\})`)

// createGoldmark builds a configured goldmark instance from options.
func createGoldmark(opts MarkdownOptions, extraParserOpts ...parser.Option) goldmark.Markdown {
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

	if len(opts.Hooks) > 0 {
		noHookOpts := opts
		noHookOpts.Hooks = nil
		childMD := createGoldmark(noHookOpts, extraParserOpts...)
		hookRenderer := newHookNodeRenderer(opts.Hooks, childMD.Renderer(), opts.HookRenderer)
		rendererOpts = append(rendererOpts, renderer.WithNodeRenderers(util.Prioritized(hookRenderer, 100)))
	}

	parserOpts := []parser.Option{}
	if opts.AutoHeadingID {
		parserOpts = append(parserOpts, parser.WithAutoHeadingID(), parser.WithAttribute())
	}
	parserOpts = append(parserOpts, extraParserOpts...)

	return goldmark.New(
		goldmark.WithExtensions(extensions...),
		goldmark.WithRendererOptions(rendererOpts...),
		goldmark.WithParserOptions(parserOpts...),
	)
}

// preprocessSource handles template tag protection/escaping before goldmark processing.
func preprocessSource(source []byte, opts MarkdownOptions) ([]byte, []string) {
	if opts.TemplateTags {
		return protectTemplateTags(source)
	}
	return escapeTemplateTags(source), nil
}

// RenderMarkdown converts Markdown source to HTML.
func RenderMarkdown(source []byte, opts MarkdownOptions) ([]byte, error) {
	src, placeholders := preprocessSource(source, opts)
	md := createGoldmark(opts)

	var buf bytes.Buffer
	if err := md.Convert(src, &buf); err != nil {
		return nil, fmt.Errorf("markdown render error: %w", err)
	}

	result := buf.Bytes()
	if len(placeholders) > 0 {
		result = restoreTemplateTags(result, placeholders)
	}

	return result, nil
}

// protectTemplateTags replaces template tags with unique placeholders.
// It handles both inline code and fenced code blocks by processing
// the entire source — tags inside code spans/blocks get placeholders too,
// which goldmark will render inside <code> elements, and then we restore them.
// blockShortcodeLineRe matches a line that contains exactly one {% %} tag
// (with optional leading/trailing whitespace). Only single-line tags qualify
// as block shortcodes — multi-tag lines and control-flow lines are left inline.
var blockShortcodeLineRe = regexp.MustCompile(`(?m)^([ \t]*)(\{%-?[^\n]*?-?%\})[ \t]*$`)

func protectTemplateTags(src []byte) ([]byte, []string) {
	var placeholders []string

	// First pass: replace block-level {% %} tags with placeholders surrounded
	// by blank lines so goldmark treats them as separate blocks.
	// Preserve leading indentation for list/blockquote context.
	result := blockShortcodeLineRe.ReplaceAllFunc(src, func(match []byte) []byte {
		trimmed := bytes.TrimSpace(match)
		// Detect leading indentation
		indent := match[:bytes.Index(match, []byte("{%"))]
		idx := len(placeholders)
		placeholders = append(placeholders, string(trimmed))
		placeholder := fmt.Sprintf("\n\n%sALLOY_TPL_%d_ELPMT\n\n", string(indent), idx)
		return []byte(placeholder)
	})

	// Second pass: replace remaining inline template tags with text placeholders
	result = templateTagPattern.ReplaceAllFunc(result, func(match []byte) []byte {
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

// TOCEntry represents a heading in the table of contents.
type TOCEntry struct {
	ID       string
	Text     string
	Level    int
	Children []TOCEntry
}

// RenderMarkdownWithTOC renders markdown and extracts a nested table of contents
// from the heading structure. h1 headings are excluded from the TOC.
// Auto heading IDs are always enabled regardless of opts.AutoHeadingID,
// since TOC entries require IDs to be useful.
func RenderMarkdownWithTOC(source []byte, opts MarkdownOptions) ([]byte, []TOCEntry, error) {
	src, placeholders := preprocessSource(source, opts)

	extraOpts := []parser.Option{}
	if !opts.AutoHeadingID {
		extraOpts = append(extraOpts, parser.WithAutoHeadingID(), parser.WithAttribute())
	}
	md := createGoldmark(opts, extraOpts...)

	reader := text.NewReader(src)
	doc := md.Parser().Parse(reader)

	var buf bytes.Buffer
	if err := md.Renderer().Render(&buf, src, doc); err != nil {
		return nil, nil, fmt.Errorf("markdown render error: %w", err)
	}

	result := buf.Bytes()
	if len(placeholders) > 0 {
		result = restoreTemplateTags(result, placeholders)
	}

	var flat []TOCEntry
	ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		heading, ok := n.(*ast.Heading)
		if !ok || heading.Level < 2 {
			return ast.WalkContinue, nil
		}

		id := ""
		if rawID, found := heading.AttributeString("id"); found {
			id = string(rawID.([]byte))
		}

		var textBuf bytes.Buffer
		extractText(&textBuf, heading, src)

		flat = append(flat, TOCEntry{
			ID:    id,
			Text:  textBuf.String(),
			Level: heading.Level,
		})
		return ast.WalkContinue, nil
	})

	toc := nestTOCEntries(flat)
	return result, toc, nil
}

// extractText recursively collects all text content from an AST node's subtree.
func extractText(buf *bytes.Buffer, node ast.Node, source []byte) {
	for child := node.FirstChild(); child != nil; child = child.NextSibling() {
		if t, ok := child.(*ast.Text); ok {
			buf.Write(t.Segment.Value(source))
		} else {
			extractText(buf, child, source)
		}
	}
}

func nestTOCEntries(flat []TOCEntry) []TOCEntry {
	if len(flat) == 0 {
		return nil
	}

	var result []TOCEntry
	var stack []*TOCEntry

	for i := range flat {
		entry := flat[i]

		for len(stack) > 0 && stack[len(stack)-1].Level >= entry.Level {
			stack = stack[:len(stack)-1]
		}

		if len(stack) == 0 {
			result = append(result, entry)
			stack = []*TOCEntry{&result[len(result)-1]}
		} else {
			parent := stack[len(stack)-1]
			parent.Children = append(parent.Children, entry)
			stack = append(stack, &parent.Children[len(parent.Children)-1])
		}
	}

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
