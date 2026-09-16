package template

import (
	"bytes"
	"fmt"
	gohtml "html/template"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/zeroedin/alloy/internal/ordered"
)

// goEngine adapts Go's html/template to the TemplateEngine interface.
type goEngine struct {
	funcMap        gohtml.FuncMap
	includesDir    string
	absIncludesDir string
	includeCache   sync.Map // map[string]*gohtml.Template
	depth          atomic.Int32
}

// NewGoEngine creates a new Go html/template engine.
func NewGoEngine() TemplateEngine {
	e := &goEngine{
		funcMap: gohtml.FuncMap{
			"oget": func(m interface{}, key string) interface{} {
				if om, ok := m.(*ordered.Map); ok {
					return om.Get(key)
				}
				if gm, ok := m.(map[string]interface{}); ok {
					return gm[key]
				}
				return nil
			},
			"orange": func(m interface{}) []ordered.KVPair {
				if om, ok := m.(*ordered.Map); ok {
					return om.Entries()
				}
				if gm, ok := m.(map[string]interface{}); ok {
					// Sorted, not ranged: Go randomizes map iteration, so
					// ranging here produced a different order on every build.
					// Every ordered map now reaches templates as a plain map
					// (issue #1237), so without this the nondeterminism would
					// apply to all data rather than just YAML and TOML.
					keys := make([]string, 0, len(gm))
					for k := range gm {
						keys = append(keys, k)
					}
					sort.Strings(keys)
					pairs := make([]ordered.KVPair, 0, len(gm))
					for _, k := range keys {
						pairs = append(pairs, ordered.KVPair{Key: k, Value: gm[k]})
					}
					return pairs
				}
				return nil
			},
			"dict": func(pairs ...interface{}) (map[string]interface{}, error) {
				if len(pairs)%2 != 0 {
					return nil, fmt.Errorf("dict requires even number of arguments, got %d", len(pairs))
				}
				m := make(map[string]interface{}, len(pairs)/2)
				for i := 0; i+1 < len(pairs); i += 2 {
					m[fmt.Sprint(pairs[i])] = pairs[i+1]
				}
				return m, nil
			},
		},
	}
	// include is a placeholder until SetIncludesDir wires the real implementation.
	// Go's html/template requires all FuncMap entries at parse time, so we register
	// a stub that the real closure (set in SetIncludesDir) replaces.
	e.funcMap["include"] = func(path string, dot interface{}) (gohtml.HTML, error) {
		return "", fmt.Errorf("include %q: includes directory not configured", path)
	}
	return e
}

// SetIncludesDir sets the layouts directory for resolving partial templates.
// Must be called before Parse — Go's html/template copies the FuncMap at parse
// time, so templates parsed before this call retain the error stub.
func (e *goEngine) SetIncludesDir(dir string) {
	e.includesDir = dir
	abs, err := filepath.Abs(dir)
	if err == nil {
		e.absIncludesDir = abs
	}
	e.funcMap["include"] = func(path string, dot interface{}) (gohtml.HTML, error) {
		return e.renderInclude(path, dot)
	}
}

const maxIncludeDepth = 100

func (e *goEngine) renderInclude(path string, dot interface{}) (gohtml.HTML, error) {
	d := e.depth.Add(1)
	defer e.depth.Add(-1)

	if d > maxIncludeDepth {
		return "", fmt.Errorf("include %q: nesting too deep (max depth %d)", path, maxIncludeDepth)
	}

	parsed, err := e.resolveInclude(path)
	if err != nil {
		return "", err
	}

	data := dot
	if ctxMap, ok := data.(map[string]interface{}); ok {
		data = markHTMLSafe(ctxMap)
	}

	var buf bytes.Buffer
	if err := parsed.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("include %q: %w", path, err)
	}

	return gohtml.HTML(buf.String()), nil
}

func (e *goEngine) resolveInclude(path string) (*gohtml.Template, error) {
	if cached, ok := e.includeCache.Load(path); ok {
		return cached.(*gohtml.Template), nil
	}

	candidates := []string{
		filepath.Join(e.includesDir, path+".html"),
	}

	var content []byte
	for _, candidate := range candidates {
		abs, err := filepath.Abs(candidate)
		if err != nil {
			continue
		}
		rel, relErr := filepath.Rel(e.absIncludesDir, abs)
		if relErr != nil || strings.HasPrefix(rel, "..") {
			return nil, fmt.Errorf("include %q: path traversal outside layouts directory", path)
		}
		content, err = os.ReadFile(candidate)
		if err == nil {
			break
		}
	}
	if content == nil {
		return nil, fmt.Errorf("include %q: no such template", path)
	}

	src := injectIncludeDot(string(content))
	tpl := gohtml.New(path).Funcs(e.funcMap)
	parsed, err := tpl.Parse(src)
	if err != nil {
		return nil, fmt.Errorf("include %q: %w", path, err)
	}

	e.includeCache.Store(path, parsed)
	return parsed, nil
}

// includeNoDotPattern matches {{ include "path" }} calls that have no second
// argument (no explicit dot). The parse-time rewrite injects `. ` so the
// include function always receives the current dot — including inside
// {{ range }} and {{ with }} blocks where dot changes.
var includeNoDotPattern = regexp.MustCompile(`(include\s+"[^"]*?")\s*(-?\}\})`)

func injectIncludeDot(src string) string {
	return includeNoDotPattern.ReplaceAllString(src, "${1} . ${2}")
}

// goTemplate wraps a parsed Go html/template.
type goTemplate struct {
	tpl    *gohtml.Template
	name   string
	engine *goEngine
}

func (e *goEngine) Parse(name string, content []byte) (Template, error) {
	src := injectIncludeDot(string(content))
	tpl := gohtml.New(name).Funcs(e.funcMap)
	parsed, err := tpl.Parse(src)
	if err != nil {
		return nil, fmt.Errorf("go template parse error in %s: %s", name, err.Error())
	}
	return &goTemplate{tpl: parsed, name: name, engine: e}, nil
}

// AddFilter registers a filter function. Must be called before Parse —
// Go's html/template binds functions at parse time, not render time.
// For known built-in filter names (upcase, downcase, etc.), the real
// implementation is used via ApplyFilter, mirroring how Liquid's
// StandardFilters take precedence over user-registered stubs.
func (e *goEngine) AddFilter(name string, fn FilterFunc) error {
	if IsBuiltinFilter(name) {
		filterName := name // capture for closure
		e.funcMap[name] = func(input interface{}, args ...interface{}) interface{} {
			return ApplyFilter(filterName, input, args...)
		}
	} else {
		e.funcMap[name] = fn
	}
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
		case []interface{}:
			out[k] = markHTMLSafeSlice(val)
		case *ordered.Map:
			// Go templates cannot traverse an *ordered.Map: text/template's
			// evalField resolves .foo as method → struct field → map index,
			// with no extension point, so dot notation needs a real map kind
			// (issue #1237). Converting costs insertion order, which a Go map
			// has nowhere to store — iteration becomes sorted instead.
			//
			// ToGoMap already recurses through nested maps and slices, but it
			// does not mark content/summary as HTML, so convert first and let
			// this walk descend into the result.
			out[k] = markHTMLSafe(val.ToGoMap())
		default:
			out[k] = v
		}
	}
	return out
}

// markHTMLSafeSlice walks a slice so ordered maps nested in arrays are
// converted too. JSON arrays of objects are the common case, and a maps-only
// walk would leave those elements untraversable by dot notation.
//
// Only map[string]interface{}, []interface{} and *ordered.Map are traversed:
// those are the complete output alphabet of ordered.UnmarshalJSONValue and
// ordered.RewrapValue, so an *ordered.Map is never reachable through a typed
// container. internal/ordered/output_shapes_test.go guards that. Reflection
// over arbitrary kinds is deliberately not used — this walk deep-copies the
// whole context on every page render, and it would buy nothing reachable.
func markHTMLSafeSlice(in []interface{}) []interface{} {
	out := make([]interface{}, len(in))
	for i, v := range in {
		switch val := v.(type) {
		case map[string]interface{}:
			out[i] = markHTMLSafe(val)
		case []interface{}:
			out[i] = markHTMLSafeSlice(val)
		case *ordered.Map:
			out[i] = markHTMLSafe(val.ToGoMap())
		default:
			out[i] = v
		}
	}
	return out
}
