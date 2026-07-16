package template

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"

	"github.com/Notifuse/liquidgo/liquid"
	"github.com/Notifuse/liquidgo/liquid/tags"
)

// liquidEngine adapts Notifuse/liquidgo to the TemplateEngine interface.
type liquidEngine struct {
	env            *liquid.Environment
	filters        *alloyFilterBridge
	includesDir    string                       // layouts directory for resolving {% include %} / {% render %}
	filterPatterns map[string]*regexp.Regexp    // compiled regexes for dynamic filters, keyed by filter name
	deferredTags   []deferredTagEntry           // tags registered via AddTag, applied per-Parse
}

// deferredTagEntry stores a tag registration for deferred liquidgo registration.
type deferredTagEntry struct {
	name   string
	endTag string
	fn     TagFunc
}

// NewLiquidEngine creates a new Liquid template engine with standard tags and filters.
func NewLiquidEngine() TemplateEngine {
	env := liquid.NewEnvironment()
	tags.RegisterStandardTags(env)

	bridge := &alloyFilterBridge{
		funcs: make(map[string]FilterFunc),
	}
	env.RegisterFilter(bridge)

	return &liquidEngine{
		env:            env,
		filters:        bridge,
		filterPatterns: make(map[string]*regexp.Regexp),
	}
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

	// Register deferred tags — detect inline vs block by scanning the
	// template source for {% endXxx %}. This determines whether Parse
	// should consume body tokens or return immediately.
	for _, dt := range e.deferredTags {
		isBlock := strings.Contains(src, "{% "+dt.endTag) || strings.Contains(src, "{%- "+dt.endTag)
		if isBlock {
			endTag := dt.endTag
			fn := dt.fn
			e.env.RegisterTag(dt.name, tags.TagConstructor(
				func(tagName, markup string, parseContext liquid.ParseContextInterface) (interface{}, error) {
					return &alloyBlockTag{
						Tag:    liquid.NewTag(tagName, markup, parseContext),
						fn:     fn,
						markup: markup,
						endTag: endTag,
					}, nil
				},
			))
		} else {
			fn := dt.fn
			e.env.RegisterTag(dt.name, tags.TagConstructor(
				func(tagName, markup string, parseContext liquid.ParseContextInterface) (interface{}, error) {
					return &alloyInlineTag{
						Tag:    liquid.NewTag(tagName, markup, parseContext),
						fn:     fn,
						markup: markup,
					}, nil
				},
			))
		}
	}

	opts := &liquid.TemplateOptions{
		Environment:   e.env,
		StrictFilters: true,
	}
	tpl, err := liquid.ParseTemplate(src, opts)
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
	return &liquidTemplate{tpl: tpl, name: name, includesDir: e.includesDir, filterPatterns: patternSnapshot}, nil
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
	"json": true, "safeHTML": true, "url": true,
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
	e.deferredTags = append(e.deferredTags, deferredTagEntry{
		name:   name,
		endTag: "end" + name,
		fn:     fn,
	})
	return nil
}

func (t *liquidTemplate) Render(ctx map[string]interface{}) ([]byte, error) {
	opts := &liquid.RenderOptions{
		StrictFilters: true,
	}
	if t.includesDir != "" {
		opts.Registers = map[string]interface{}{
			"file_system": &alloyFileSystem{root: t.includesDir, filterPatterns: t.filterPatterns},
		}
	}
	result := t.tpl.Render(ctx, opts)
	for _, err := range t.tpl.Errors() {
		if _, ok := err.(*liquid.FileSystemError); ok && t.includesDir == "" {
			continue
		}
		return nil, fmt.Errorf("liquid render error in %s: %s", t.name, err.Error())
	}
	return []byte(result), nil
}

var (
	defaultEnv     *liquid.Environment
	defaultEnvOnce sync.Once
)

func getDefaultEnv() *liquid.Environment {
	defaultEnvOnce.Do(func() {
		defaultEnv = liquid.NewEnvironment()
		tags.RegisterStandardTags(defaultEnv)
		bridge := &alloyFilterBridge{
			funcs: make(map[string]FilterFunc, len(builtinFilters)),
		}
		for name, fn := range builtinFilters {
			bridge.funcs[name] = fn
		}
		defaultEnv.RegisterFilter(bridge)
	})
	return defaultEnv
}

// RenderTemplate renders a template string with the given context.
// Returns an error that includes the source file path on failure.
func RenderTemplate(source string, sourcePath string, ctx map[string]interface{}) (string, error) {
	opts := &liquid.TemplateOptions{
		Environment:   getDefaultEnv(),
		StrictFilters: true,
	}
	tpl, err := liquid.ParseTemplate(source, opts)
	if err != nil {
		errMsg := strings.ReplaceAll(err.Error(), "errors.syntax.unknown_tag", "unknown tag")
		return "", fmt.Errorf("%s: %s", sourcePath, errMsg)
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
// the layouts directory. Tries name.liquid, then name.html.
// ---------------------------------------------------------------------------

type alloyFileSystem struct {
	root           string
	filterPatterns map[string]*regexp.Regexp
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
			return src, nil
		}
	}
	return "", liquid.NewFileSystemError(fmt.Sprintf("no such template %q", templatePath))
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

func (f *alloyFilterBridge) SafeHTML(input interface{}, args ...interface{}) interface{} {
	return f.call("safeHTML", input, args...)
}

func (f *alloyFilterBridge) Cachebust(input interface{}, args ...interface{}) interface{} {
	return f.call("cachebust", input, args...)
}

func (f *alloyFilterBridge) GetHash(input interface{}, args ...interface{}) interface{} {
	return f.call("get_hash", input, args...)
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
// alloyInlineTag handles inline shortcodes ({% tag "args" %}).
// Parse returns immediately — no token consumption.
// ---------------------------------------------------------------------------

type alloyInlineTag struct {
	*liquid.Tag
	fn     TagFunc
	markup string
}

func (t *alloyInlineTag) Parse(tokenizer *liquid.Tokenizer) error { return nil }

func (t *alloyInlineTag) Render(context liquid.TagContext) string {
	args := resolveTagArgs(parseTagTokens(t.markup), context)
	return t.fn(args, "")
}

func (t *alloyInlineTag) RenderToOutputBuffer(context liquid.TagContext, output *string) {
	*output += t.Render(context)
}

// alloyBlockTag handles block shortcodes ({% tag %}body{% endtag %}).
// Parse consumes tokens until the matching end tag.
type alloyBlockTag struct {
	*liquid.Tag
	fn       TagFunc
	markup   string
	endTag   string
	bodyText string
}

func (t *alloyBlockTag) Parse(tokenizer *liquid.Tokenizer) error {
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

func (t *alloyBlockTag) RenderToOutputBuffer(context liquid.TagContext, output *string) {
	*output += t.Render(context)
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
