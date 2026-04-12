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

// AddFilter registers a filter function. Must be called before Parse —
// Go's html/template binds functions at parse time, not render time.
func (e *goEngine) AddFilter(name string, fn FilterFunc) error {
	e.funcMap[name] = fn
	return nil
}

func (e *goEngine) AddTag(name string, fn TagFunc) error {
	e.funcMap[name] = func(args ...string) string {
		return fn(args, "")
	}
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
// (content, summary) to template.HTML so they render unescaped.
// Recurses into nested maps so that fields like page.content and
// page.summary are also converted at any depth.
func markHTMLSafe(ctx map[string]interface{}) map[string]interface{} {
	out := make(map[string]interface{}, len(ctx))
	for k, v := range ctx {
		switch val := v.(type) {
		case string:
			if k == "content" || k == "summary" {
				out[k] = gohtml.HTML(val)
			} else {
				out[k] = v
			}
		case map[string]interface{}:
			out[k] = markHTMLSafe(val)
		default:
			out[k] = v
		}
	}
	return out
}
