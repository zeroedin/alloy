package data_test

import (
	"path/filepath"
	"runtime"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/zeroedin/alloy/internal/data"
	"github.com/zeroedin/alloy/internal/ordered"
)

// ── Inline tables in a TOML array keep their own key order (issue #1414) ──
// MetaData.Keys() lists an inline array's tables one after another, with no
// index and no marker between them. Before #1414 every table took the first
// table's order. A repeated key is not a reliable boundary: a table can
// start with a key the table before it didn't have (shift below).
//
// shape prints a loaded value with its keys in order: {z a} is a table
// with keys z then a, [..] an array, _ a scalar, n:{y x} a key whose value
// is a table.
//
// See PLAN.md → "Inline tables in an array keep their own order too
// (issue #1414)".
var _ = Describe("Inline tables in a TOML array keep their own key order (issue #1414)", Ordered, ContinueOnFailure, func() {
	var shape func(v interface{}) string
	shape = func(v interface{}) string {
		switch val := v.(type) {
		case *ordered.Map:
			parts := make([]string, 0, val.Len())
			for _, kv := range val.Entries() {
				switch kv.Value.(type) {
				case *ordered.Map, []interface{}:
					parts = append(parts, kv.Key+":"+shape(kv.Value))
				default:
					parts = append(parts, kv.Key)
				}
			}
			return "{" + strings.Join(parts, " ") + "}"
		case []interface{}:
			parts := make([]string, len(val))
			for i, e := range val {
				parts[i] = shape(e)
			}
			return "[" + strings.Join(parts, " ") + "]"
		default:
			return "_"
		}
	}

	var doc *ordered.Map
	BeforeAll(func() {
		_, file, _, _ := runtime.Caller(0)
		v, err := data.LoadFileAny(filepath.Join(filepath.Dir(file), "testdata-keyorder", "order-inline-arrays.toml"))
		Expect(err).NotTo(HaveOccurred())
		var ok bool
		doc, ok = v.(*ordered.Map)
		Expect(ok).To(BeTrue(), "expected *ordered.Map, got %T", v)
	})
	of := func(key string) string {
		GinkgoHelper()
		Expect(doc.Has(key)).To(BeTrue(), "the fixture has no key %q", key)
		return shape(doc.Get(key))
	}

	It("keeps each table's order in an array of inline tables", func() {
		Expect(of("items")).To(Equal("[{z a} {a z}]"))
	})
	It("keeps the order of a table that starts with a key the table before it doesn't have", func() {
		Expect(of("shift")).To(Equal("[{b c} {a b}]"))
	})
	It("keeps each of three tables' order", func() {
		Expect(of("three")).To(Equal("[{z a} {a z} {m z}]"))
	})
	It("keeps the order of an inline table inside each table", func() {
		Expect(of("deep")).To(Equal("[{n:{y x}} {n:{x y}}]"))
	})
	It("keeps each table's order in an array inside an inline table", func() {
		Expect(of("inner")).To(Equal("{list:[{b a} {a b}]}"))
	})
	It("keeps each table's order in an array of arrays", func() {
		Expect(of("nested")).To(Equal("[[{b a}] [{a b} {c a}]]"))
	})
	It("keeps each table's order in an array that mixes tables and scalars", func() {
		Expect(of("mixed")).To(Equal("[_ {b a} _ {a b}]"))
	})
	It("keeps each table's order next to empty tables", func() {
		Expect(of("empty")).To(Equal("[{} {b a} {} {a b}]"))
	})
	It("keeps each table's order when a key holds an empty table", func() {
		Expect(of("emptyval")).To(Equal("[{n:{} b} {b n:{}}]"))
	})
	It("keeps each table's order with dotted keys", func() {
		Expect(of("dotted")).To(Equal("[{p:{y x} q} {q p:{x y}}]"))
	})
	It("keeps each table's order when a key holds an array of scalars", func() {
		Expect(of("lists")).To(Equal("[{l:[_ _] b} {b l:[_]}]"))
	})
	It("keeps each table's order in an inline array inside each [[table]]", func() {
		Expect(of("rows")).To(Equal("[{x:[{b a} {a b}]} {x:[{c a}]}]"))
	})

	// Issue #1425: two cases a first fix for #1414 got wrong, and more
	// dotted keys inside inline tables.
	It("keeps a table's order when a dotted key comes back after another key", func() {
		Expect(of("interleaved")).To(Equal("[{p:{z y x} q}]"))
	})
	It("keeps every level's order when dotted keys at two depths interleave", func() {
		Expect(of("deepdotted")).To(Equal("[{a:{b:{y x} z} c}]"))
	})
	It("keeps each table's order when a dotted key holds an inline array", func() {
		Expect(of("dottedarray")).To(Equal("[{p:{l:[{b a} {a b}] m} q} {q p:{m}}]"))
	})
	It("starts the next table when a dotted key's parent repeats", func() {
		Expect(of("splitdotted")).To(Equal("[{p:{z} q} {p:{y z}}]"))
	})
	It("gives a later [[table]] element its own keys under an earlier inline array's path", func() {
		Expect(of("scope")).To(Equal("[{x:[]} {x:{child:{z a}}}]"))
	})

	// A path that is a [[table]] array in one element and an inline array
	// in another: MetaData keeps one type per path, the last one written.
	Describe("when a path is a [[table]] array in one element and an inline array in another", func() {
		var mixed *ordered.Map
		BeforeAll(func() {
			_, file, _, _ := runtime.Caller(0)
			v, err := data.LoadFileAny(filepath.Join(filepath.Dir(file), "testdata-keyorder", "order-mixed-array-kinds.toml"))
			Expect(err).NotTo(HaveOccurred())
			mixed = v.(*ordered.Map)
		})
		It("keeps each [[table]]'s order when a later element writes an inline array there (guard)", func() {
			Expect(shape(mixed.Get("aot"))).To(Equal("[{x:[{z a} {z a}]} {x:[]}]"))
		})
		It("keeps each inline table's order when an earlier element wrote [[table]]s there", func() {
			Expect(shape(mixed.Get("mix"))).To(Equal("[{x:[{z a}]} {x:[{b a} {a b}]}]"))
		})

		// Issue #1434: the inline array comes first. The last write to the
		// path is a [[table]] header, so MetaData calls the inline array an
		// array of tables too.
		It("keeps each inline table's order when a later element writes [[table]]s there", func() {
			Expect(shape(mixed.Get("rev"))).To(Equal("[{x:[{b a} {a b}]} {x:[{z a}]}]"))
		})
		It("keeps each inline table's order when two earlier elements wrote inline arrays there", func() {
			Expect(shape(mixed.Get("revtwo"))).To(Equal("[{x:[{b a}]} {x:[{d c} {c d}]} {x:[{z a}]}]"))
		})
		It("keeps each table's order when an inline array comes between two [[table]] elements", func() {
			Expect(shape(mixed.Get("between"))).To(Equal("[{x:[{z a}]} {x:[{b a} {a b}]} {x:[{y c}]}]"))
		})
		It("keeps a [[table]]'s order after an empty inline array at the same path (guard)", func() {
			Expect(shape(mixed.Get("revempty"))).To(Equal("[{x:[]} {x:[{z a}]}]"))
		})
	})

	// Guards: these hold today and must keep holding.
	It("keeps a single inline table's order (guard)", func() {
		Expect(of("single")).To(Equal("[{b a}]"))
	})
	It("keeps a table's key order when it is written after its sub-table (guard)", func() {
		// A key repeats in Keys() here without any inline array: a fix that
		// reads every repeated key as a new array element sorts these keys.
		_, file, _, _ := runtime.Caller(0)
		v, err := data.LoadFileAny(filepath.Join(filepath.Dir(file), "testdata-keyorder", "order-table-after-subtable.toml"))
		Expect(err).NotTo(HaveOccurred())
		Expect(shape(v)).To(Equal("{a:{b:{c:{x} q p} z y}}"))
	})
	It("keeps the top-level keys in file order (guard)", func() {
		Expect(doc.Keys()).To(Equal([]string{
			"items", "shift", "three", "deep", "inner", "nested", "mixed",
			"empty", "emptyval", "dotted", "lists", "single", "interleaved",
			"deepdotted", "dottedarray", "splitdotted", "rows", "scope",
		}))
	})
})
