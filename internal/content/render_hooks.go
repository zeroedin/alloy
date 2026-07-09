package content

import (
	"bytes"
	"fmt"
	"regexp"
	"strings"
	"unicode"

	"github.com/yuin/goldmark/ast"
	extast "github.com/yuin/goldmark/extension/ast"
	"github.com/yuin/goldmark/renderer"
	"github.com/yuin/goldmark/util"
)

type hookNodeRenderer struct {
	hooks         map[string]string
	childRenderer renderer.Renderer
	renderHook    HookRenderer
}

func newHookNodeRenderer(hooks map[string]string, childRenderer renderer.Renderer, renderHook HookRenderer) *hookNodeRenderer {
	return &hookNodeRenderer{hooks: hooks, childRenderer: childRenderer, renderHook: renderHook}
}

func (r *hookNodeRenderer) RegisterFuncs(reg renderer.NodeRendererFuncRegisterer) {
	if hasAnyCodeblockHook(r.hooks) {
		reg.Register(ast.KindFencedCodeBlock, r.renderFencedCodeBlock)
	}
	if _, ok := r.hooks["link"]; ok {
		reg.Register(ast.KindLink, r.renderLink)
	}
	if _, ok := r.hooks["heading"]; ok {
		reg.Register(ast.KindHeading, r.renderHeading)
	}
	if _, ok := r.hooks["image"]; ok {
		reg.Register(ast.KindImage, r.renderImage)
	}
	if _, ok := r.hooks["blockquote"]; ok {
		reg.Register(ast.KindBlockquote, r.renderBlockquote)
	}
	if _, ok := r.hooks["table"]; ok {
		reg.Register(extast.KindTable, r.renderTable)
	}
}

func hasAnyCodeblockHook(hooks map[string]string) bool {
	for key := range hooks {
		if key == "codeblock" || strings.HasPrefix(key, "codeblock-") {
			return true
		}
	}
	return false
}

func (r *hookNodeRenderer) renderFencedCodeBlock(
	w util.BufWriter, source []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	if !entering {
		return ast.WalkContinue, nil
	}
	n := node.(*ast.FencedCodeBlock)
	language := string(n.Language(source))

	hookName := "codeblock-" + language
	hookTemplate, found := r.hooks[hookName]
	if !found {
		hookName = "codeblock"
		hookTemplate, found = r.hooks[hookName]
	}
	if !found {
		return ast.WalkContinue, nil
	}

	var codeBuf bytes.Buffer
	lines := n.Lines()
	for i := 0; i < lines.Len(); i++ {
		line := lines.At(i)
		codeBuf.Write(line.Value(source))
	}

	ctx := map[string]interface{}{
		"markup": map[string]interface{}{
			"language": language,
			"inner":    escapeLiquidDelimiters(codeBuf.String()),
		},
	}
	rendered, err := r.renderHookTemplate(hookTemplate, ctx)
	if err != nil {
		return ast.WalkStop, err
	}
	_, _ = w.WriteString(rendered)
	return ast.WalkSkipChildren, nil
}

func (r *hookNodeRenderer) renderLink(
	w util.BufWriter, source []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	if !entering {
		return ast.WalkContinue, nil
	}
	n := node.(*ast.Link)
	destination := string(n.Destination)

	var textBuf bytes.Buffer
	extractText(&textBuf, n, source)

	isExternal := strings.HasPrefix(destination, "http://") || strings.HasPrefix(destination, "https://")

	ctx := map[string]interface{}{
		"markup": map[string]interface{}{
			"destination": destination,
			"text":        textBuf.String(),
			"title":       string(n.Title),
			"is_external": isExternal,
		},
	}
	rendered, err := r.renderHookTemplate(r.hooks["link"], ctx)
	if err != nil {
		return ast.WalkStop, err
	}
	_, _ = w.WriteString(rendered)
	return ast.WalkSkipChildren, nil
}

func (r *hookNodeRenderer) renderHeading(
	w util.BufWriter, source []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	if !entering {
		return ast.WalkContinue, nil
	}
	n := node.(*ast.Heading)

	var textBuf bytes.Buffer
	extractText(&textBuf, n, source)
	text := textBuf.String()

	inner, err := renderChildrenToHTML(r.childRenderer, source, n)
	if err != nil {
		return ast.WalkStop, err
	}

	id := slugifyHeading(text)
	attrs := make(map[string]interface{})
	for _, attr := range n.Attributes() {
		name := string(attr.Name)
		switch v := attr.Value.(type) {
		case []byte:
			attrs[name] = string(v)
		default:
			attrs[name] = v
		}
		if name == "id" {
			id = attrs[name].(string)
		}
	}

	ctx := map[string]interface{}{
		"markup": map[string]interface{}{
			"level":      n.Level,
			"id":         id,
			"inner":      inner,
			"text":       text,
			"attributes": attrs,
		},
	}
	rendered, err := r.renderHookTemplate(r.hooks["heading"], ctx)
	if err != nil {
		return ast.WalkStop, err
	}
	_, _ = w.WriteString(rendered)
	return ast.WalkSkipChildren, nil
}

func (r *hookNodeRenderer) renderImage(
	w util.BufWriter, source []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	if !entering {
		return ast.WalkContinue, nil
	}
	n := node.(*ast.Image)

	var altBuf bytes.Buffer
	extractText(&altBuf, n, source)

	ctx := map[string]interface{}{
		"markup": map[string]interface{}{
			"src":   string(n.Destination),
			"alt":   altBuf.String(),
			"title": string(n.Title),
		},
	}
	rendered, err := r.renderHookTemplate(r.hooks["image"], ctx)
	if err != nil {
		return ast.WalkStop, err
	}
	_, _ = w.WriteString(rendered)
	return ast.WalkSkipChildren, nil
}

func (r *hookNodeRenderer) renderBlockquote(
	w util.BufWriter, source []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	if !entering {
		return ast.WalkContinue, nil
	}

	inner, err := renderChildrenToHTML(r.childRenderer, source, node)
	if err != nil {
		return ast.WalkStop, err
	}

	ctx := map[string]interface{}{
		"markup": map[string]interface{}{
			"inner": inner,
		},
	}
	rendered, err := r.renderHookTemplate(r.hooks["blockquote"], ctx)
	if err != nil {
		return ast.WalkStop, err
	}
	_, _ = w.WriteString(rendered)
	return ast.WalkSkipChildren, nil
}

func (r *hookNodeRenderer) renderTable(
	w util.BufWriter, source []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	if !entering {
		return ast.WalkContinue, nil
	}

	var buf bytes.Buffer
	if err := r.childRenderer.Render(&buf, source, node); err != nil {
		return ast.WalkStop, err
	}

	ctx := map[string]interface{}{
		"markup": map[string]interface{}{
			"inner": buf.String(),
		},
	}
	rendered, err := r.renderHookTemplate(r.hooks["table"], ctx)
	if err != nil {
		return ast.WalkStop, err
	}
	_, _ = w.WriteString(rendered)
	return ast.WalkSkipChildren, nil
}

func renderChildrenToHTML(r renderer.Renderer, source []byte, parent ast.Node) (string, error) {
	var buf bytes.Buffer
	for child := parent.FirstChild(); child != nil; child = child.NextSibling() {
		if err := r.Render(&buf, source, child); err != nil {
			return "", err
		}
	}
	return buf.String(), nil
}

var nonAlphanumRe = regexp.MustCompile(`[^a-z0-9]+`)

func slugifyHeading(text string) string {
	s := strings.ToLower(text)
	s = strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == ' ' || r == '-' {
			return r
		}
		return -1
	}, s)
	s = nonAlphanumRe.ReplaceAllString(s, "-")
	s = strings.Trim(s, "-")
	return s
}

// escapeLiquidDelimiters entity-encodes Liquid delimiters so the hook
// template engine does not interpret code content as Liquid syntax.
func escapeLiquidDelimiters(s string) string {
	s = strings.ReplaceAll(s, "{{", "&#123;&#123;")
	s = strings.ReplaceAll(s, "}}", "&#125;&#125;")
	s = strings.ReplaceAll(s, "{%", "&#123;%")
	s = strings.ReplaceAll(s, "%}", "%&#125;")
	return s
}

func (r *hookNodeRenderer) renderHookTemplate(tmplSource string, ctx map[string]interface{}) (string, error) {
	if r.renderHook == nil {
		return "", fmt.Errorf("render hook template requires a HookRenderer callback")
	}
	return r.renderHook(tmplSource, ctx)
}
