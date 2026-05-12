package ordered_test

import (
	"encoding/json"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/zeroedin/alloy/internal/ordered"
)

var _ = Describe("Ordered Map", func() {

	// ── Basic operations ────────────────────────────────────────────

	Context("Set and Get", func() {
		It("preserves insertion order", func() {
			m := ordered.New()
			m.Set("white", "#ffffff")
			m.Set("black", "#000000")
			m.Set("accent", "#ee0000")
			m.Set("brand", "#e00")

			Expect(m.Keys()).To(Equal([]string{"white", "black", "accent", "brand"}),
				"Keys() must return keys in insertion order")
		})

		It("Get returns the correct value", func() {
			m := ordered.New()
			m.Set("name", "Alice")
			Expect(m.Get("name")).To(Equal("Alice"))
		})

		It("GetValue returns ok=false for missing keys", func() {
			m := ordered.New()
			_, ok := m.GetValue("missing")
			Expect(ok).To(BeFalse())
		})

		It("Set does not change order for existing keys", func() {
			m := ordered.New()
			m.Set("a", 1)
			m.Set("b", 2)
			m.Set("a", 99) // update, not re-insert

			Expect(m.Keys()).To(Equal([]string{"a", "b"}),
				"updating an existing key must not change its position")
			Expect(m.Get("a")).To(Equal(99),
				"value must be updated")
		})

		It("Has returns true for existing keys", func() {
			m := ordered.New()
			m.Set("x", 1)
			Expect(m.Has("x")).To(BeTrue())
			Expect(m.Has("y")).To(BeFalse())
		})

		It("Len returns key count", func() {
			m := ordered.New()
			Expect(m.Len()).To(Equal(0))
			m.Set("a", 1)
			m.Set("b", 2)
			Expect(m.Len()).To(Equal(2))
		})

		It("Entries returns KVPairs in insertion order", func() {
			m := ordered.New()
			m.Set("x", 10)
			m.Set("y", 20)

			entries := m.Entries()
			Expect(entries).To(HaveLen(2))
			Expect(entries[0].Key).To(Equal("x"))
			Expect(entries[0].Value).To(Equal(10))
			Expect(entries[1].Key).To(Equal("y"))
			Expect(entries[1].Value).To(Equal(20))
		})
	})

	// ── Delete ──────────────────────────────────────────────────────

	Context("Delete", func() {
		It("removes a key and maintains order of remaining keys", func() {
			m := ordered.New()
			m.Set("a", 1)
			m.Set("b", 2)
			m.Set("c", 3)
			m.Delete("b")

			Expect(m.Keys()).To(Equal([]string{"a", "c"}),
				"remaining keys must preserve their original order")
			Expect(m.Has("b")).To(BeFalse())
			Expect(m.Len()).To(Equal(2))
		})
	})

	// ── JSON unmarshal ──────────────────────────────────────────────

	Context("UnmarshalJSON", func() {
		It("preserves key insertion order from JSON", func() {
			input := `{"white":"#fff","black":"#000","accent":"#e00","brand":"#ee0"}`
			m := ordered.New()
			err := json.Unmarshal([]byte(input), m)
			Expect(err).NotTo(HaveOccurred())

			Expect(m.Keys()).To(Equal([]string{"white", "black", "accent", "brand"}),
				"UnmarshalJSON must preserve the exact key order from the JSON source")
		})

		It("recursively creates ordered maps for nested objects", func() {
			input := `{"alice":{"name":"Alice","email":"alice@example.com"},"bob":{"name":"Bob","email":"bob@example.com"}}`
			m := ordered.New()
			err := json.Unmarshal([]byte(input), m)
			Expect(err).NotTo(HaveOccurred())

			Expect(m.Keys()).To(Equal([]string{"alice", "bob"}),
				"top-level keys must be in insertion order")

			alice, ok := m.Get("alice").(*ordered.Map)
			Expect(ok).To(BeTrue(),
				"nested JSON objects must be *ordered.Map, not map[string]interface{}")
			Expect(alice.Keys()).To(Equal([]string{"name", "email"}),
				"nested keys must preserve insertion order")
		})

		It("handles arrays correctly", func() {
			input := `{"tags":["go","web","alloy"]}`
			m := ordered.New()
			err := json.Unmarshal([]byte(input), m)
			Expect(err).NotTo(HaveOccurred())

			tags, ok := m.Get("tags").([]interface{})
			Expect(ok).To(BeTrue(), "JSON arrays must remain []interface{}")
			Expect(tags).To(Equal([]interface{}{"go", "web", "alloy"}))
		})

		It("handles mixed value types", func() {
			input := `{"name":"Test","count":42,"active":true,"data":null}`
			m := ordered.New()
			err := json.Unmarshal([]byte(input), m)
			Expect(err).NotTo(HaveOccurred())

			Expect(m.Keys()).To(Equal([]string{"name", "count", "active", "data"}))
			Expect(m.Get("name")).To(Equal("Test"))
			Expect(m.Get("count")).To(BeNumerically("==", 42))
			Expect(m.Get("active")).To(BeTrue())
			Expect(m.Get("data")).To(BeNil())
		})
	})

	// ── JSON marshal ────────────────────────────────────────────────

	Context("MarshalJSON", func() {
		It("emits keys in insertion order", func() {
			m := ordered.New()
			m.Set("z", 1)
			m.Set("a", 2)
			m.Set("m", 3)

			data, err := json.Marshal(m)
			Expect(err).NotTo(HaveOccurred())
			Expect(string(data)).To(Equal(`{"z":1,"a":2,"m":3}`),
				"MarshalJSON must emit keys in insertion order, not sorted")
		})

		It("round-trips JSON preserving order", func() {
			input := `{"white":"#fff","black":"#000","accent":"#e00"}`
			m := ordered.New()
			Expect(json.Unmarshal([]byte(input), m)).To(Succeed())

			output, err := json.Marshal(m)
			Expect(err).NotTo(HaveOccurred())
			Expect(string(output)).To(Equal(input),
				"JSON round-trip must preserve key order exactly")
		})
	})

	// ── Liquid integration (issue #456) ─────────────────────────────
	// ordered.Map must work with liquidgo's iteration and property
	// access interfaces for templates to consume JSON data in order.

	Context("Liquid integration", func() {
		It("Each yields [key, value] pairs in insertion order", func() {
			m := ordered.New()
			m.Set("white", "#fff")
			m.Set("black", "#000")
			m.Set("accent", "#e00")

			var keys []string
			m.Each(func(pair interface{}) {
				kv, ok := pair.([]interface{})
				Expect(ok).To(BeTrue(), "Each must yield []interface{}{key, value}")
				Expect(kv).To(HaveLen(2))
				keys = append(keys, kv[0].(string))
			})

			Expect(keys).To(Equal([]string{"white", "black", "accent"}),
				"Each must iterate in insertion order — "+
					"Liquid {% for item in data %} depends on this")
		})

		It("LiquidMethodMissing enables property access", func() {
			m := ordered.New()
			m.Set("name", "Alice")
			m.Set("role", "Engineer")

			result := m.LiquidMethodMissing("name")
			Expect(result).To(Equal("Alice"),
				"LiquidMethodMissing must return the value for the key — "+
					"{{ site.data.team.name }} calls LiquidMethodMissing(\"name\")")

			missing := m.LiquidMethodMissing("nonexistent")
			Expect(missing).To(BeNil(),
				"LiquidMethodMissing must return nil for missing keys")
		})

		It("First returns the first [key, value] pair", func() {
			m := ordered.New()
			m.Set("white", "#fff")
			m.Set("black", "#000")

			first := m.First()
			kv, ok := first.([]interface{})
			Expect(ok).To(BeTrue())
			Expect(kv[0]).To(Equal("white"))
			Expect(kv[1]).To(Equal("#fff"))
		})

		It("First returns nil for empty map", func() {
			m := ordered.New()
			Expect(m.First()).To(BeNil())
		})
	})

	// ── UnmarshalJSONValue (issue #456) ─────────────────────────────

	Context("UnmarshalJSONValue", func() {
		It("parses top-level object as *Map", func() {
			result, err := ordered.UnmarshalJSONValue([]byte(`{"a":1,"b":2}`))
			Expect(err).NotTo(HaveOccurred())

			m, ok := result.(*ordered.Map)
			Expect(ok).To(BeTrue())
			Expect(m.Keys()).To(Equal([]string{"a", "b"}))
		})

		It("parses top-level array preserving nested object order", func() {
			result, err := ordered.UnmarshalJSONValue([]byte(`[{"z":1},{"a":2}]`))
			Expect(err).NotTo(HaveOccurred())

			arr, ok := result.([]interface{})
			Expect(ok).To(BeTrue())
			Expect(arr).To(HaveLen(2))

			first, ok := arr[0].(*ordered.Map)
			Expect(ok).To(BeTrue())
			Expect(first.Keys()).To(Equal([]string{"z"}))
		})

		It("parses scalar values", func() {
			result, err := ordered.UnmarshalJSONValue([]byte(`"hello"`))
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal("hello"))
		})
	})

	// ── RewrapValue (issue #571) ────────────────────────────────────
	// After JSON round-trip through plugin serialization, *ordered.Map
	// values become map[string]interface{}. RewrapValue converts them
	// back so Each() and LiquidMethodMissing() are available.

	Context("RewrapValue (issue #571)", func() {
		It("converts map[string]interface{} to *ordered.Map", func() {
			input := map[string]interface{}{"a": 1, "b": 2}
			result := ordered.RewrapValue(input)
			m, ok := result.(*ordered.Map)
			Expect(ok).To(BeTrue(),
				"RewrapValue must convert map[string]interface{} to *ordered.Map — "+
					"this restores Each() and LiquidMethodMissing() after JSON round-trip (issue #571)")
			Expect(m.Len()).To(Equal(2))
			Expect(m.Get("a")).To(BeNumerically("==", 1))
			Expect(m.Get("b")).To(BeNumerically("==", 2))
		})

		It("recursively converts nested maps", func() {
			input := map[string]interface{}{
				"color": map[string]interface{}{
					"red": "#f00",
				},
			}
			result := ordered.RewrapValue(input)
			m, ok := result.(*ordered.Map)
			Expect(ok).To(BeTrue())
			nested, ok := m.Get("color").(*ordered.Map)
			Expect(ok).To(BeTrue(),
				"nested map[string]interface{} must also be converted to *ordered.Map — "+
					"JSON data structures are deeply nested (issue #571)")
			Expect(nested.Get("red")).To(Equal("#f00"))
		})

		It("converts maps inside arrays", func() {
			input := []interface{}{
				map[string]interface{}{"name": "Alice"},
				map[string]interface{}{"name": "Bob"},
			}
			result := ordered.RewrapValue(input)
			arr, ok := result.([]interface{})
			Expect(ok).To(BeTrue(), "arrays must remain arrays")
			Expect(arr).To(HaveLen(2))
			first, ok := arr[0].(*ordered.Map)
			Expect(ok).To(BeTrue(),
				"maps inside arrays must be converted — JSON data files "+
					"contain arrays of objects (issue #571)")
			Expect(first.Get("name")).To(Equal("Alice"))
		})

		It("passes through primitives unchanged", func() {
			Expect(ordered.RewrapValue("hello")).To(Equal("hello"))
			Expect(ordered.RewrapValue(42.0)).To(BeNumerically("==", 42))
			Expect(ordered.RewrapValue(true)).To(BeTrue())
			Expect(ordered.RewrapValue(nil)).To(BeNil())
		})

		It("passes through *ordered.Map unchanged", func() {
			m := ordered.New()
			m.Set("a", 1)
			result := ordered.RewrapValue(m)
			Expect(result).To(BeIdenticalTo(m),
				"RewrapValue must not re-wrap an already-ordered map — "+
					"avoids unnecessary allocation when data was never serialized (issue #571)")
		})
	})
})
