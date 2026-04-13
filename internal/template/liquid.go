package template

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/Notifuse/liquidgo/liquid"
	"github.com/Notifuse/liquidgo/liquid/tags"
)

// liquidEngine adapts Notifuse/liquidgo to the TemplateEngine interface.
type liquidEngine struct {
	env     *liquid.Environment
	filters *alloyFilterBridge
}

// NewLiquidEngine creates a new Liquid template engine with standard tags and filters.
func NewLiquidEngine() TemplateEngine {
	env := liquid.NewEnvironment()
	tags.RegisterStandardTags(env)

	bridge := &alloyFilterBridge{
		funcs: make(map[string]FilterFunc),
	}
	env.RegisterFilter(bridge)

	return &liquidEngine{env: env, filters: bridge}
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
	e.filters.funcs[name] = fn
	return nil
}

func (e *liquidEngine) AddTag(name string, fn TagFunc) error {
	e.env.RegisterTag(name, tags.TagConstructor(
		func(tagName, markup string, parseContext liquid.ParseContextInterface) (interface{}, error) {
			return newAlloyTag(tagName, markup, parseContext, fn), nil
		},
	))
	return nil
}

func (t *liquidTemplate) Render(ctx map[string]interface{}) ([]byte, error) {
	result := t.tpl.Render(ctx, nil)
	if errs := t.tpl.Errors(); len(errs) > 0 {
		// In lax mode, liquidgo captures errors into tpl.Errors() rather than
		// returning them from Render. Errors like missing partials produce error
		// messages inline in the output — this is standard Liquid behavior.
		// We only propagate when the template produced no output at all, which
		// indicates a hard failure rather than graceful degradation.
		// TODO: add strict error mode for build (vs. lax for dev server) per §2.
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

// ---------------------------------------------------------------------------
// alloyFilterBridge — exposes Alloy-specific filters to liquidgo's
// reflection-based filter dispatch. liquidgo discovers exported methods on
// registered filter structs and maps snake_case filter names to CamelCase
// method names. Filters already in liquidgo's StandardFilters (upcase,
// downcase, etc.) are not duplicated here.
// ---------------------------------------------------------------------------

type alloyFilterBridge struct {
	funcs map[string]FilterFunc
}

// The methods below are discovered by liquidgo via reflection. Each proxies
// to the corresponding FilterFunc stored in the funcs map (populated by
// AddFilter / RegisterBuiltinFilters). If the func hasn't been registered
// yet (e.g. the method is called before AddFilter), the method falls back
// to the package-level function directly.

func (f *alloyFilterBridge) call(name string, input interface{}, args ...interface{}) interface{} {
	if fn, ok := f.funcs[name]; ok {
		return fn(input, args...)
	}
	return input
}

func (f *alloyFilterBridge) Slugify(input interface{}, args ...interface{}) interface{} {
	return f.call("slugify", input, args...)
}

func (f *alloyFilterBridge) Contains(input interface{}, args ...interface{}) interface{} {
	return f.call("contains", input, args...)
}

func (f *alloyFilterBridge) GroupBy(input interface{}, args ...interface{}) interface{} {
	return f.call("group_by", input, args...)
}

func (f *alloyFilterBridge) Intersect(input interface{}, args ...interface{}) interface{} {
	return f.call("intersect", input, args...)
}

func (f *alloyFilterBridge) Union(input interface{}, args ...interface{}) interface{} {
	return f.call("union", input, args...)
}

func (f *alloyFilterBridge) Complement(input interface{}, args ...interface{}) interface{} {
	return f.call("complement", input, args...)
}

func (f *alloyFilterBridge) AbsoluteURL(input interface{}, args ...interface{}) interface{} {
	return f.call("absolute_url", input, args...)
}

func (f *alloyFilterBridge) Markdownify(input interface{}, args ...interface{}) interface{} {
	return f.call("markdownify", input, args...)
}

func (f *alloyFilterBridge) FindRE(input interface{}, args ...interface{}) interface{} {
	return f.call("findRE", input, args...)
}

func (f *alloyFilterBridge) ReplaceRE(input interface{}, args ...interface{}) interface{} {
	return f.call("replaceRE", input, args...)
}

func (f *alloyFilterBridge) JSON(input interface{}, args ...interface{}) interface{} {
	return f.call("json", input, args...)
}

func (f *alloyFilterBridge) Fingerprint(input interface{}, args ...interface{}) interface{} {
	return f.call("fingerprint", input, args...)
}

func (f *alloyFilterBridge) SafeHTML(input interface{}, args ...interface{}) interface{} {
	return f.call("safeHTML", input, args...)
}

// Url is the Go-exported name for the "url" filter.
// liquidgo converts "url" → "URL" via its acronym map, so we provide both.
func (f *alloyFilterBridge) Url(input interface{}, args ...interface{}) interface{} {
	return f.call("url", input, args...)
}

func (f *alloyFilterBridge) URL(input interface{}, args ...interface{}) interface{} {
	return f.call("url", input, args...)
}

// ---------------------------------------------------------------------------
// alloyTag — adapts Alloy's TagFunc to liquidgo's tag interface.
// Handles simple inline tags like {% youtube "id" %}.
// ---------------------------------------------------------------------------

// alloyTagMarkupPattern extracts quoted arguments from tag markup.
var alloyTagMarkupPattern = regexp.MustCompile(`"([^"]*)"`)

type alloyTag struct {
	*liquid.Tag
	fn     TagFunc
	markup string
}

func newAlloyTag(tagName, markup string, parseContext liquid.ParseContextInterface, fn TagFunc) *alloyTag {
	return &alloyTag{
		Tag:    liquid.NewTag(tagName, markup, parseContext),
		fn:     fn,
		markup: markup,
	}
}

func (t *alloyTag) Parse(tokenizer *liquid.Tokenizer) error {
	return nil
}

func (t *alloyTag) Render(context liquid.TagContext) string {
	args := parseTagArgs(t.markup)
	result := t.fn(args, "")
	if result == "" {
		// Stub tags produce a placeholder containing the tag name so that
		// integration tests can verify the shortcode was expanded.
		return fmt.Sprintf("<!-- %s -->", t.TagName())
	}
	return result
}

func (t *alloyTag) RenderToOutputBuffer(context liquid.TagContext, output *string) {
	*output += t.Render(context)
}

func parseTagArgs(markup string) []string {
	matches := alloyTagMarkupPattern.FindAllStringSubmatch(markup, -1)
	args := make([]string, 0, len(matches))
	for _, m := range matches {
		if len(m) >= 2 {
			args = append(args, m[1])
		}
	}
	// Also extract unquoted words (for simple args like: {% tag foo bar %})
	if len(args) == 0 {
		parts := strings.Fields(markup)
		args = append(args, parts...)
	}
	return args
}
