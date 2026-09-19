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

// decodeYAMLOrdered parses YAML preserving mapping key order.
//
// yaml.Unmarshal into an interface{} builds Go maps, so order is gone
// before we can read it. Decoding into a yaml.Node keeps the document's
// own key/value sequence, but it also bypasses everything yaml.Unmarshal
// did for us — duplicate-key rejection and merge-key expansion are
// reapplied below.
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
	return yamlNodeValue(doc.Content[0])
}

func yamlNodeValue(n *yaml.Node) (interface{}, error) {
	switch n.Kind {
	case yaml.DocumentNode:
		if len(n.Content) == 0 {
			return nil, nil
		}
		return yamlNodeValue(n.Content[0])

	case yaml.AliasNode:
		if n.Alias == nil {
			return nil, fmt.Errorf("yaml: line %d: unresolved alias %q", n.Line, n.Value)
		}
		return yamlNodeValue(n.Alias)

	case yaml.MappingNode:
		return yamlMappingValue(n)

	case yaml.SequenceNode:
		out := make([]interface{}, len(n.Content))
		for i, item := range n.Content {
			v, err := yamlNodeValue(item)
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

// yamlMappingValue builds an ordered map from a mapping node's flat
// key/value content, reapplying the two semantics the node walk bypasses.
func yamlMappingValue(n *yaml.Node) (*ordered.Map, error) {
	// Explicit keys are collected first so a merge key can never overwrite
	// one, wherever the two appear relative to each other. The value is
	// the line the key was first seen on, for the duplicate-key error.
	explicit := make(map[string]int, len(n.Content)/2)
	for i := 0; i+1 < len(n.Content); i += 2 {
		k := n.Content[i]
		if isYAMLMergeKey(k) {
			continue
		}
		name, err := yamlKeyName(k)
		if err != nil {
			return nil, err
		}
		// yaml.Unmarshal rejects a repeated key; a Content loop would
		// accept the file and let the last value win, turning a fatal
		// malformed file into a silent one. Name both lines, as the
		// plain decoder did — the second one alone does not tell the
		// author where to look.
		if first, dup := explicit[name]; dup {
			return nil, fmt.Errorf("yaml: line %d: mapping key %q already defined at line %d", k.Line, name, first)
		}
		explicit[name] = k.Line
	}

	om := ordered.New()
	for i := 0; i+1 < len(n.Content); i += 2 {
		keyNode, valNode := n.Content[i], n.Content[i+1]

		if isYAMLMergeKey(keyNode) {
			// "<<: *base" expands into this mapping at the position the
			// merge key appeared. A plain Content loop would leave a
			// literal "<<" key and drop the merged-in keys entirely.
			merged, err := yamlMergeSources(valNode)
			if err != nil {
				return nil, err
			}
			for _, src := range merged {
				for _, kv := range src.Entries() {
					// Local keys win, and among several merge sources the
					// earlier one wins — both are plain "already have it".
					if _, local := explicit[kv.Key]; local || om.Has(kv.Key) {
						continue
					}
					om.Set(kv.Key, kv.Value)
				}
			}
			continue
		}

		name, err := yamlKeyName(keyNode)
		if err != nil {
			return nil, err
		}
		v, err := yamlNodeValue(valNode)
		if err != nil {
			return nil, err
		}
		om.Set(name, v)
	}
	return om, nil
}

// yamlMergeSources resolves a merge key's value to the mappings it pulls
// in. It is either one mapping (usually via an alias) or a sequence of
// them, earliest first.
func yamlMergeSources(n *yaml.Node) ([]*ordered.Map, error) {
	if n.Kind == yaml.AliasNode {
		if n.Alias == nil {
			return nil, fmt.Errorf("yaml: line %d: unresolved alias %q", n.Line, n.Value)
		}
		n = n.Alias
	}
	switch n.Kind {
	case yaml.MappingNode:
		m, err := yamlMappingValue(n)
		if err != nil {
			return nil, err
		}
		return []*ordered.Map{m}, nil
	case yaml.SequenceNode:
		out := make([]*ordered.Map, 0, len(n.Content))
		for _, item := range n.Content {
			sub, err := yamlMergeSources(item)
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

// yamlKeyName renders a mapping key as the string an ordered map needs.
// Decoding rather than reading n.Value keeps a quoted "1" distinct from a
// bare 1, matching how the plain decode stringifies non-string keys.
func yamlKeyName(n *yaml.Node) (string, error) {
	if n.Kind == yaml.AliasNode && n.Alias != nil {
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
