package content_test

import (
	"errors"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"

	"github.com/zeroedin/alloy/internal/content"
	tmpl "github.com/zeroedin/alloy/internal/template"
)

// nonByteAttrTransformer is a goldmark AST transformer that injects
// non-[]byte attribute values on heading nodes, simulating third-party
// goldmark extensions that store typed attributes (e.g. bool, float64).
type nonByteAttrTransformer struct{}

func (t *nonByteAttrTransformer) Transform(node *ast.Document, reader text.Reader, pc parser.Context) {
	_ = ast.Walk(node, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		if _, ok := n.(*ast.Heading); ok {
			n.SetAttribute([]byte("visible"), true)
			n.SetAttribute([]byte("weight"), 1.5)
		}
		return ast.WalkContinue, nil
	})
}

var _ = Describe("Render hook context edge cases (issue #896)", func() {
	// HookRenderer wraps Liquid template rendering — same setup as
	// the render hooks tests in markdown_test.go.
	hookRenderer := func(templateSrc string, ctx map[string]interface{}) (string, error) {
		engine := tmpl.NewLiquidEngine()
		tpl, err := engine.Parse("hook", []byte(templateSrc))
		if err != nil {
			return "", err
		}
		result, err := tpl.Render(ctx)
		if err != nil {
			return "", err
		}
		return string(result), nil
	}

	// ── Test 1: Empty heading text with explicit {#id} ──────────────

	It("heading hook with empty text uses explicit {#id} for markup.id (issue #896)", func() {
		opts := content.MarkdownOptions{
			Unsafe: true, Typographer: true, TemplateTags: true,
			AutoHeadingID: true,
			Hooks: map[string]string{
				"heading": `<h{{ markup.level }} id="{{ markup.id }}"><span class="text">{{ markup.text }}</span><span class="inner">{{ markup.inner }}</span></h{{ markup.level }}>`,
			},
			HookRenderer: hookRenderer,
		}
		out, _, err := content.RenderMarkdown(
			[]byte("## {#custom-id}"),
			content.CreateGoldmark(opts),
		)
		Expect(err).NotTo(HaveOccurred())
		html := string(out)
		Expect(html).To(ContainSubstring(`id="custom-id"`),
			"markup.id must use the explicit {#custom-id} attribute — "+
				"when extractText returns empty, slugifyHeading produces empty string, "+
				"so the explicit attribute must override it")
		Expect(html).NotTo(ContainSubstring(`id=""`),
			"markup.id must not be the empty string from slugifyHeading — "+
				"the explicit {#custom-id} attribute takes precedence")
		Expect(html).To(MatchRegexp(`<span class="text">\s*</span>`),
			"markup.text must be empty (or whitespace-only) when heading "+
				"contains only an attribute block and no visible text")
		Expect(html).To(MatchRegexp(`<span class="inner">\s*</span>`),
			"markup.inner must be empty (or whitespace-only) when heading "+
				"has no inline children to render as HTML")
	})

	// ── Test 2: Multiple nested inline elements ──────────────────────

	It("heading hook renders links, bold, and code in markup.inner and strips them from markup.text (issue #896)", func() {
		opts := content.MarkdownOptions{
			Unsafe: true, Typographer: true, TemplateTags: true,
			AutoHeadingID: true,
			Hooks: map[string]string{
				"heading": `<h{{ markup.level }}><span class="inner">{{ markup.inner }}</span><span class="text">{{ markup.text }}</span></h{{ markup.level }}>`,
			},
			HookRenderer: hookRenderer,
		}
		// Heading with bold wrapping a link, plus inline code
		out, _, err := content.RenderMarkdown(
			[]byte("## Hello **[world](https://example.com)** and `code`"),
			content.CreateGoldmark(opts),
		)
		Expect(err).NotTo(HaveOccurred())
		html := string(out)

		// markup.inner must contain all rendered inline HTML elements
		Expect(html).To(ContainSubstring("<strong>"),
			"markup.inner must render bold formatting — "+
				"renderChildrenToHTML must preserve <strong> tags")
		Expect(html).To(ContainSubstring(`<a href="https://example.com">`),
			"markup.inner must render links nested inside bold — "+
				"renderChildrenToHTML must handle nested inline elements")
		Expect(html).To(ContainSubstring("<code>code</code>"),
			"markup.inner must render inline code — "+
				"renderChildrenToHTML must preserve <code> elements")

		// markup.text must strip ALL inline formatting to plain text
		Expect(html).To(ContainSubstring(`<span class="text">Hello world and code</span>`),
			"markup.text must be plain text with all inline formatting stripped — "+
				"extractText must recursively collect text from links, bold, "+
				"and code spans without any HTML tags")
	})

	// ── Test 3: HookRenderer error propagation ──────────────────────
	// Note: the issue requests testing renderChildrenToHTML's error path,
	// but that function is unexported and goldmark's renderer does not error
	// on valid AST nodes. Instead, this test verifies that errors from the
	// HookRenderer callback propagate correctly through the heading hook —
	// the same error handling pattern that renderChildrenToHTML errors use.

	It("heading hook propagates HookRenderer errors to RenderMarkdown (issue #896)", func() {
		expectedErr := errors.New("template rendering failed: simulated error")
		failingRenderer := func(templateSrc string, ctx map[string]interface{}) (string, error) {
			return "", expectedErr
		}
		opts := content.MarkdownOptions{
			Unsafe: true, Typographer: true, TemplateTags: true,
			AutoHeadingID: true,
			Hooks: map[string]string{
				"heading": `<h{{ markup.level }}>{{ markup.inner }}</h{{ markup.level }}>`,
			},
			HookRenderer: failingRenderer,
		}
		_, _, err := content.RenderMarkdown(
			[]byte("## This heading triggers the error"),
			content.CreateGoldmark(opts),
		)
		Expect(err).To(HaveOccurred(),
			"RenderMarkdown must propagate errors from the HookRenderer callback — "+
				"the heading hook returns ast.WalkStop on error, which must surface "+
				"as a non-nil error from RenderMarkdown")
		Expect(err.Error()).To(ContainSubstring("simulated error"),
			"the original error message must be preserved in the propagated error — "+
				"not swallowed or replaced with a generic message")
	})

	// ── Test 4: Non-string attribute values ──────────────────────────

	It("heading hook handles non-[]byte attribute values from goldmark extensions (issue #896)", func() {
		opts := content.MarkdownOptions{
			Unsafe: true, Typographer: true, TemplateTags: true,
			AutoHeadingID: true,
			Hooks: map[string]string{
				"heading": fmt.Sprintf(
					`<h{{ markup.level }} data-visible="{{ markup.attributes.visible }}" data-weight="{{ markup.attributes.weight }}">{{ markup.inner }}</h{{ markup.level }}>`),
			},
			HookRenderer: hookRenderer,
		}
		// Use a custom AST transformer to inject non-[]byte attributes.
		// This simulates third-party goldmark extensions that produce typed
		// attribute values (bool, float64) rather than the standard []byte.
		transformer := parser.WithASTTransformers(
			util.Prioritized(&nonByteAttrTransformer{}, 999),
		)
		md := content.CreateGoldmark(opts, transformer)
		out, _, err := content.RenderMarkdown([]byte("## Test Heading"), md)
		Expect(err).NotTo(HaveOccurred(),
			"non-[]byte attribute values must not cause a panic or error — "+
				"the type switch in renderHeading must handle the default case "+
				"by passing non-byte values through to the template context")
		html := string(out)
		Expect(html).To(ContainSubstring(`data-visible="true"`),
			"bool attribute values must be accessible in the template — "+
				"the default case in the type switch passes non-byte values "+
				"through as-is, and Liquid renders bool true as \"true\"")
		Expect(html).To(ContainSubstring(`data-weight="1.5"`),
			"float64 attribute values must be accessible in the template — "+
				"the default case in the type switch passes non-byte values "+
				"through as-is, and Liquid renders 1.5 as \"1.5\"")
	})
})
