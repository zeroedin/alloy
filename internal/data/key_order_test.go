package data_test

import (
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"gopkg.in/yaml.v3"

	"github.com/zeroedin/alloy/internal/data"
	"github.com/zeroedin/alloy/internal/ordered"
)

// ── Data files preserve author key order (issue #1262) ────────────────
//
// A data file's keys must reach templates and plugins in the order the
// author wrote them, whatever the format. Today only JSON does, so
// renaming nav.json to nav.yaml silently reorders a nav menu. Measured on
// d9c7cfe with one file saved three ways, keys written zebra/apple/middle:
//
//	Format  Go templates   Liquid
//	YAML    sorted         sorted
//	TOML    sorted         sorted
//	JSON    sorted         file order
//
// Liquid is where the split shows; Go templates sort every map shape
// (issue #1237) and are unaffected either way. Plugins see the same split.
//
// Scope is data files only — the _data.yaml cascade is deliberately
// excluded, because cascade.DeepMerge type-asserts map[string]interface{}
// and silently drops keys when handed an *ordered.Map.
//
// Spec: PLAN.md → "Data files preserve author key order (issue #1262)".

var _ = Describe("Data file key order (issue #1262)", func() {

	// Fixtures live in their own directory rather than testdata/, because
	// testdata/ is walked by the LoadDirectory tests and same-stem files
	// across formats are a deliberate build error there. Mirrors the
	// existing testdata-errors/ split.
	keyOrderDir := func() string {
		_, file, _, _ := runtime.Caller(0)
		return filepath.Join(filepath.Dir(file), "testdata-keyorder")
	}

	load := func(name string) interface{} {
		GinkgoHelper()
		v, err := data.LoadFileAny(filepath.Join(keyOrderDir(), name))
		Expect(err).NotTo(HaveOccurred())
		return v
	}

	orderedMap := func(v interface{}) *ordered.Map {
		GinkgoHelper()
		om, ok := v.(*ordered.Map)
		Expect(ok).To(BeTrue(),
			"expected *ordered.Map, got %T — without it there is no order to "+
				"preserve and every consumer falls back to sorted or random "+
				"iteration (issue #1262)", v)
		return om
	}

	fileOrder := []string{"white", "black", "accent", "brand", "surface"}

	// ── RED: order is preserved for every format ──────────────────────

	Context("Key order survives loading", func() {
		It("preserves YAML key order", func() {
			om := orderedMap(load("order-yaml.yaml"))
			Expect(om.Keys()).To(Equal(fileOrder),
				"YAML keys must reach consumers in file order — sorted output "+
					"here means yaml.Unmarshal into interface{} is still being "+
					"used instead of a yaml.Node walk")
		})

		It("preserves TOML key order", func() {
			om := orderedMap(load("order-toml.toml"))
			Expect(om.Keys()).To(Equal(fileOrder),
				"TOML keys must reach consumers in file order, via "+
					"MetaData.Keys() rather than the plain decode")
		})

		It("preserves TOML key order inside nested tables", func() {
			// Ordering only the root passes the test above. MetaData.Keys()
			// reports every level as dotted paths, so each level has to be
			// reassembled against the paths for that level.
			om := orderedMap(load("order-nested.toml"))
			Expect(om.Keys()).To(Equal([]string{"zebra", "apple", "srv"}),
				"root keys must be in file order (sorted would be apple, srv, zebra)")

			srv := orderedMap(om.Get("srv"))
			Expect(srv.Keys()).To(Equal([]string{"port", "host", "auth", "tls"}),
				"a nested [table] must be ordered too — sorted would be "+
					"auth, host, port, tls")

			tls := orderedMap(srv.Get("tls"))
			Expect(tls.Keys()).To(Equal([]string{"on", "ca"}),
				"a [table.subtable] must be ordered at its own level — "+
					"sorted would be ca, on")
		})

		It("preserves TOML key order for dotted keys with no table header", func() {
			// MetaData.Keys() reports only the leaf of a dotted key —
			// "zebra.value", never a bare "zebra" — so a parent that never
			// appears as a terminal key has to be recorded as it is walked
			// past, or it loses its position and lands in the sorted
			// remainder behind keys written after it.
			om := orderedMap(load("order-dotted.toml"))
			Expect(om.Keys()).To(Equal([]string{"zebra", "apple", "middle"}),
				"implicit parents of dotted keys must hold their file "+
					"position (sorted would be apple, middle, zebra)")

			middle := orderedMap(om.Get("middle"))
			Expect(middle.Keys()).To(Equal([]string{"deep"}),
				"an implicit parent must still carry its own children")
		})

		It("preserves JSON key order, unchanged (issue #453)", func() {
			// Green guard: JSON already did this, and must keep doing it.
			v, err := data.LoadFileAny(filepath.Join(testdataDir(), "ordered-keys.json"))
			Expect(err).NotTo(HaveOccurred())
			om := orderedMap(v)
			Expect(om.Keys()).To(Equal(fileOrder),
				"JSON ordering predates this change and must not regress")
		})

		It("preserves order at every nesting level", func() {
			// A single-level conversion passes the first test and fails here.
			om := orderedMap(load("order-nested.yaml"))
			Expect(om.Keys()).To(Equal([]string{"zebra", "nested", "apple", "list"}),
				"top level must be in file order")

			nested := orderedMap(om.Get("nested"))
			Expect(nested.Keys()).To(Equal([]string{"second", "first", "deeper"}),
				"nested mappings must be ordered too — calling Decode on a "+
					"mapping node produces a plain map and loses order here")

			deeper := orderedMap(nested.Get("deeper"))
			Expect(deeper.Keys()).To(Equal([]string{"y", "x"}),
				"ordering must hold at arbitrary depth")
		})

		It("preserves order inside sequence elements", func() {
			om := orderedMap(load("order-nested.yaml"))
			list, ok := om.Get("list").([]interface{})
			Expect(ok).To(BeTrue(),
				"a YAML sequence must decode to []interface{}, not a typed slice")
			Expect(list).To(HaveLen(1))

			elem := orderedMap(list[0])
			Expect(elem.Keys()).To(Equal([]string{"beta", "alpha"}),
				"mappings inside sequences must be ordered — the walk has to "+
					"recurse through sequence nodes, not just mapping nodes")
		})
	})

	// ── RED: container shapes the template converter can traverse ─────

	Context("Output shapes stay traversable", func() {
		It("normalizes TOML array-of-tables to []interface{}", func() {
			// toml.Decode returns []map[string]interface{}, which the Go
			// template converter does not traverse (PLAN.md, "Which
			// containers the walk must traverse"). Left typed, its elements
			// would stay *ordered.Map and dot notation would fail at render:
			//   {{ range .site.data.nav.items }}{{ .name }};{{ end }}
			// works on d9c7cfe and must still work after this change.
			om := orderedMap(load("order-tables.toml"))
			items := om.Get("items")

			_, typed := items.([]map[string]interface{})
			Expect(typed).To(BeFalse(),
				"array-of-tables must not stay a typed []map[string]interface{} — "+
					"the Go engine's converter skips that shape, so ordered maps "+
					"inside it are never converted and {{ .name }} fails with "+
					"\"can't evaluate field name\"")

			arr, ok := items.([]interface{})
			Expect(ok).To(BeTrue(),
				"array-of-tables must be normalized to []interface{}, got %T", items)
			Expect(arr).To(HaveLen(2))

			first := orderedMap(arr[0])
			Expect(first.Keys()).To(Equal([]string{"name", "url"}),
				"keys within an array-of-tables element must keep file order")
			Expect(first.Get("name")).To(Equal("first"))
		})

		It("emits no container the template converter cannot traverse", func() {
			// The same alphabet issue #1237 requires of the JSON producers:
			// *ordered.Map, []interface{}, scalars. Anything else hides
			// ordered maps from the Go engine's walk.
			var walk func(v interface{}, path string)
			walk = func(v interface{}, path string) {
				GinkgoHelper()
				switch val := v.(type) {
				case *ordered.Map:
					for _, kv := range val.Entries() {
						walk(kv.Value, path+"."+kv.Key)
					}
				case map[string]interface{}:
					// Traversable by the Go engine, but it has already lost
					// author order — Liquid and plugins would see sorted keys.
					// After this change no loaded object should be a plain map.
					Fail("at " + path + ": loader returned a plain " +
						"map[string]interface{}. It is traversable, but its key " +
						"order is gone, so Liquid and plugins still see sorted " +
						"keys. Every object a data file produces must be an " +
						"*ordered.Map (issue #1262)")
				case []interface{}:
					for _, item := range val {
						walk(item, path+"[]")
					}
				default:
					if v == nil {
						return
					}
					k := reflect.TypeOf(v).Kind()
					Expect(k).NotTo(Or(Equal(reflect.Map), Equal(reflect.Slice), Equal(reflect.Array)),
						"at %s: loader emitted a %T, a container the Go template "+
							"converter does not traverse — an *ordered.Map inside "+
							"it would never be converted and dot notation would "+
							"fail silently", path, v)
				}
			}
			for _, f := range []string{
				"order-yaml.yaml", "order-toml.toml", "order-nested.yaml",
				"order-nested.toml", "order-tables.toml", "order-scalars.yaml",
				"order-merge.yaml", "order-dotted.toml",
			} {
				walk(load(f), f)
			}
		})
	})

	// ── GREEN GUARDS: things this must not disturb ────────────────────

	Context("YAML semantics the existing decoder provides", func() {
		// A yaml.Node walk bypasses yaml.Unmarshal entirely, so every
		// semantic that decoder applied has to be reapplied by hand.
		// Both of these pass today and must keep passing.

		It("still rejects duplicate mapping keys", func() {
			// yaml.Unmarshal errors on a repeated key; decoding into a
			// yaml.Node does not. A naive Content loop would silently accept
			// the file and let the last value win, turning a fatal malformed
			// file into a silent one. Verified against the built CLI on main:
			//   Error: ... parsing YAML .../dup.yaml: yaml: unmarshal errors:
			//     line 3: mapping key "a" already defined at line 1
			_, err := data.LoadFileAny(filepath.Join(keyOrderDir(), "order-duplicate.yaml"))
			Expect(err).To(HaveOccurred(),
				"a duplicate mapping key must stay a fatal parse error — "+
					"PLAN.md requires malformed data files to fail the build "+
					"(issue #982), and a yaml.Node walk does not enforce this "+
					"on its own")
			Expect(err.Error()).To(ContainSubstring("already defined"),
				"the error must still name the duplicate, so the author can "+
					"find it; got: %v", err)
		})

		It("rejects a duplicate merge key", func() {
			// yaml.Node keeps both "<<" entries in a mapping's Content, and
			// yaml.Unmarshal rejects the repeat exactly as it rejects any
			// other duplicate key:
			//   line 7: mapping key "<<" already defined at line 6
			// Skipping merge keys before duplicate tracking would let this
			// file through and silently apply both merges.
			_, err := data.LoadFileAny(filepath.Join(keyOrderDir(), "order-dup-merge.yaml"))
			Expect(err).To(HaveOccurred(),
				"a repeated \"<<\" must stay a fatal parse error — the plain "+
					"decoder rejects it, so the node walk has to as well")
			Expect(err.Error()).To(ContainSubstring("already defined"),
				"the error must name the duplicate; got: %v", err)
		})

		It("rejects a self-referential alias instead of overflowing the stack", func() {
			// "a: &a [*a]" makes the walk re-enter the node it is already
			// expanding. Without an active-alias set this is not a slow
			// build or a panic a caller could recover — it is a fatal
			// runtime stack overflow that takes the process down, with no
			// file named. yaml.Unmarshal reports "anchor 'a' value contains
			// itself".
			_, err := data.LoadFileAny(filepath.Join(keyOrderDir(), "order-recursive.yaml"))
			Expect(err).To(HaveOccurred(),
				"a recursive alias must be a parse error, not a crash")
			Expect(err.Error()).To(ContainSubstring("contains itself"),
				"the error must say the anchor references itself, as the "+
					"plain decoder does; got: %v", err)
		})

		It("rejects a document whose aliases expand without bound", func() {
			// 716 bytes that expand to ~10^8 nodes. yaml.Unmarshal bounds
			// alias-driven work against document size and rejects this;
			// a node walk without the same budget would sit there building
			// it. The thresholds and ratio curve are taken from yaml.v3, so
			// a document this walk accepts is one the plain decode accepted.
			_, err := data.LoadFileAny(filepath.Join(keyOrderDir(), "order-aliasbomb.yaml"))
			Expect(err).To(HaveOccurred(),
				"unbounded alias expansion must be refused, matching the "+
					"plain decoder rather than attempting the expansion")
			Expect(err.Error()).To(ContainSubstring("excessive aliasing"),
				"the error must match the plain decoder's wording; got: %v", err)
		})

		It("still resolves merge keys and aliases", func() {
			// yaml.Unmarshal expands "<<: *base" into the parent mapping.
			// A naive node walk leaves a literal "<<" key holding the merged
			// map, and the merged-in keys never appear. Verified on main:
			//   DERIVED: x=1 y=99
			// Asserted through LoadFile, which returns a plain map both
			// before and after this change, so this establishes a real
			// baseline rather than only describing the post-change world.
			result, err := data.LoadFile(filepath.Join(keyOrderDir(), "order-merge.yaml"))
			Expect(err).NotTo(HaveOccurred())

			derived, ok := result["derived"].(map[string]interface{})
			Expect(ok).To(BeTrue(), "derived must be a map, got %T", result["derived"])

			Expect(derived).NotTo(HaveKey("<<"),
				"the merge key must be resolved, not carried through as a "+
					"literal \"<<\" key — a yaml.Node walk does not expand it")
			Expect(derived["x"]).To(Equal(1),
				"merged-in keys must be present — x comes from the anchor and "+
					"disappears entirely if << is left unresolved")
			Expect(derived["y"]).To(Equal(99),
				"a local key must override the merged value")

			Expect(result["scalar_ref"]).To(Equal("hello"),
				"a scalar alias must resolve to its anchor's value")
		})
	})

	Context("Scalar types are unchanged", func() {
		It("keeps every scalar type identical to the plain decode", func() {
			// Date fidelity is load-bearing: page sorting and date filters
			// need time.Time. Reading node.Value as a string would demote
			// dates and numbers and break them silently.
			om := orderedMap(load("order-scalars.yaml"))

			// Compare against the values the existing decoder produces, not
			// just the type: a decoder could return a zero or shifted
			// timestamp and still be a time.Time.
			var viaUnmarshal map[string]interface{}
			raw, err := os.ReadFile(filepath.Join(keyOrderDir(), "order-scalars.yaml"))
			Expect(err).NotTo(HaveOccurred())
			Expect(yaml.Unmarshal(raw, &viaUnmarshal)).To(Succeed())

			for _, key := range []string{"date", "stamp"} {
				want, ok := viaUnmarshal[key].(time.Time)
				Expect(ok).To(BeTrue(),
					"fixture sanity: %s must decode to time.Time via yaml.Unmarshal", key)
				got, ok := om.Get(key).(time.Time)
				Expect(ok).To(BeTrue(),
					"%s must stay time.Time, not become a string — sort and "+
						"date filters depend on it; got %T", key, om.Get(key))
				Expect(got.Equal(want)).To(BeTrue(),
					"%s must equal the value the existing decoder produces: "+
						"want %v, got %v", key, want, got)
				Expect(got.Location().String()).To(Equal(want.Location().String()),
					"%s must keep the same location as the existing decoder — "+
						"a shifted zone changes rendered dates", key)
			}
			Expect(om.Get("int")).To(Equal(42),
				"integers must stay int, not string or float64")
			Expect(om.Get("float")).To(Equal(1.5))
			Expect(om.Get("bool")).To(Equal(true))
			Expect(om.Get("nul")).To(BeNil())
			Expect(om.Get("quoted")).To(Equal("text"))
			Expect(om.Get("bare")).To(Equal("text"))
		})
	})

	Context("Existing loader behavior holds", func() {
		It("LoadFile still returns a plain map", func() {
			// LoadFile's signature and flattening are relied on by callers
			// that want a plain map; only LoadFileAny changes.
			result, err := data.LoadFile(filepath.Join(keyOrderDir(), "order-yaml.yaml"))
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(BeAssignableToTypeOf(map[string]interface{}{}),
				"LoadFile must keep returning map[string]interface{} — it "+
					"flattens through ToGoMap and its signature is unchanged")
			Expect(result).To(HaveKey("white"))
		})

		It("still loads a root-level YAML array", func() {
			result, err := data.LoadFileAny(filepath.Join(testdataDir(), "members.yaml"))
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(BeAssignableToTypeOf([]interface{}{}),
				"a root-level sequence must still decode to []interface{}")
		})

		It("still fails on an unsupported extension", func() {
			_, err := data.LoadFileAny(filepath.Join(testdataDir(), "team.csv"))
			Expect(err).To(HaveOccurred(),
				"LoadFileAny must keep rejecting .csv — CSV goes through "+
					"LoadCSV and is out of scope for ordering")
		})
	})
})
