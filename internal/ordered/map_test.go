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
})
