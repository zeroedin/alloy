package template

// liquidEngine adapts Notifuse/liquidgo to the TemplateEngine interface.
type liquidEngine struct{}

// NewLiquidEngine creates a new Liquid template engine.
func NewLiquidEngine() TemplateEngine {
	return &liquidEngine{}
}

func (e *liquidEngine) Parse(name string, content []byte) (Template, error) {
	return nil, ErrNotImplemented
}

func (e *liquidEngine) AddFilter(name string, fn FilterFunc) error {
	return ErrNotImplemented
}

func (e *liquidEngine) AddTag(name string, fn TagFunc) error {
	return ErrNotImplemented
}

// ParseWithIncludes parses a template that may contain {% include %} or {% render %}
// tags. The includesDir specifies where partial templates are located.
func (e *liquidEngine) ParseWithIncludes(name string, content []byte, includesDir string) (Template, error) {
	return nil, ErrNotImplemented
}

// RenderTemplate renders a template string with the given context.
// Returns an error that includes the source file path on failure.
func RenderTemplate(source string, sourcePath string, ctx map[string]interface{}) (string, error) {
	return "", ErrNotImplemented
}
