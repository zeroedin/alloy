package template

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/Notifuse/liquidgo/liquid"
	"github.com/Notifuse/liquidgo/liquid/tags"
)

// liquidEngine adapts Notifuse/liquidgo to the TemplateEngine interface.
type liquidEngine struct {
	env            *liquid.Environment
	filters        *alloyFilterBridge
	includesDir    string          // layouts directory for resolving {% include %} / {% render %}
	dynamicFilters map[string]bool // novel filter names needing template pre-processing
}

// NewLiquidEngine creates a new Liquid template engine with standard tags and filters.
func NewLiquidEngine() TemplateEngine {
	env := liquid.NewEnvironment()
	tags.RegisterStandardTags(env)

	bridge := &alloyFilterBridge{
		funcs: make(map[string]FilterFunc),
	}
	env.RegisterFilter(bridge)

	return &liquidEngine{env: env, filters: bridge, dynamicFilters: make(map[string]bool)}
}

// SetIncludesDir sets the directory used to resolve {% include %} and {% render %} tags.
func (e *liquidEngine) SetIncludesDir(dir string) {
	e.includesDir = dir
}

// liquidTemplate wraps a parsed liquidgo template.
type liquidTemplate struct {
	tpl         *liquid.Template
	name        string
	includesDir string
}

func (e *liquidEngine) Parse(name string, content []byte) (Template, error) {
	src := string(content)

	// Pre-process: rewrite novel/plugin filter references to use the
	// plugin_filter bridge, which liquidgo can dispatch via PluginFilter().
	for filterName := range e.dynamicFilters {
		src = rewriteFilterToPlugin(src, filterName)
	}

	opts := &liquid.TemplateOptions{
		Environment: e.env,
	}
	tpl, err := liquid.ParseTemplate(src, opts)
	if err != nil {
		return nil, fmt.Errorf("liquid parse error in %s: %s", name, err.Error())
	}
	tpl.SetName(name)
	return &liquidTemplate{tpl: tpl, name: name, includesDir: e.includesDir}, nil
}

// rewriteFilterToPlugin replaces occurrences of a novel filter name in Liquid
// templates with a plugin_filter bridge call. For example:
//
//	{{ x | myFilter }}           → {{ x | plugin_filter: "myFilter" }}
//	{{ x | myFilter: arg1 }}    → {{ x | plugin_filter: "myFilter", arg1 }}
func rewriteFilterToPlugin(src, filterName string) string {
	// Match: | filterName optionally followed by : (with args)
	pattern := regexp.MustCompile(`\|\s*` + regexp.QuoteMeta(filterName) + `\b(\s*:\s*)?`)
	return pattern.ReplaceAllStringFunc(src, func(match string) string {
		if strings.Contains(match, ":") {
			// Has args: append filter name and comma before existing args
			return `| plugin_filter: "` + filterName + `", `
		}
		// No args
		return `| plugin_filter: "` + filterName + `"`
	})
}

// knownLiquidFilters lists filter names that liquidgo can dispatch natively
// via reflection — either through StandardFilters or alloyFilterBridge methods.
// Novel/plugin filters not in this set need template pre-processing to route
// through the PluginFilter bridge.
var knownLiquidFilters = map[string]bool{
	// liquidgo StandardFilters (snake_case names used in templates)
	"size": true, "downcase": true, "upcase": true, "capitalize": true,
	"escape": true, "h": true, "escape_once": true,
	"url_encode": true, "url_decode": true,
	"base64_encode": true, "base64_decode": true,
	"base64_url_safe_encode": true, "base64_url_safe_decode": true,
	"slice": true, "truncate": true, "truncatewords": true,
	"split": true, "strip": true, "lstrip": true, "rstrip": true,
	"strip_html": true, "first": true, "last": true, "join": true,
	"date": true, "strip_newlines": true, "newline_to_br": true,
	"replace": true, "replace_first": true, "replace_last": true,
	"remove": true, "remove_first": true, "remove_last": true,
	"append": true, "prepend": true,
	"abs": true, "plus": true, "minus": true, "times": true,
	"divided_by": true, "modulo": true, "round": true,
	"ceil": true, "floor": true, "at_least": true, "at_most": true,
	"default": true, "reverse": true, "sort": true, "sort_natural": true,
	"uniq": true, "compact": true, "map": true,
	"where": true, "reject": true, "has": true,
	"find": true, "find_index": true, "concat": true, "sum": true,
	// alloyFilterBridge methods
	"slugify": true, "contains": true, "group_by": true,
	"intersect": true, "union": true, "complement": true,
	"absolute_url": true, "markdownify": true,
	"findRE": true, "replaceRE": true, "json": true,
	"fingerprint": true, "safeHTML": true, "url": true,
}

func (e *liquidEngine) AddFilter(name string, fn FilterFunc) error {
	e.filters.funcs[name] = fn
	// Novel filters need template pre-processing since they don't have
	// exported methods on any registered filter struct for liquidgo's
	// reflection-based dispatch.
	if !knownLiquidFilters[name] {
		e.dynamicFilters[name] = true
	}
	return nil
}

func (e *liquidEngine) AddTag(name string, fn TagFunc) error {
	endTag := "end" + name
	e.env.RegisterTag(name, tags.TagConstructor(
		func(tagName, markup string, parseContext liquid.ParseContextInterface) (interface{}, error) {
			return newAlloyTag(tagName, markup, parseContext, fn, endTag), nil
		},
	))
	return nil
}

func (t *liquidTemplate) Render(ctx map[string]interface{}) ([]byte, error) {
	var opts *liquid.RenderOptions
	if t.includesDir != "" {
		opts = &liquid.RenderOptions{
			Registers: map[string]interface{}{
				"file_system": &alloyFileSystem{root: t.includesDir},
			},
		}
	}
	result := t.tpl.Render(ctx, opts)
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
// alloyFileSystem — resolves {% include %} / {% render %} templates from
// the layouts directory. Tries name.liquid, name.html, then the raw name.
// ---------------------------------------------------------------------------

type alloyFileSystem struct {
	root string
}

func (fs *alloyFileSystem) ReadTemplateFile(templatePath string) (string, error) {
	absRoot, err := filepath.Abs(fs.root)
	if err != nil {
		return "", fmt.Errorf("illegal template path %q: %w", templatePath, err)
	}
	candidates := []string{
		filepath.Join(fs.root, templatePath+".liquid"),
		filepath.Join(fs.root, templatePath+".html"),
		filepath.Join(fs.root, templatePath),
	}
	for _, path := range candidates {
		abs, err := filepath.Abs(path)
		if err != nil {
			continue
		}
		rel, relErr := filepath.Rel(absRoot, abs)
		if relErr != nil || strings.HasPrefix(rel, "..") {
			return "", fmt.Errorf("illegal template path %q", templatePath)
		}
		data, err := os.ReadFile(path)
		if err == nil {
			return string(data), nil
		}
	}
	return "", fmt.Errorf("no such template %q", templatePath)
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

// PluginFilter dispatches novel/plugin filters that don't have their own
// exported method on the bridge. Template pre-processing rewrites
// {{ x | myFilter }} → {{ x | plugin_filter: "myFilter" }} so liquidgo
// routes here via reflection. The first arg is the filter name.
func (f *alloyFilterBridge) PluginFilter(input interface{}, args ...interface{}) interface{} {
	if len(args) >= 1 {
		if name, ok := args[0].(string); ok {
			result := f.call(name, input, args[1:]...)
			if result != nil {
				return result
			}
			// Filter returned nil: fall back to input (passthrough).
			return input
		}
	}
	return input
}

// Reverse overrides liquidgo's built-in Reverse (which reverses arrays) when
// a plugin registers a filter named "reverse". If no plugin override exists,
// returns nil to let liquidgo's StandardFilters.Reverse handle it.
func (f *alloyFilterBridge) Reverse(input interface{}, args ...interface{}) interface{} {
	if fn, ok := f.funcs["reverse"]; ok {
		return fn(input, args...)
	}
	return nil
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
// Handles both inline tags ({% youtube "id" %}) and block tags
// ({% callout "warning" %}content{% endcallout %}).
// ---------------------------------------------------------------------------

// alloyTagMarkupPattern extracts quoted arguments from tag markup.
var alloyTagMarkupPattern = regexp.MustCompile(`"([^"]*)"`)

type alloyTag struct {
	*liquid.Tag
	fn       TagFunc
	markup   string
	endTag   string
	bodyText string
}

func newAlloyTag(tagName, markup string, parseContext liquid.ParseContextInterface, fn TagFunc, endTag string) *alloyTag {
	return &alloyTag{
		Tag:    liquid.NewTag(tagName, markup, parseContext),
		fn:     fn,
		markup: markup,
		endTag: endTag,
	}
}

func (t *alloyTag) Parse(tokenizer *liquid.Tokenizer) error {
	// Consume tokens until the end tag to support block shortcodes.
	// For inline tags (no matching end tag), this loop simply finds
	// nothing and bodyText stays empty.
	for {
		token := tokenizer.Shift()
		if token == "" {
			break
		}
		if strings.HasPrefix(token, "{%") && strings.HasSuffix(token, "%}") {
			content := strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(token, "{%"), "%}"))
			// Handle whitespace-control variants like {%- endcallout -%}
			content = strings.TrimLeft(content, "- ")
			content = strings.TrimRight(content, "- ")
			content = strings.TrimSpace(content)
			if content == t.endTag {
				return nil
			}
		}
		t.bodyText += token
	}
	return nil
}

func (t *alloyTag) Render(context liquid.TagContext) string {
	args := parseTagArgs(t.markup)
	result := t.fn(args, t.bodyText)
	if result == "" {
		return fmt.Sprintf(`<alloy-shortcode data-tag="%s"></alloy-shortcode>`, t.TagName())
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
