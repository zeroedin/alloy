package template

// goEngine adapts Go's html/template to the TemplateEngine interface.
type goEngine struct{}

// NewGoEngine creates a new Go html/template engine.
func NewGoEngine() TemplateEngine {
	return &goEngine{}
}

func (e *goEngine) Parse(name string, content []byte) (Template, error) {
	return nil, ErrNotImplemented
}

func (e *goEngine) AddFilter(name string, fn FilterFunc) error {
	return ErrNotImplemented
}

func (e *goEngine) AddTag(name string, fn TagFunc) error {
	return ErrNotImplemented
}
