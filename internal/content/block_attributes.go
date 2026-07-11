package content

import (
	"bytes"
	"fmt"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	extast "github.com/yuin/goldmark/extension/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
)

// ── Custom AST node for parsed attribute blocks ──────────────────────

var KindAttributesBlock = ast.NewNodeKind("AttributesBlock")

type attributesBlock struct {
	ast.BaseBlock
	Attrs parser.Attributes
}

func (n *attributesBlock) Kind() ast.NodeKind { return KindAttributesBlock }
func (n *attributesBlock) Dump(source []byte, level int) {
	ast.DumpHelper(n, source, level, nil, nil)
}

// ── Block parser for standalone {.class #id key=value} lines ─────────
// Runs at block level before inline parsing, so the '{' is consumed
// before any inline parser (e.g. template tags) sees it.
// Follows the approach used by Hugo's goldmark attribute extensions.

type attributeBlockParser struct{}

func (p *attributeBlockParser) Trigger() []byte { return []byte{'{'} }

func (p *attributeBlockParser) Open(parent ast.Node, reader text.Reader, pc parser.Context) (ast.Node, parser.State) {
	line, _ := reader.PeekLine()
	if len(line) == 0 || line[0] != '{' {
		return nil, parser.NoChildren
	}
	attrs, ok := parser.ParseAttributes(reader)
	if !ok {
		return nil, parser.RequireParagraph
	}
	return &attributesBlock{Attrs: attrs}, parser.NoChildren
}

func (p *attributeBlockParser) Continue(node ast.Node, reader text.Reader, pc parser.Context) parser.State {
	return parser.Close
}

func (p *attributeBlockParser) Close(node ast.Node, reader text.Reader, pc parser.Context) {}
func (p *attributeBlockParser) CanInterruptParagraph() bool                                 { return true }
func (p *attributeBlockParser) CanAcceptIndentedLine() bool                                 { return false }

// ── AST transformer ──────────────────────────────────────────────────
// Phase 1: Move attributes from attributesBlock nodes to their preceding
//          sibling (blockquote, table, paragraph, etc.)
// Phase 2: Parse attributes from fenced code block info strings
// Phase 3: Check for attribute rows absorbed by the table parser

type blockAttributeTransformer struct{}

func (t *blockAttributeTransformer) Transform(node *ast.Document, reader text.Reader, pc parser.Context) {
	source := reader.Source()

	var toRemove []ast.Node
	_ = ast.Walk(node, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		switch n.Kind() {
		case KindAttributesBlock:
			ab := n.(*attributesBlock)
			prev := n.PreviousSibling()
			if prev != nil && !n.HasBlankPreviousLines() {
				setBlockAttributes(prev, ab.Attrs)
			}
			toRemove = append(toRemove, n)

		case ast.KindFencedCodeBlock:
			t.transformFencedCodeBlock(n.(*ast.FencedCodeBlock), source)

		case extast.KindTable:
			t.transformTable(n, source)
		}
		return ast.WalkContinue, nil
	})

	for _, n := range toRemove {
		if p := n.Parent(); p != nil {
			p.RemoveChild(p, n)
		}
	}
}

// transformFencedCodeBlock parses attributes from the info string after the
// language identifier: ```go {.highlight #id key=value}
func (t *blockAttributeTransformer) transformFencedCodeBlock(n *ast.FencedCodeBlock, source []byte) {
	if n.Info == nil {
		return
	}
	info := n.Info.Segment.Value(source)
	idx := bytes.IndexByte(info, '{')
	if idx < 0 {
		return
	}
	attrs, ok := parser.ParseAttributes(text.NewReader(info[idx:]))
	if !ok {
		return
	}
	setBlockAttributes(n, attrs)
}

// transformTable checks if the table's last row is actually an attribute
// block that the table parser absorbed (e.g. {.striped} treated as a row).
func (t *blockAttributeTransformer) transformTable(n ast.Node, source []byte) {
	var lastRow ast.Node
	for child := n.FirstChild(); child != nil; child = child.NextSibling() {
		if child.Kind() == extast.KindTableRow {
			lastRow = child
		}
	}
	if lastRow == nil {
		return
	}
	cellText := collectCellText(lastRow, source)
	cellText = bytes.TrimSpace(cellText)
	if !isAttributeBlock(cellText) {
		return
	}
	attrs, ok := parser.ParseAttributes(text.NewReader(cellText))
	if !ok {
		return
	}
	setBlockAttributes(n, attrs)
	n.RemoveChild(n, lastRow)
}

// collectCellText concatenates all text node values from a table row's first
// cell. The '{' may be split into a separate node by inline parsing.
func collectCellText(row ast.Node, source []byte) []byte {
	firstCell := row.FirstChild()
	if firstCell == nil || firstCell.Kind() != extast.KindTableCell {
		return nil
	}
	var buf bytes.Buffer
	for child := firstCell.FirstChild(); child != nil; child = child.NextSibling() {
		if t, ok := child.(*ast.Text); ok {
			buf.Write(t.Segment.Value(source))
		}
	}
	return buf.Bytes()
}

func isAttributeBlock(b []byte) bool {
	b = bytes.TrimSpace(b)
	return len(b) >= 3 && b[0] == '{' && b[len(b)-1] == '}'
}

// setBlockAttributes normalizes parsed attribute values to []byte and sets
// them on the target node. Goldmark's ParseAttributes returns []byte for
// quoted strings and .class/#id shorthands, but bool for unquoted true/false.
// HTML attributes are always strings, so we normalize everything to []byte.
func setBlockAttributes(node ast.Node, attrs parser.Attributes) {
	for _, attr := range attrs {
		switch v := attr.Value.(type) {
		case []byte:
			node.SetAttribute(attr.Name, v)
		default:
			node.SetAttribute(attr.Name, []byte(fmt.Sprint(v)))
		}
	}
}

// ── Extension ─────────────────────────────────────────────────────────

type blockAttributesExtension struct{}

func (e *blockAttributesExtension) Extend(m goldmark.Markdown) {
	m.Parser().AddOptions(
		parser.WithBlockParsers(
			util.Prioritized(&attributeBlockParser{}, 100),
		),
		parser.WithASTTransformers(
			util.Prioritized(&blockAttributeTransformer{}, 100),
		),
	)
}
