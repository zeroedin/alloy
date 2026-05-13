package content

import (
	"bytes"
	"fmt"
	htmlstd "html"
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

// ── Custom AST node types for template tags ───────────────────────────

var KindTemplateTagInline = ast.NewNodeKind("TemplateTagInline")

type TemplateTagInline struct {
	ast.BaseInline
	TagText []byte
}

func (n *TemplateTagInline) Kind() ast.NodeKind { return KindTemplateTagInline }
func (n *TemplateTagInline) IsRaw() bool        { return true }
func (n *TemplateTagInline) Dump(source []byte, level int) {
	ast.DumpHelper(n, source, level, nil, nil)
}

var KindTemplateTagBlock = ast.NewNodeKind("TemplateTagBlock")

type TemplateTagBlock struct {
	ast.BaseBlock
	TagText []byte
}

func (n *TemplateTagBlock) Kind() ast.NodeKind { return KindTemplateTagBlock }
func (n *TemplateTagBlock) IsRaw() bool        { return true }
func (n *TemplateTagBlock) Dump(source []byte, level int) {
	ast.DumpHelper(n, source, level, nil, nil)
}

// ── Inline parser ─────────────────────────────────────────────────────

type templateTagInlineParser struct{}

func (p *templateTagInlineParser) Trigger() []byte {
	return []byte{'{'}
}

func (p *templateTagInlineParser) Parse(parent ast.Node, block text.Reader, pc parser.Context) ast.Node {
	_, seg := block.PeekLine()
	source := block.Source()
	start := seg.Start

	if start+3 >= len(source) {
		return nil
	}

	if source[start] != '{' {
		return nil
	}

	isExpression := source[start+1] == '{'
	isControl := source[start+1] == '%'
	if !isExpression && !isControl {
		return nil
	}

	var closer byte
	if isExpression {
		closer = '}'
	} else {
		closer = '%'
	}

	for i := start + 2; i < len(source)-1; i++ {
		if source[i] == '\n' && i+1 < len(source) && source[i+1] == '\n' {
			return nil
		}
		if !isExpression && source[i] == '-' && i+2 < len(source) && source[i+1] == closer && source[i+2] == '}' {
			end := i + 3
			tagText := make([]byte, end-start)
			copy(tagText, source[start:end])
			node := &TemplateTagInline{TagText: tagText}
			block.Advance(end - start)
			return node
		}
		if source[i] == closer && source[i+1] == '}' {
			end := i + 2
			tagText := make([]byte, end-start)
			copy(tagText, source[start:end])
			node := &TemplateTagInline{TagText: tagText}
			block.Advance(end - start)
			return node
		}
	}

	return nil
}

// ── Block parser ──────────────────────────────────────────────────────

type templateTagBlockParser struct{}

func (p *templateTagBlockParser) Trigger() []byte {
	return []byte{'{'}
}

func (p *templateTagBlockParser) Open(parent ast.Node, reader text.Reader, pc parser.Context) (ast.Node, parser.State) {
	line, _ := reader.PeekLine()
	trimmed := bytes.TrimSpace(line)

	if len(trimmed) < 4 || trimmed[0] != '{' || trimmed[1] != '%' {
		return nil, parser.NoChildren
	}

	closeIdx := bytes.Index(trimmed[2:], []byte("%}"))
	if closeIdx == -1 {
		return nil, parser.NoChildren
	}

	tagEnd := 2 + closeIdx + 2
	remaining := bytes.TrimSpace(trimmed[tagEnd:])
	if len(remaining) > 0 {
		return nil, parser.NoChildren
	}

	tagText := make([]byte, len(trimmed))
	copy(tagText, trimmed)
	node := &TemplateTagBlock{TagText: tagText}

	return node, parser.NoChildren
}

func (p *templateTagBlockParser) Continue(node ast.Node, reader text.Reader, pc parser.Context) parser.State {
	return parser.Close
}

func (p *templateTagBlockParser) Close(node ast.Node, reader text.Reader, pc parser.Context) {}

func (p *templateTagBlockParser) CanInterruptParagraph() bool { return true }

func (p *templateTagBlockParser) CanAcceptIndentedLine() bool { return false }

// ── Custom renderer ───────────────────────────────────────────────────

type templateTagRenderer struct{}

func (r *templateTagRenderer) RegisterFuncs(reg renderer.NodeRendererFuncRegisterer) {
	reg.Register(KindTemplateTagInline, r.renderInline)
	reg.Register(KindTemplateTagBlock, r.renderBlock)
}

func (r *templateTagRenderer) renderInline(w util.BufWriter, source []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	if entering {
		n := node.(*TemplateTagInline)
		_, _ = w.Write(n.TagText)
	}
	return ast.WalkContinue, nil
}

func (r *templateTagRenderer) renderBlock(w util.BufWriter, source []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	if entering {
		n := node.(*TemplateTagBlock)
		_, _ = w.Write(n.TagText)
		_ = w.WriteByte('\n')
	}
	return ast.WalkContinue, nil
}

// ── Extension ─────────────────────────────────────────────────────────

type templateTagsExtension struct{}

func (e *templateTagsExtension) Extend(m goldmark.Markdown) {
	m.Parser().AddOptions(
		parser.WithBlockParsers(
			util.Prioritized(&templateTagBlockParser{}, 50),
		),
		parser.WithInlineParsers(
			util.Prioritized(&templateTagInlineParser{}, 90),
		),
	)
	m.Renderer().AddOptions(
		renderer.WithNodeRenderers(
			util.Prioritized(&templateTagRenderer{}, 100),
		),
	)
}

// ── Goldmark configuration ───────────────────────────────────────────

// CreateGoldmark builds a configured goldmark instance from options.
// It configures extensions (Table, TaskList, Footnote, optionally Typographer),
// parser options (AutoHeadingID, Attribute), and renderer options (Unsafe).
// When TemplateTags is true, the templateTagsExtension is registered to handle
// {{ }} and {% %} patterns via custom AST nodes.
func CreateGoldmark(opts MarkdownOptions, extraParserOpts ...parser.Option) goldmark.Markdown {
	extensions := []goldmark.Extender{
		extension.Table,
		extension.TaskList,
		extension.NewFootnote(),
	}
	if opts.Typographer {
		extensions = append(extensions, extension.NewTypographer())
	}
	if opts.TemplateTags {
		extensions = append(extensions, &templateTagsExtension{})
	}

	rendererOpts := []renderer.Option{}
	if opts.Unsafe {
		rendererOpts = append(rendererOpts, html.WithUnsafe())
	}

	if len(opts.Hooks) > 0 {
		noHookOpts := opts
		noHookOpts.Hooks = nil
		childMD := CreateGoldmark(noHookOpts, extraParserOpts...)
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

// ── Rendering ─────────────────────────────────────────────────────────

// RenderMarkdown converts Markdown source to HTML.
func RenderMarkdown(source []byte, opts MarkdownOptions) ([]byte, error) {
	src := source
	if !opts.TemplateTags {
		src = escapeTemplateTags(source)
	}
	md := CreateGoldmark(opts)

	var buf bytes.Buffer
	if err := md.Convert(src, &buf); err != nil {
		return nil, fmt.Errorf("markdown render error: %w", err)
	}

	return buf.Bytes(), nil
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

// ── Table of contents ─────────────────────────────────────────────────

// TOCEntry represents a heading in the table of contents.
type TOCEntry struct {
	ID       string     `json:"id"`
	Text     string     `json:"text"`
	Level    int        `json:"level"`
	Children []TOCEntry `json:"children,omitempty"`
}

// RenderMarkdownWithTOC renders markdown and extracts a nested table of contents
// from the heading structure. h1 headings are excluded from the TOC.
// Auto heading IDs are always enabled regardless of opts.AutoHeadingID,
// since TOC entries require IDs to be useful.
func RenderMarkdownWithTOC(source []byte, opts MarkdownOptions) ([]byte, []TOCEntry, error) {
	src := source
	if !opts.TemplateTags {
		src = escapeTemplateTags(source)
	}

	extraOpts := []parser.Option{}
	if !opts.AutoHeadingID {
		extraOpts = append(extraOpts, parser.WithAutoHeadingID(), parser.WithAttribute())
	}
	md := CreateGoldmark(opts, extraOpts...)

	reader := text.NewReader(src)
	doc := md.Parser().Parse(reader)

	var buf bytes.Buffer
	if err := md.Renderer().Render(&buf, src, doc); err != nil {
		return nil, nil, fmt.Errorf("markdown render error: %w", err)
	}

	result := buf.Bytes()

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
		} else if t, ok := child.(*TemplateTagInline); ok {
			buf.Write(t.TagText)
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
	buf.WriteString(htmlstd.EscapeString(string(source)))
	buf.WriteString("</pre>")
	return buf.Bytes(), nil
}
