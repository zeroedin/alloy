package data

import (
	"fmt"
	"sort"

	"github.com/BurntSushi/toml"
	"github.com/zeroedin/alloy/internal/ordered"
	"gopkg.in/yaml.v3"
)

// Data files preserve the key order their author wrote (issue #1262).
//
// Both decoders here replace a plain unmarshal that produced
// map[string]interface{}, which has no order, so renaming nav.json to
// nav.yaml silently reordered a nav menu. Liquid and plugins see the
// difference.
//
// Only LoadFileAny uses these. LoadFile keeps flattening through ToGoMap.

// ── YAML ──────────────────────────────────────────────────────────────

// yamlWalker carries the per-document state a yaml.Node walk needs.
//
// Unmarshalling into a yaml.Node short-circuits yaml.v3's decoder before
// it builds any value: it sees the destination is a Node, copies it, and
// returns, so none of the safeguards in its value path ever run. Measured
// on the same bytes — a duplicate key and a self-referential alias both
// error into interface{} and both return nil into a yaml.Node. Every one
// of those safeguards is therefore ours to reapply. These counters mirror
// gopkg.in/yaml.v3's own: an active-alias set to reject a self-referential
// anchor, and an expansion budget so a compact document cannot expand
// without bound through repeated aliases.
type yamlWalker struct {
	// active holds the alias nodes currently being expanded. A node
	// reached while already expanding it is a cycle.
	active map[*yaml.Node]bool
	// decodeCount counts every node visited; aliasCount counts those
	// visited underneath an alias expansion. aliasDepth tracks whether
	// we are inside one.
	decodeCount int
	aliasCount  int
	aliasDepth  int
}

// Thresholds and the ratio curve are taken from yaml.v3's decoder, so a
// document this walk accepts is one the plain decode accepted too.
const (
	yamlAliasRatioRangeLow  = 400000
	yamlAliasRatioRangeHigh = 4000000
)

func yamlAllowedAliasRatio(decodeCount int) float64 {
	switch {
	case decodeCount <= yamlAliasRatioRangeLow:
		return 0.99
	case decodeCount >= yamlAliasRatioRangeHigh:
		return 0.10
	default:
		span := float64(yamlAliasRatioRangeHigh - yamlAliasRatioRangeLow)
		return 0.99 - 0.89*(float64(decodeCount-yamlAliasRatioRangeLow)/span)
	}
}

// budget reports an error once alias expansion accounts for more of the
// work than a document of this size should need.
func (w *yamlWalker) budget() error {
	w.decodeCount++
	if w.aliasDepth > 0 {
		w.aliasCount++
	}
	if w.aliasCount > 100 && w.decodeCount > 1000 &&
		float64(w.aliasCount)/float64(w.decodeCount) > yamlAllowedAliasRatio(w.decodeCount) {
		return fmt.Errorf("yaml: document contains excessive aliasing")
	}
	return nil
}

// decodeYAMLOrdered parses YAML preserving mapping key order.
//
// yaml.Unmarshal into an interface{} builds Go maps, so order is gone
// before we can read it. Unmarshalling into a yaml.Node keeps the
// document's own key/value sequence, but it returns before the decoder's
// value path runs (see yamlWalker), so duplicate-key rejection, merge-key
// expansion and the alias guards are all reapplied below.
func decodeYAMLOrdered(b []byte) (interface{}, error) {
	var doc yaml.Node
	if err := yaml.Unmarshal(b, &doc); err != nil {
		return nil, err
	}
	// An empty file yields a node with no content. Return what the plain
	// decode returned for it rather than indexing into nothing.
	if doc.Kind == 0 || len(doc.Content) == 0 {
		return nil, nil
	}
	w := &yamlWalker{active: map[*yaml.Node]bool{}}
	return w.value(doc.Content[0])
}

func (w *yamlWalker) value(n *yaml.Node) (interface{}, error) {
	if err := w.budget(); err != nil {
		return nil, err
	}
	switch n.Kind {
	case yaml.DocumentNode:
		if len(n.Content) == 0 {
			return nil, nil
		}
		return w.value(n.Content[0])

	case yaml.AliasNode:
		return w.alias(n, func(target *yaml.Node) (interface{}, error) {
			return w.value(target)
		})

	case yaml.MappingNode:
		return w.mapping(n)

	case yaml.SequenceNode:
		out := make([]interface{}, len(n.Content))
		for i, item := range n.Content {
			v, err := w.value(item)
			if err != nil {
				return nil, err
			}
			out[i] = v
		}
		return out, nil

	default:
		// Scalars keep Decode. It produces exactly what yaml.Unmarshal
		// produces — time.Time for dates, int, float64, bool, nil —
		// whereas reading n.Value as a string would demote dates and
		// numbers and silently break sort and the date filters.
		var v interface{}
		if err := n.Decode(&v); err != nil {
			return nil, err
		}
		return v, nil
	}
}

// alias resolves an alias node and runs fn against its target, refusing a
// node already being expanded. Without this the walk recurses forever on
// "a: &a [*a]" and the process dies with a stack overflow rather than a
// build error naming the file.
func (w *yamlWalker) alias(n *yaml.Node, fn func(*yaml.Node) (interface{}, error)) (interface{}, error) {
	if n.Alias == nil {
		return nil, fmt.Errorf("yaml: line %d: unresolved alias %q", n.Line, n.Value)
	}
	if w.active[n] {
		// Same wording as the plain decoder, so the author sees the
		// message they saw before.
		return nil, fmt.Errorf("yaml: anchor %q value contains itself", n.Value)
	}
	w.active[n] = true
	w.aliasDepth++
	v, err := fn(n.Alias)
	w.aliasDepth--
	delete(w.active, n)
	return v, err
}

// mapping builds an ordered map from a mapping node's flat key/value
// content, reapplying the semantics the node walk bypasses.
func (w *yamlWalker) mapping(n *yaml.Node) (*ordered.Map, error) {
	// Two passes. The first rejects duplicates and records which keys the
	// author wrote here, so a merge key can never overwrite one wherever
	// the two appear relative to each other.
	//
	// seen covers every key including "<<", because yaml.Unmarshal rejects
	// a repeated merge key too; explicit covers only non-merge keys,
	// because those are what win over merged values.
	seen := make(map[string]int, len(n.Content)/2)
	explicit := make(map[string]bool, len(n.Content)/2)
	for i := 0; i+1 < len(n.Content); i += 2 {
		k := n.Content[i]
		name, err := w.keyName(k)
		if err != nil {
			return nil, err
		}
		// A repeated key is a parse error in the plain decoder. A Content
		// loop would accept the file and let the last value win, turning
		// a fatal malformed file into a silent one. Name both lines — the
		// second alone does not tell the author where to look.
		if first, dup := seen[name]; dup {
			return nil, fmt.Errorf("yaml: line %d: mapping key %q already defined at line %d", k.Line, name, first)
		}
		seen[name] = k.Line
		if !isYAMLMergeKey(k) {
			explicit[name] = true
		}
	}

	om := ordered.New()
	for i := 0; i+1 < len(n.Content); i += 2 {
		keyNode, valNode := n.Content[i], n.Content[i+1]

		if isYAMLMergeKey(keyNode) {
			// "<<: *base" expands into this mapping at the position the
			// merge key appeared. A plain Content loop would leave a
			// literal "<<" key and drop the merged-in keys entirely.
			merged, err := w.mergeSources(valNode)
			if err != nil {
				return nil, err
			}
			for _, src := range merged {
				for _, kv := range src.Entries() {
					// Local keys win, and among several merge sources the
					// earlier one wins — both are plain "already have it".
					if explicit[kv.Key] || om.Has(kv.Key) {
						continue
					}
					om.Set(kv.Key, kv.Value)
				}
			}
			continue
		}

		name, err := w.keyName(keyNode)
		if err != nil {
			return nil, err
		}
		v, err := w.value(valNode)
		if err != nil {
			return nil, err
		}
		om.Set(name, v)
	}
	return om, nil
}

// mergeSources resolves a merge key's value to the mappings it pulls in.
// It is either one mapping (usually via an alias) or a sequence of them,
// earliest first. Alias resolution goes through w.alias so a merge key
// pointing at its own mapping is rejected rather than recursed into.
func (w *yamlWalker) mergeSources(n *yaml.Node) ([]*ordered.Map, error) {
	if err := w.budget(); err != nil {
		return nil, err
	}
	if n.Kind == yaml.AliasNode {
		v, err := w.alias(n, func(target *yaml.Node) (interface{}, error) {
			srcs, err := w.mergeSources(target)
			return srcs, err
		})
		if err != nil {
			return nil, err
		}
		srcs, _ := v.([]*ordered.Map)
		return srcs, nil
	}
	switch n.Kind {
	case yaml.MappingNode:
		m, err := w.mapping(n)
		if err != nil {
			return nil, err
		}
		return []*ordered.Map{m}, nil
	case yaml.SequenceNode:
		out := make([]*ordered.Map, 0, len(n.Content))
		for _, item := range n.Content {
			sub, err := w.mergeSources(item)
			if err != nil {
				return nil, err
			}
			out = append(out, sub...)
		}
		return out, nil
	default:
		return nil, fmt.Errorf("yaml: line %d: merge key value must be a mapping or a sequence of mappings", n.Line)
	}
}

func isYAMLMergeKey(n *yaml.Node) bool {
	return n.Kind == yaml.ScalarNode && n.Tag == "!!merge" && n.Value == "<<"
}

// keyName renders a mapping key as the string an ordered map needs.
// Decoding rather than reading n.Value keeps a quoted "1" distinct from a
// bare 1, matching how the plain decode stringifies non-string keys.
func (w *yamlWalker) keyName(n *yaml.Node) (string, error) {
	if n.Kind == yaml.AliasNode {
		if n.Alias == nil {
			return "", fmt.Errorf("yaml: line %d: unresolved alias %q", n.Line, n.Value)
		}
		n = n.Alias
	}
	if n.Kind != yaml.ScalarNode {
		return "", fmt.Errorf("yaml: line %d: mapping key must be a scalar", n.Line)
	}
	var v interface{}
	if err := n.Decode(&v); err != nil {
		return "", err
	}
	if s, ok := v.(string); ok {
		return s, nil
	}
	return fmt.Sprintf("%v", v), nil
}

// ── TOML ──────────────────────────────────────────────────────────────

// tomlOrder is the key order of one decoded value, read from the document
// rather than from the decoded map (which has none).
type tomlOrder struct {
	keys     []string
	children map[string][]*tomlOrder
	// inline marks the order of an inline array. Keys() lists the keys of
	// every table in it one after another, with nothing between tables, so
	// they are kept here as paths relative to the array, in document order,
	// and matched against the decoded array in applyTOMLInline (issue #1414).
	inline     bool
	inlineKeys [][]string
}

func newTOMLOrder() *tomlOrder {
	return &tomlOrder{children: map[string][]*tomlOrder{}}
}

// decodeTOMLOrdered parses TOML preserving key order.
//
// toml.Decode gives a plain map, but MetaData.Keys() reports every key in
// document order as a dotted path, so the order is reassembled level by
// level against those paths.
func decodeTOMLOrdered(b []byte) (interface{}, error) {
	var decoded map[string]interface{}
	md, err := toml.Decode(string(b), &decoded)
	if err != nil {
		return nil, err
	}
	return applyTOMLOrder(decoded, buildTOMLOrder(md)), nil
}

// buildTOMLOrder turns the flat key stream into a tree.
//
// Array-of-tables elements are indistinguishable in the stream — "items",
// "items.name", "items", "items.name", with no index — so a repeat of an
// ArrayHash path is what starts a new element, and subsequent child paths
// belong to the most recent one.
//
// An inline array ("items = [{z = 1, a = 2}, {a = 3, z = 4}]") is one Array
// entry followed by every key of every table in it, depth first, with no
// marker between tables: "items", "items.z", "items.a", "items.a", "items.z".
// A repeated key is no boundary, since a table can start with a key the one
// before it doesn't have. So those keys go to the array's order as relative
// paths, and the decoded array supplies the boundaries (issue #1414).
func buildTOMLOrder(md toml.MetaData) *tomlOrder {
	root := newTOMLOrder()
	var inlineAt []string
	var inline *tomlOrder
	for _, key := range md.Keys() {
		path := []string(key)
		if len(path) == 0 {
			continue
		}
		if inline != nil {
			if len(path) > len(inlineAt) && hasPathPrefix(path, inlineAt) {
				rel := append([]string(nil), path[len(inlineAt):]...)
				inline.inlineKeys = append(inline.inlineKeys, rel)
				continue
			}
			// The array's keys are done. A later key under the same path
			// belongs to something else, such as a [[table]] element that
			// writes "x.child" after an earlier one wrote "x = []".
			inline, inlineAt = nil, nil
		}
		cur := root
		// Walk to the parent level, following the current element of any
		// array on the way.
		for _, part := range path[:len(path)-1] {
			// Keys() reports a dotted key as its full path —
			// "zebra.value = 1" yields []string{"zebra", "value"} — but
			// emits no separate entry for the implicit parent. "zebra" is
			// therefore never a terminal key, and without recording it
			// here it would miss its position entirely and fall into the
			// sorted remainder.
			if !contains(cur.keys, part) {
				cur.keys = append(cur.keys, part)
			}
			kids := cur.children[part]
			if len(kids) == 0 {
				kids = append(kids, newTOMLOrder())
				cur.children[part] = kids
			}
			cur = kids[len(kids)-1]
		}
		name := path[len(path)-1]
		if !contains(cur.keys, name) {
			cur.keys = append(cur.keys, name)
		}
		switch md.Type(path...) {
		case "ArrayHash":
			// A new element of this array starts here.
			cur.children[name] = append(cur.children[name], newTOMLOrder())
		case "Array":
			// An inline array: the keys that follow under this path are
			// its tables' keys. A [[table]] element that repeats the
			// array gets a fresh order of its own.
			//
			// md.Type keeps one type per path, the last one written, so
			// [[x]] headers read as "Array" too when a later element
			// writes x = [...]. Each header is then one table: keep a
			// holder per header, and applyTOMLChild pairs them with the
			// decoded tables (issue #1428).
			inline = newTOMLOrder()
			inline.inline = true
			inlineAt = path
			if kids := cur.children[name]; len(kids) > 0 && kids[len(kids)-1].inline {
				cur.children[name] = append(kids, inline)
			} else {
				cur.children[name] = []*tomlOrder{inline}
			}
		default:
			if len(cur.children[name]) == 0 {
				cur.children[name] = []*tomlOrder{newTOMLOrder()}
			}
		}
	}
	return root
}

// applyTOMLOrder rebuilds the decoded tree with ordered maps, and
// normalizes array-of-tables to []interface{}.
//
// toml.Decode returns array-of-tables as []map[string]interface{}.
// Normalizing to []interface{} gives them the same shape as YAML and JSON
// arrays. (It was added for the Go template engine's converter, issue
// #1237; Go templates are gone, issue #1285, and the shape is kept.)
func applyTOMLOrder(v interface{}, ord *tomlOrder) interface{} {
	switch val := v.(type) {
	case map[string]interface{}:
		om := ordered.New()
		placed := make(map[string]bool, len(val))
		if ord != nil {
			for _, k := range ord.keys {
				cv, ok := val[k]
				if !ok {
					continue
				}
				placed[k] = true
				om.Set(k, applyTOMLChild(cv, ord.children[k]))
			}
		}
		// Anything the key stream did not cover still has to appear.
		// Sorted, so the result is at least deterministic.
		rest := make([]string, 0, len(val)-len(placed))
		for k := range val {
			if !placed[k] {
				rest = append(rest, k)
			}
		}
		sort.Strings(rest)
		for _, k := range rest {
			om.Set(k, applyTOMLOrder(val[k], nil))
		}
		return om

	case []map[string]interface{}:
		return applyTOMLChild(val, nil)

	case []interface{}:
		return applyTOMLChild(val, nil)

	default:
		return v
	}
}

// applyTOMLChild walks a child value. An inline array's order goes to
// tomlInlineCursor; an array-of-tables gets one order per element; anything
// else shares a single order.
func applyTOMLChild(v interface{}, kids []*tomlOrder) interface{} {
	if len(kids) > 0 && kids[0].inline {
		// [[x]] headers that read as "Array": one holder per header,
		// one decoded table each (issue #1428).
		if tables, ok := v.([]map[string]interface{}); ok && len(tables) == len(kids) {
			out := make([]interface{}, len(tables))
			for i, t := range tables {
				c := &tomlInlineCursor{paths: kids[i].inlineKeys}
				out[i] = tomlInlineFinish(c.value(t, nil))
			}
			return out
		}
		c := &tomlInlineCursor{paths: kids[len(kids)-1].inlineKeys}
		return tomlInlineFinish(c.value(v, nil))
	}
	elems, ok := tomlElements(v)
	if !ok {
		var ord *tomlOrder
		if len(kids) > 0 {
			ord = kids[0]
		}
		return applyTOMLOrder(v, ord)
	}
	out := make([]interface{}, len(elems))
	for i, e := range elems {
		var ord *tomlOrder
		switch {
		case len(kids) == len(elems):
			ord = kids[i]
		case len(kids) > 0:
			ord = kids[0]
		}
		out[i] = applyTOMLOrder(e, ord)
	}
	return out
}

// tomlInlineCursor matches an inline array's key paths, in document order,
// against its decoded value (issue #1414).
type tomlInlineCursor struct {
	paths [][]string
	i     int
}

// tomlInlineTable is a table being filled from an inline array's key paths.
// It stays open until its array element is done, so a dotted key written
// after another key ("p.z = 1, q = 2, p.y = 3") still adds to p.
type tomlInlineTable struct {
	val    map[string]interface{}
	prefix []string
	keys   []string
	kids   map[string]interface{}
}

// wants reports whether rel, a key path relative to t, belongs to t: its
// first key is one t has and hasn't placed yet, or a table t has placed
// that wants the rest of the path. TOML forbids a key twice in one table,
// so a path t doesn't want starts the next table in the array.
func (t *tomlInlineTable) wants(rel []string) bool {
	if len(rel) == 0 {
		return false
	}
	if _, ok := t.val[rel[0]]; !ok {
		return false
	}
	kid, placed := t.kids[rel[0]]
	if !placed {
		return true
	}
	sub, ok := kid.(*tomlInlineTable)
	return ok && len(rel) > 1 && sub.wants(rel[1:])
}

// value orders v, whose keys are listed under prefix. An array's elements
// share the array's prefix, since Keys() gives them no index.
func (c *tomlInlineCursor) value(v interface{}, prefix []string) interface{} {
	if m, ok := v.(map[string]interface{}); ok {
		t := &tomlInlineTable{val: m, prefix: prefix, kids: map[string]interface{}{}}
		c.fill(t)
		return t
	}
	if elems, ok := tomlElements(v); ok {
		out := make([]interface{}, len(elems))
		for i, e := range elems {
			out[i] = c.value(e, prefix)
		}
		return out
	}
	return v
}

// fill takes the paths t wants, in order. A dotted key ("p.y = 1") has no
// entry of its own for "p", so the cursor steps only past an exact
// prefix+key entry and leaves "p.y" for p's table.
func (c *tomlInlineCursor) fill(t *tomlInlineTable) {
	for c.i < len(c.paths) {
		p := c.paths[c.i]
		if len(p) <= len(t.prefix) || !hasPathPrefix(p, t.prefix) || !t.wants(p[len(t.prefix):]) {
			return
		}
		k := p[len(t.prefix)]
		if kid, placed := t.kids[k]; placed {
			// A dotted key written after another key: it goes on with
			// k's table.
			c.fill(kid.(*tomlInlineTable))
			continue
		}
		t.keys = append(t.keys, k)
		if len(p) == len(t.prefix)+1 {
			c.i++
		}
		t.kids[k] = c.value(t.val[k], append(append([]string(nil), t.prefix...), k))
	}
}

// tomlInlineFinish turns the filled tables into ordered maps. Anything the
// key stream did not cover goes last, sorted, as in applyTOMLOrder.
func tomlInlineFinish(v interface{}) interface{} {
	switch val := v.(type) {
	case *tomlInlineTable:
		om := ordered.New()
		for _, k := range val.keys {
			om.Set(k, tomlInlineFinish(val.kids[k]))
		}
		rest := make([]string, 0, len(val.val)-len(val.keys))
		for k := range val.val {
			if _, placed := val.kids[k]; !placed {
				rest = append(rest, k)
			}
		}
		sort.Strings(rest)
		for _, k := range rest {
			om.Set(k, applyTOMLOrder(val.val[k], nil))
		}
		return om
	case []interface{}:
		out := make([]interface{}, len(val))
		for i, e := range val {
			out[i] = tomlInlineFinish(e)
		}
		return out
	default:
		return v
	}
}

func hasPathPrefix(path, prefix []string) bool {
	if len(path) < len(prefix) {
		return false
	}
	for i, p := range prefix {
		if path[i] != p {
			return false
		}
	}
	return true
}

// tomlElements normalizes either slice shape toml.Decode can produce to a
// plain []interface{}.
func tomlElements(v interface{}) ([]interface{}, bool) {
	switch val := v.(type) {
	case []map[string]interface{}:
		out := make([]interface{}, len(val))
		for i, e := range val {
			out[i] = e
		}
		return out, true
	case []interface{}:
		return val, true
	default:
		return nil, false
	}
}

func contains(s []string, v string) bool {
	for _, item := range s {
		if item == v {
			return true
		}
	}
	return false
}
