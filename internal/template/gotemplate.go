package template

import (
	"bytes"
	"fmt"
	gohtml "html/template"
)

// goEngine adapts Go's html/template to the TemplateEngine interface.
type goEngine struct {
	funcMap gohtml.FuncMap
}

// NewGoEngine creates a new Go html/template engine.
func NewGoEngine() TemplateEngine {
	return &goEngine{
		funcMap: gohtml.FuncMap{},
	}
}

// goTemplate wraps a parsed Go html/template.
type goTemplate struct {
	tpl  *gohtml.Template
	name string
}

func (e *goEngine) Parse(name string, content []byte) (Template, error) {
	tpl := gohtml.New(name).Funcs(e.funcMap)
	parsed, err := tpl.Parse(string(content))
	if err != nil {
		return nil, fmt.Errorf("go template parse error in %s: %s", name, err.Error())
	}
	return &goTemplate{tpl: parsed, name: name}, nil
}

func (e *goEngine) AddFilter(name string, fn FilterFunc) error {
	e.funcMap[name] = fn
	return nil
}

func (e *goEngine) AddTag(name string, fn TagFunc) error {
	return nil
}

func (t *goTemplate) Render(ctx map[string]interface{}) ([]byte, error) {
	data := markHTMLSafe(ctx)
	var buf bytes.Buffer
	if err := t.tpl.Execute(&buf, data); err != nil {
		return nil, fmt.Errorf("go template render error in %s: %s", t.name, err.Error())
	}
	return buf.Bytes(), nil
}

// markHTMLSafe recursively converts string values in known HTML fields
// (like "content") to template.HTML so they render unescaped.
func markHTMLSafe(ctx map[string]interface{}) map[string]interface{} {
	out := make(map[string]interface{}, len(ctx))
	for k, v := range ctx {
		if k == "content" {
			if s, ok := v.(string); ok {
				out[k] = gohtml.HTML(s)
				continue
			}
		}
		out[k] = v
	}
	return out
}
