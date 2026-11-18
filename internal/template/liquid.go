package template

import (
	gocontext "context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"

	"github.com/Notifuse/liquidgo/v5/liquid"
	"github.com/Notifuse/liquidgo/v5/liquid/tags"
)

// liquidEngine adapts Notifuse/liquidgo (v5) to the TemplateEngine interface.
//
// A liquidgo v5 environment is built once from its config and can't change
// (issue #1363), so the engine collects tags and builds the environment when
// the first template is parsed. Filters need no rebuild: the environment's
// filter functions read the bridge's funcs map when they run.
type liquidEngine struct {
	filters        *alloyFilterBridge
	includesDir    string                    // layouts directory for resolving {% include %} / {% render %}
	filterPatterns map[string]*regexp.Regexp // compiled regexes for dynamic filters, keyed by filter name

	// envMu guards the tags and the environment built from them. AddTag
	// and RegisterInlineTag drop the environment, and the next Parse
	// builds it again.
	envMu        sync.Mutex
	deferredTags []deferredTagEntry // shortcodes registered via AddTag
	extraTags    map[string]liquid.TagConstructor
	env          *liquid.Environment
}

// deferredTagEntry stores a tag registration for deferred liquidgo registration.
type deferredTagEntry struct {
	name   string
	endTag string
	fn     TagFunc
}

// NewLiquidEngine creates a new Liquid template engine with standard tags and filters.
func NewLiquidEngine() TemplateEngine {
	return newLiquidEngine()
}

// NewLiquidEngineWithIncludes creates a Liquid engine whose {% include %} and
// {% render %} tags resolve against includesDir. The build uses it with the
// site's layouts directory.
func NewLiquidEngineWithIncludes(includesDir string) TemplateEngine {
	e := newLiquidEngine()
	e.SetIncludesDir(includesDir)
	return e
}

func newLiquidEngine() *liquidEngine {
	return &liquidEngine{
		filters: &alloyFilterBridge{
			funcs: make(map[string]FilterFunc),
		},
		filterPatterns: make(map[string]*regexp.Regexp),
		extraTags:      make(map[string]liquid.TagConstructor),
	}
}

// addTagConstructor adds a tag the engine's environment has, such as
// {% inline %}, and drops the environment built without it.
func (e *liquidEngine) addTagConstructor(name string, ctor liquid.TagConstructor) {
	e.envMu.Lock()
	defer e.envMu.Unlock()
	e.extraTags[name] = ctor
	e.env = nil
}

// environment returns the engine's environment, built on first use, and
// the names of its shortcodes.
//
// Every shortcode is a block tag (issue #1367). Whether a use is inline or
// a block is decided per template, partials included, by
// closeInlineShortcodes, which adds an {% endname %} after each inline use
// before the template is parsed. So one environment serves every template.
func (e *liquidEngine) environment() (*liquid.Environment, map[string]bool, error) {
	e.envMu.Lock()
	defer e.envMu.Unlock()

	names := make(map[string]bool, len(e.deferredTags))
	for _, dt := range e.deferredTags {
		names[dt.name] = true
	}
	if e.env != nil {
		return e.env, names, nil
	}

	tagMap := tags.Standard()
	for name, ctor := range e.extraTags {
		tagMap[name] = ctor
	}
	for _, dt := range e.deferredTags {
		fn, endTag := dt.fn, dt.endTag
		tagMap[dt.name] = func(tagName, markup string, parseContext liquid.ParseContextInterface) (liquid.Node, error) {
			return &alloyBlockTag{
				Tag:    liquid.NewTag(tagName, markup, parseContext),
				fn:     fn,
				markup: markup,
				endTag: endTag,
			}, nil
		}
	}

	env, err := newAlloyEnvironment(tagMap, e.filters)
	if err != nil {
		return nil, nil, err
	}
	e.env = env
	return env, names, nil
}

// newAlloyEnvironment builds a liquidgo environment with Alloy's filters, in
// strict mode: markup that doesn't parse fails the build instead of
// rendering as nothing, as it did before v5 (issue #1363).
func newAlloyEnvironment(tagMap map[string]liquid.TagConstructor, bridge *alloyFilterBridge) (*liquid.Environment, error) {
	return liquid.NewEnvironment(liquid.EnvironmentConfig{
		ErrorMode: liquid.ErrorModeStrict,
		Tags:      tagMap,
		Filters:   bridgeFilterDefs(bridge),
	})
}

// SetIncludesDir sets the directory used to resolve {% include %} and {% render %} tags.
func (e *liquidEngine) SetIncludesDir(dir string) {
	e.includesDir = dir
}

// liquidTemplate wraps a parsed liquidgo template.
type liquidTemplate struct {
	tpl            *liquid.Template
	name           string
	includesDir    string
	filterPatterns map[string]*regexp.Regexp
	shortcodes     map[string]bool // the engine's shortcodes, for partials
}

func (e *liquidEngine) Parse(name string, content []byte) (Template, error) {
	src := string(content)

	// Pre-process: rewrite novel/plugin filter references to use the
	// plugin_filter bridge, which liquidgo can dispatch via PluginFilter().
	for filterName, pattern := range e.filterPatterns {
		if strings.Contains(src, filterName) {
			src = rewriteFilterToPlugin(src, filterName, pattern)
		}
	}

	env, shortcodes, err := e.environment()
	if err != nil {
		return nil, fmt.Errorf("liquid environment for %s: %w", name, err)
	}
	src = closeInlineShortcodes(src, shortcodes)

	tpl, err := liquid.ParseTemplate(src, &liquid.TemplateOptions{Environment: env})
	if err != nil {
		errMsg := err.Error()
		// Replace liquidgo's unresolved i18n key with a readable message
		errMsg = strings.ReplaceAll(errMsg, "errors.syntax.unknown_tag", "unknown tag")
		return nil, fmt.Errorf("liquid parse error in %s: %s", name, errMsg)
	}
	tpl.SetName(name)
	patternSnapshot := make(map[string]*regexp.Regexp, len(e.filterPatterns))
	for k, v := range e.filterPatterns {
		patternSnapshot[k] = v
	}
	return &liquidTemplate{tpl: tpl, name: name, includesDir: e.includesDir,
		filterPatterns: patternSnapshot, shortcodes: shortcodes}, nil
}

func compileFilterPattern(filterName string) *regexp.Regexp {
	return regexp.MustCompile(`\|\s*` + regexp.QuoteMeta(filterName) + `\b(\s*:\s*)?`)
}

// rewriteFilterToPlugin replaces occurrences of a novel filter name in Liquid
// templates with a plugin_filter bridge call. For example:
//
//	{{ x | myFilter }}           → {{ x | plugin_filter: "myFilter" }}
//	{{ x | myFilter: arg1 }}    → {{ x | plugin_filter: "myFilter", arg1 }}
func rewriteFilterToPlugin(src, filterName string, pattern *regexp.Regexp) string {
	return pattern.ReplaceAllStringFunc(src, func(match string) string {
		if strings.Contains(match, ":") {
			return `| plugin_filter: "` + filterName + `", `
		}
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
	// alloyFilterBridge methods (excluding contains, findRE, replaceRE
	// which need the plugin_filter bridge for correct Liquid behavior)
	"slugify": true, "group_by": true, "flatten": true,
	"intersect": true, "union": true, "complement": true,
	"absolute_url": true, "markdownify": true,
	"json": true, "url": true,
	"cachebust": true, "get_hash": true,
}

func (e *liquidEngine) AddFilter(name string, fn FilterFunc) error {
	_, alreadyRegistered := e.filters.funcs[name]
	if alreadyRegistered || !knownLiquidFilters[name] {
		e.filterPatterns[name] = compileFilterPattern(name)
	}
	e.filters.funcs[name] = fn
	return nil
}

func (e *liquidEngine) AddTag(name string, fn TagFunc) error {
	e.envMu.Lock()
	defer e.envMu.Unlock()
	e.deferredTags = append(e.deferredTags, deferredTagEntry{
		name:   name,
		endTag: "end" + name,
		fn:     fn,
	})
	e.env = nil
	return nil
}

func (t *liquidTemplate) Render(ctx map[string]interface{}) ([]byte, error) {
	opts := []liquid.RenderOption{liquid.WithStrictFilters()}
	if t.includesDir != "" {
		opts = append(opts, liquid.WithFileSystem(
			&alloyFileSystem{root: t.includesDir, filterPatterns: t.filterPatterns, shortcodes: t.shortcodes}))
	}
	res, err := t.tpl.Render(gocontext.Background(), ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("liquid render error in %s: %s", t.name, err.Error())
	}
	for _, err := range res.Errors {
		var fsErr *liquid.FileSystemError
		if errors.As(err, &fsErr) && t.includesDir == "" {
			continue
		}
		return nil, fmt.Errorf("liquid render error in %s: %s", t.name, err.Error())
	}
	return []byte(res.Output), nil
}

var (
	defaultEnv     *liquid.Environment
	defaultEnvOnce sync.Once
)

func getDefaultEnv() *liquid.Environment {
	defaultEnvOnce.Do(func() {
		bridge := &alloyFilterBridge{
			funcs: make(map[string]FilterFunc, len(builtinFilters)),
		}
		for name, fn := range builtinFilters {
			bridge.funcs[name] = fn
		}
		env, err := newAlloyEnvironment(tags.Standard(), bridge)
		if err != nil {
			// The config is fixed in code: an error is a bug, not input.
			panic(fmt.Sprintf("alloy: building the default Liquid environment: %v", err))
		}
		defaultEnv = env
	})
	return defaultEnv
}

// RenderTemplate renders a template string with the given context.
// Returns an error that includes the source file path on failure.
func RenderTemplate(source string, sourcePath string, ctx map[string]interface{}) (string, error) {
	tpl, err := liquid.ParseTemplate(source, &liquid.TemplateOptions{Environment: getDefaultEnv()})
	if err != nil {
		errMsg := strings.ReplaceAll(err.Error(), "errors.syntax.unknown_tag", "unknown tag")
		return "", fmt.Errorf("%s: %s", sourcePath, errMsg)
	}
	tpl.SetName(sourcePath)
	res, err := tpl.Render(gocontext.Background(), ctx, liquid.WithStrictFilters())
	if err != nil {
		return "", fmt.Errorf("%s: %s", sourcePath, err.Error())
	}
	if len(res.Errors) > 0 {
		return "", fmt.Errorf("%s: %s", sourcePath, res.Errors[0].Error())
	}
	return res.Output, nil
}

// ---------------------------------------------------------------------------
// alloyFileSystem — resolves {% include %} / {% render %} templates from
// the layouts directory. Tries name.liquid, then name.html.
// ---------------------------------------------------------------------------

type alloyFileSystem struct {
	root           string
	filterPatterns map[string]*regexp.Regexp
	shortcodes     map[string]bool
}

func (fs *alloyFileSystem) ReadTemplateFile(templatePath string) (string, error) {
	absRoot, err := filepath.Abs(fs.root)
	if err != nil {
		return "", liquid.NewFileSystemError(fmt.Sprintf("cannot resolve includes root for %q", templatePath))
	}
	candidates := []string{
		filepath.Join(fs.root, templatePath+".liquid"),
		filepath.Join(fs.root, templatePath+".html"),
	}
	for _, path := range candidates {
		abs, err := filepath.Abs(path)
		if err != nil {
			continue
		}
		rel, relErr := filepath.Rel(absRoot, abs)
		if relErr != nil || strings.HasPrefix(rel, "..") {
			return "", liquid.NewFileSystemError(fmt.Sprintf("illegal template path %q: traversal outside includes directory", templatePath))
		}
		data, err := os.ReadFile(path)
		if err == nil {
			src := string(data)
			for filterName, pattern := range fs.filterPatterns {
				if strings.Contains(src, filterName) {
					src = rewriteFilterToPlugin(src, filterName, pattern)
				}
			}
			// A partial decides for itself whether each shortcode use is
			// inline or a block (issue #1367).
			return closeInlineShortcodes(src, fs.shortcodes), nil
		}
	}
	return "", liquid.NewFileSystemError(fmt.Sprintf("no such template %q", templatePath))
}

// ---------------------------------------------------------------------------
// alloyFilterBridge — exposes Alloy-specific filters to liquidgo. liquidgo v5
// doesn't find filters by method name: bridgeFilterDefs declares each one.
// Declared filters take precedence over liquidgo's standard filters of the
// same name (where, sort, map, reverse...). Filters only in liquidgo's
// standard set (upcase, downcase, etc.) are not duplicated here.
// ---------------------------------------------------------------------------

type alloyFilterBridge struct {
	funcs map[string]FilterFunc
}

// bridgeMethod is the signature of the bridge's filter methods.
type bridgeMethod func(input interface{}, args ...interface{}) interface{}

// bridgeFilterDefs declares the bridge's filters to liquidgo, by the names
// templates write. plugin_filter dispatches every filter a template
// pre-processing step rewrote to it (see rewriteFilterToPlugin).
func bridgeFilterDefs(f *alloyFilterBridge) []liquid.FilterDef {
	methods := []struct {
		name string
		fn   bridgeMethod
	}{
		{"plugin_filter", f.PluginFilter},
		{"reverse", f.Reverse},
		{"slugify", f.Slugify},
		{"contains", f.Contains},
		{"group_by", f.GroupBy},
		{"where", f.Where},
		{"sort", f.Sort},
		{"map", f.Map},
		{"flatten", f.Flatten},
		{"intersect", f.Intersect},
		{"union", f.Union},
		{"complement", f.Complement},
		{"absolute_url", f.AbsoluteURL},
		{"markdownify", f.Markdownify},
		// Parse rewrites findRE and replaceRE to plugin_filter once they
		// are registered, but RenderTemplate parses its source unchanged.
		{"findRE", f.FindRE},
		{"replaceRE", f.ReplaceRE},
		{"json", f.JSON},
		{"cachebust", f.Cachebust},
		{"get_hash", f.GetHash},
		{"url", f.URL},
	}
	defs := make([]liquid.FilterDef, len(methods))
	for i, m := range methods {
		defs[i] = liquid.Filter(m.name, wrapBridgeMethod(m.fn))
	}
	return defs
}

// wrapBridgeMethod adapts a bridge method to liquidgo's filter signature.
// A plugin filter that fails panics with a *PluginCallError (see
// pipeline/state.go), which liquidgo records as a raised Liquid error.
func wrapBridgeMethod(fn bridgeMethod) liquid.FilterFunc {
	return func(_ *liquid.Context, input interface{}, args ...interface{}) (interface{}, error) {
		return fn(input, args...), nil
	}
}

// The methods below are declared to liquidgo by bridgeFilterDefs. Each
// proxies to the corresponding FilterFunc stored in the funcs map
// (populated by AddFilter / RegisterBuiltinFilters). If the func hasn't
// been registered, the input passes through unchanged.

func (f *alloyFilterBridge) call(name string, input interface{}, args ...interface{}) interface{} {
	if fn, ok := f.funcs[name]; ok {
		return fn(input, args...)
	}
	return input
}

// PluginFilter dispatches novel/plugin filters that don't have their own
// declared name on the bridge. Template pre-processing rewrites
// {{ x | myFilter }} → {{ x | plugin_filter: "myFilter" }} so liquidgo
// routes here. The first arg is the filter name.
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

// Reverse handles the "reverse" filter. When a plugin override is registered,
// it delegates to the plugin (allowing string reversal etc.). Otherwise,
// implements the default array/slice reverse behavior matching liquidgo.
func (f *alloyFilterBridge) Reverse(input interface{}, args ...interface{}) interface{} {
	if fn, ok := f.funcs["reverse"]; ok {
		return fn(input, args...)
	}
	// Default: reverse array/slice elements (same as liquidgo's built-in)
	if arr, ok := input.([]interface{}); ok {
		reversed := make([]interface{}, len(arr))
		for i, v := range arr {
			reversed[len(arr)-1-i] = v
		}
		return reversed
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

func (f *alloyFilterBridge) Where(input interface{}, args ...interface{}) interface{} {
	return f.call("where", input, args...)
}

func (f *alloyFilterBridge) Sort(input interface{}, args ...interface{}) interface{} {
	return f.call("sort", input, args...)
}

func (f *alloyFilterBridge) Map(input interface{}, args ...interface{}) interface{} {
	return f.call("map", input, args...)
}

func (f *alloyFilterBridge) Flatten(input interface{}, args ...interface{}) interface{} {
	return f.call("flatten", input, args...)
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

func (f *alloyFilterBridge) Cachebust(input interface{}, args ...interface{}) interface{} {
	return f.call("cachebust", input, args...)
}

func (f *alloyFilterBridge) GetHash(input interface{}, args ...interface{}) interface{} {
	return f.call("get_hash", input, args...)
}

func (f *alloyFilterBridge) URL(input interface{}, args ...interface{}) interface{} {
	return f.call("url", input, args...)
}

// ---------------------------------------------------------------------------
// PluginCallError wraps a plugin runtime error (filter or shortcode call
// failure) for propagation through the template rendering pipeline via panic.
// It implements liquid.LiquidError, so liquidgo treats the panic as a raised
// Liquid error, as liquid.Raise does: the render records it with its own
// message. Without that, liquidgo v5 would end the render with a
// *liquid.PanicError, or record the generic message "internal".
// ---------------------------------------------------------------------------

type PluginCallError struct {
	Kind    string // "filter" or "shortcode"
	Name    string
	Wrapped error
	liqErr  liquid.Error
}

func NewPluginCallError(kind, name string, err error) *PluginCallError {
	msg := fmt.Sprintf("%s %q: %v", kind, name, err)
	return &PluginCallError{
		Kind:    kind,
		Name:    name,
		Wrapped: err,
		liqErr:  liquid.Error{Message: msg},
	}
}

func (e *PluginCallError) Error() string {
	return fmt.Sprintf("%s %q: %v", e.Kind, e.Name, e.Wrapped)
}

func (e *PluginCallError) GetError() *liquid.Error {
	return &e.liqErr
}

// ---------------------------------------------------------------------------
// alloyBlockTag handles shortcodes ({% tag %}body{% endtag %}). Parse
// consumes tokens until the matching end tag. An inline use
// ({% tag "args" %}) is a block with an empty body: closeInlineShortcodes
// adds its end tag before the template is parsed (issue #1367).
// ---------------------------------------------------------------------------
type alloyBlockTag struct {
	*liquid.Tag
	fn       TagFunc
	markup   string
	endTag   string
	bodyText string
}

func (t *alloyBlockTag) Parse(tokenizer *liquid.Tokenizer) error {
	// Inside {% liquid %}, a shortcode is a statement on its own line, with
	// no {% %} for closeInlineShortcodes to close: it stays inline, as
	// before issue #1367.
	if tokenizer.ForLiquidTag() {
		return nil
	}
	var body strings.Builder
	for {
		token := tokenizer.Shift()
		if token == "" {
			return fmt.Errorf("liquid syntax error: '%s' tag was never closed", t.TagName())
		}
		if strings.HasPrefix(token, "{%") && strings.HasSuffix(token, "%}") {
			content := strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(token, "{%"), "%}"))
			content = strings.TrimLeft(content, "- ")
			content = strings.TrimRight(content, "- ")
			content = strings.TrimSpace(content)
			if content == t.endTag {
				t.bodyText = body.String()
				return nil
			}
		}
		body.WriteString(token)
	}
}

func (t *alloyBlockTag) Render(context liquid.TagContext) string {
	args := resolveTagArgs(parseTagTokens(t.markup), context)
	return t.fn(args, t.bodyText)
}

func (t *alloyBlockTag) RenderToOutput(context liquid.TagContext, output *liquid.OutputBuffer) {
	output.WriteString(t.Render(context))
}

// parsedArg represents a single argument token from tag markup.
// Quoted args are literal strings; unquoted args may be variable references.
type parsedArg struct {
	value  string
	quoted bool
}

// parseTagTokens parses tag markup into a sequence of quoted and unquoted
// argument tokens, preserving order. Both single and double quotes are
// recognized as string delimiters (Liquid supports both). Supports mixed
// forms like:
//
//	"primary" page.size   → [{value:"primary", quoted:true}, {value:"page.size", quoted:false}]
//	'label' count         → [{value:"label", quoted:true}, {value:"count", quoted:false}]
func parseTagTokens(markup string) []parsedArg {
	s := strings.TrimSpace(markup)
	var args []parsedArg
	for len(s) > 0 {
		if s[0] == '"' || s[0] == '\'' {
			quote := s[0]
			end := strings.IndexByte(s[1:], quote)
			if end >= 0 {
				args = append(args, parsedArg{value: s[1 : end+1], quoted: true})
				s = strings.TrimSpace(s[end+2:])
			} else {
				args = append(args, parsedArg{value: s[1:], quoted: false})
				break
			}
		} else {
			end := strings.IndexAny(s, " \t\"'")
			if end >= 0 {
				if s[:end] != "" {
					args = append(args, parsedArg{value: s[:end], quoted: false})
				}
				s = strings.TrimSpace(s[end:])
			} else {
				args = append(args, parsedArg{value: s, quoted: false})
				break
			}
		}
	}
	return args
}

// resolveTagArgs converts parsed arg tokens into final string values by
// resolving unquoted args against the Liquid render context. Quoted args
// remain literal. Unquoted args that don't match a context variable fall
// back to their literal token string (backward compatible).
func resolveTagArgs(tokens []parsedArg, context liquid.TagContext) []string {
	args := make([]string, len(tokens))
	for i, tok := range tokens {
		if tok.quoted || context == nil {
			args[i] = tok.value
			continue
		}
		val, found := resolveVariable(tok.value, context)
		if found {
			args[i] = fmt.Sprint(val)
		} else {
			args[i] = tok.value
		}
	}
	return args
}

// resolveVariable looks up a possibly-dotted variable path (e.g. "page.videoId")
// against the Liquid render context using liquidgo's native variable lookup,
// which handles map[string]interface{}, typed maps (via reflection), Drops,
// array index access, and command methods. Returns the resolved value and true
// if found, or (nil, false) if the variable does not exist or is nil.
func resolveVariable(name string, context liquid.TagContext) (interface{}, bool) {
	vl := liquid.VariableLookupParse(name, nil, nil)
	val := context.Evaluate(vl)
	if val == nil {
		return nil, false
	}
	return val, true
}

// parseTagArgs extracts args from tag markup as plain strings without
// variable resolution. Used by tags that don't need context-aware args
// (e.g. the {% inline %} tag).
func parseTagArgs(markup string) []string {
	tokens := parseTagTokens(markup)
	args := make([]string, len(tokens))
	for i, t := range tokens {
		args[i] = t.value
	}
	return args
}
