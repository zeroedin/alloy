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
// nav.yaml silently reordered a nav menu. Liquid and plugins are what see
// the difference; Go templates sort every map shape (issue #1237) and are
// unaffected either way.
//
// Only LoadFileAny uses these. LoadFile keeps flattening through ToGoMap.

// ── YAML ──────────────────────────────────────────────────────────────

// yamlWalker carries the per-document state a yaml.Node walk needs.
//
// Decoding into a yaml.Node bypasses yaml.Unmarshal entirely, so every
// safeguard that decoder applied is ours to reapply. These counters
// mirror gopkg.in/yaml.v3's own: an active-alias set to reject a
// self-referential anchor, and an expansion budget so a compact document
// cannot expand without bound through repeated aliases.
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
// before we can read it. Decoding into a yaml.Node keeps the document's
// own key/value sequence, but it also bypasses everything yaml.Unmarshal
// did for us — duplicate-key rejection, merge-key expansion, and the
// alias guards on yamlWalker are all reapplied below.
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
func buildTOMLOrder(md toml.MetaData) *tomlOrder {
	root := newTOMLOrder()
	for _, key := range md.Keys() {
		path := []string(key)
		if len(path) == 0 {
			continue
		}
		cur := root
		// Walk to the parent level, following the current element of any
		// array on the way.
		for _, part := range path[:len(path)-1] {
			// A dotted key with no table header of its own — "zebra.value
			// = 1" — reports only the leaf path, so the parent is never a
			// terminal key and would otherwise miss its position entirely
			// and fall into the sorted remainder.
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
		if md.Type(path...) == "ArrayHash" {
			// A new element of this array starts here.
			cur.children[name] = append(cur.children[name], newTOMLOrder())
		} else if len(cur.children[name]) == 0 {
			cur.children[name] = []*tomlOrder{newTOMLOrder()}
		}
	}
	return root
}

// applyTOMLOrder rebuilds the decoded tree with ordered maps, and
// normalizes array-of-tables to []interface{}.
//
// toml.Decode returns array-of-tables as []map[string]interface{}, a
// container the Go template engine's converter does not traverse
// (PLAN.md, "Which containers the walk must traverse"). Left typed, the
// ordered maps inside it would never be converted and {{ .name }} would
// fail at render on a case that works today.
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

// applyTOMLChild walks a child value, handing each element of an array
// its own order when the document gave one per element (array-of-tables)
// and sharing a single order otherwise (an inline array of inline tables
// reports its keys once).
func applyTOMLChild(v interface{}, kids []*tomlOrder) interface{} {
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
