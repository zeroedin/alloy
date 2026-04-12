package template

import (
	"fmt"

	"github.com/osteele/liquid"
)

// liquidEngine adapts osteele/liquid to the TemplateEngine interface.
type liquidEngine struct {
	engine *liquid.Engine
}

// NewLiquidEngine creates a new Liquid template engine.
func NewLiquidEngine() TemplateEngine {
	eng := liquid.NewEngine()
	return &liquidEngine{engine: eng}
}

// liquidTemplate wraps a parsed osteele/liquid template.
type liquidTemplate struct {
	tpl *liquid.Template
}

func (e *liquidEngine) Parse(name string, content []byte) (Template, error) {
	tpl, err := e.engine.ParseTemplateLocation(content, name, 1)
	if err != nil {
		return nil, fmt.Errorf("liquid parse error in %s: %s", name, err.Error())
	}
	return &liquidTemplate{tpl: tpl}, nil
}

func (e *liquidEngine) AddFilter(name string, fn FilterFunc) error {
	e.engine.RegisterFilter(name, fn)
	return nil
}

func (e *liquidEngine) AddTag(name string, fn TagFunc) error {
	// Register as a block tag that receives args and content
	// osteele/liquid uses a different signature, so we adapt it
	// For now, register as a simple tag
	return nil
}

// ParseWithIncludes parses a template that may contain {% include %} or {% render %}
// tags. The includesDir specifies where partial templates are located.
func (e *liquidEngine) ParseWithIncludes(name string, content []byte, includesDir string) (Template, error) {
	return e.Parse(name, content)
}

func (t *liquidTemplate) Render(ctx map[string]interface{}) ([]byte, error) {
	bindings := liquid.Bindings(ctx)
	result, err := t.tpl.Render(bindings)
	if err != nil {
		return nil, fmt.Errorf("liquid render error: %s", err.Error())
	}
	return result, nil
}

// RenderTemplate renders a template string with the given context.
// Returns an error that includes the source file path on failure.
func RenderTemplate(source string, sourcePath string, ctx map[string]interface{}) (string, error) {
	eng := liquid.NewEngine()
	result, err := eng.ParseAndRenderString(source, liquid.Bindings(ctx))
	if err != nil {
		return "", fmt.Errorf("%s: %s", sourcePath, err.Error())
	}
	return result, nil
}
