package template

import (
	"fmt"

	"github.com/Notifuse/liquidgo/liquid"
	"github.com/Notifuse/liquidgo/liquid/tags"
)

// liquidEngine adapts Notifuse/liquidgo to the TemplateEngine interface.
type liquidEngine struct {
	env *liquid.Environment
}

// NewLiquidEngine creates a new Liquid template engine with standard tags and filters.
func NewLiquidEngine() TemplateEngine {
	env := liquid.NewEnvironment()
	tags.RegisterStandardTags(env)
	return &liquidEngine{env: env}
}

// liquidTemplate wraps a parsed liquidgo template.
type liquidTemplate struct {
	tpl  *liquid.Template
	name string
}

func (e *liquidEngine) Parse(name string, content []byte) (Template, error) {
	opts := &liquid.TemplateOptions{
		Environment: e.env,
	}
	tpl, err := liquid.ParseTemplate(string(content), opts)
	if err != nil {
		return nil, fmt.Errorf("liquid parse error in %s: %s", name, err.Error())
	}
	tpl.SetName(name)
	return &liquidTemplate{tpl: tpl, name: name}, nil
}

func (e *liquidEngine) AddFilter(name string, fn FilterFunc) error {
	// liquidgo registers filters as struct methods via reflection.
	// Wrapping arbitrary FilterFunc into a struct-based filter is non-trivial;
	// this is a placeholder for the filter bridge layer.
	return nil
}

func (e *liquidEngine) AddTag(name string, fn TagFunc) error {
	return nil
}

func (t *liquidTemplate) Render(ctx map[string]interface{}) ([]byte, error) {
	result := t.tpl.Render(ctx, nil)
	// Check for errors captured during rendering
	if errs := t.tpl.Errors(); len(errs) > 0 {
		// In lax mode, liquidgo captures errors rather than returning them.
		// Only propagate if the output is empty (indicates a real failure).
		if result == "" {
			return nil, fmt.Errorf("liquid render error in %s: %s", t.name, errs[0].Error())
		}
	}
	return []byte(result), nil
}

// RenderTemplate renders a template string with the given context.
// Returns an error that includes the source file path on failure.
func RenderTemplate(source string, sourcePath string, ctx map[string]interface{}) (string, error) {
	env := liquid.NewEnvironment()
	tags.RegisterStandardTags(env)
	opts := &liquid.TemplateOptions{
		Environment:   env,
		StrictFilters: true,
	}
	tpl, err := liquid.ParseTemplate(source, opts)
	if err != nil {
		return "", fmt.Errorf("%s: %s", sourcePath, err.Error())
	}
	tpl.SetName(sourcePath)
	renderOpts := &liquid.RenderOptions{
		StrictFilters: true,
	}
	result := tpl.Render(ctx, renderOpts)
	if errs := tpl.Errors(); len(errs) > 0 {
		return "", fmt.Errorf("%s: %s", sourcePath, errs[0].Error())
	}
	return result, nil
}
