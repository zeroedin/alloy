package ordered_test

import (
	"reflect"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/zeroedin/alloy/internal/ordered"
)

// ── Producer output alphabet (issue #1237) ────────────────────────────
//
// The Go template engine converts *ordered.Map values to
// map[string]interface{} by walking only three container shapes:
// *ordered.Map, map[string]interface{}, and []interface{}. That narrowing
// is only safe because the two functions that produce ordered maps emit
// nothing else — an ordered map can never be reachable through a typed
// container such as []map[string]interface{}, because nothing builds one.
//
// If a producer ever starts emitting a typed container, these tests fail
// and the engine's walk must be widened (reflection over slice and map
// kinds) before that producer ships. Fixing the producer or widening the
// walk are the valid responses; relaxing these assertions is not.
//
// Spec: PLAN.md → "Ordered Data in Go Templates (issue #1237)",
// "Which containers the walk must traverse".

var _ = Describe("Ordered-map producer output shapes (issue #1237)", func() {

	// walk asserts that every value in the tree is one of the three shapes
	// the engine's converter knows how to traverse, or a leaf scalar.
	var walk func(v interface{}, path string)
	walk = func(v interface{}, path string) {
		GinkgoHelper()
		switch val := v.(type) {
		case *ordered.Map:
			for _, kv := range val.Entries() {
				walk(kv.Value, path+"."+kv.Key)
			}
		case map[string]interface{}:
			for k, item := range val {
				walk(item, path+"."+k)
			}
		case []interface{}:
			for i, item := range val {
				walk(item, path+"[]")
				_ = i
			}
		default:
			// Leaf. It must not be a container the walk cannot traverse.
			if v == nil {
				return
			}
			k := reflect.TypeOf(v).Kind()
			Expect(k).NotTo(Or(Equal(reflect.Map), Equal(reflect.Slice), Equal(reflect.Array)),
				"at %s: producer emitted a %T, which is a container the Go "+
					"template converter does not traverse. An *ordered.Map nested "+
					"inside it would never be converted and dot notation would "+
					"fail silently. Widen the walk in markHTMLSafe (reflection "+
					"over slice and map kinds) before shipping this producer — "+
					"see PLAN.md, \"Which containers the walk must traverse\"",
				path, v)
		}
	}

	Context("UnmarshalJSONValue", func() {
		It("emits only *Map, []interface{}, and scalars", func() {
			v, err := ordered.UnmarshalJSONValue([]byte(`{
				"obj":    {"a": 1, "nested": {"b": 2}},
				"arr":    [1, "two", {"c": 3}, [4, {"d": 5}]],
				"scalar": "s",
				"num":    1.5,
				"bool":   true,
				"null":   null
			}`))
			Expect(err).NotTo(HaveOccurred())
			Expect(v).To(BeAssignableToTypeOf(&ordered.Map{}),
				"a JSON object must decode to *ordered.Map")
			walk(v, "$")
		})

		It("emits []interface{} for a root-level array", func() {
			v, err := ordered.UnmarshalJSONValue([]byte(`[{"a": 1}, [{"b": 2}]]`))
			Expect(err).NotTo(HaveOccurred())
			Expect(v).To(BeAssignableToTypeOf([]interface{}{}),
				"a root JSON array must decode to []interface{}, not a typed slice")
			walk(v, "$")
		})
	})

	Context("RewrapValue", func() {
		It("emits only *Map, []interface{}, and scalars", func() {
			in := map[string]interface{}{
				"obj": map[string]interface{}{"a": 1, "nested": map[string]interface{}{"b": 2}},
				"arr": []interface{}{1, "two", map[string]interface{}{"c": 3}},
				// An array whose elements are ALL objects — the commonest JSON
				// shape, and the one a producer is most likely to "optimize"
				// into a typed slice. A mixed array does not exercise that path.
				"objArr": []interface{}{
					map[string]interface{}{"a": 1},
					map[string]interface{}{"b": 2},
				},
				"nestedObjArr": []interface{}{
					map[string]interface{}{"inner": []interface{}{
						map[string]interface{}{"c": 3},
					}},
				},
				"scalar": "s",
			}
			walk(ordered.RewrapValue(in), "$")
		})

		It("passes typed containers through unchanged, so producers must not supply them", func() {
			// RewrapValue does not descend into typed containers — it returns
			// them via its default branch. This documents why the guard above
			// matters: a typed container reaching RewrapValue keeps whatever is
			// inside it, unconverted and unreachable by the engine's walk.
			typed := []map[string]interface{}{{"a": 1}}
			out := ordered.RewrapValue(typed)
			Expect(out).To(BeAssignableToTypeOf([]map[string]interface{}{}),
				"RewrapValue must leave a typed slice untouched — if this ever "+
					"changes to wrap typed containers, the engine's walk and "+
					"PLAN.md's container list must change together")
		})
	})
})
