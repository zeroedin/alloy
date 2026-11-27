package template_test

import (
	"fmt"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	tmpl "github.com/zeroedin/alloy/internal/template"
)

// ── sort puts numbers before text (issue #1418) ─────────────────────────
// The sort filter compares values in one order, with or without a key:
//  1. numbers, smallest first: ints and floats, negatives and fractions
//     included, and strings of digits only;
//  2. everything else, as text;
//  3. missing or nil values, last.
//
// Equal values keep their input order.
//
// Before, a number and a text value compared as two strings. So 2 < 10
// (numbers), "10" < "1a" and "1a" < "2" (strings) made a loop, and the
// result depended on the input order.
//
// See PLAN.md → "How the sort filter compares values".
var _ = Describe("sort puts numbers before text (issue #1418)", func() {
	// perms returns every order of vals.
	var perms func(vals []interface{}) [][]interface{}
	perms = func(vals []interface{}) [][]interface{} {
		if len(vals) <= 1 {
			return [][]interface{}{append([]interface{}{}, vals...)}
		}
		var out [][]interface{}
		for i := range vals {
			rest := append(append([]interface{}{}, vals[:i]...), vals[i+1:]...)
			for _, p := range perms(rest) {
				out = append(out, append([]interface{}{vals[i]}, p...))
			}
		}
		return out
	}

	// items wraps each value as {"v": value}, or {} for a missing value.
	items := func(vals []interface{}) []interface{} {
		out := make([]interface{}, len(vals))
		for i, v := range vals {
			if v == absent {
				out[i] = map[string]interface{}{}
				continue
			}
			out[i] = map[string]interface{}{"v": v}
		}
		return out
	}

	// show prints a sorted list as 2;10;1a;, with - for a missing value.
	show := func(result interface{}, byKey bool) string {
		var b strings.Builder
		for _, item := range result.([]interface{}) {
			v := item
			if byKey {
				m := item.(map[string]interface{})
				var ok bool
				if v, ok = m["v"]; !ok {
					b.WriteString("-;")
					continue
				}
			}
			fmt.Fprintf(&b, "%v;", v)
		}
		return b.String()
	}

	Describe(`with "2", "10" and "1a"`, func() {
		for _, p := range perms([]interface{}{"2", "10", "1a"}) {
			in := p
			It(fmt.Sprintf("sorts %v as 2, 10, 1a by key", in), func() {
				Expect(show(tmpl.Sort(items(in), "v"), true)).To(Equal("2;10;1a;"))
			})
			It(fmt.Sprintf("sorts %v as 2, 10, 1a with no key", in), func() {
				Expect(show(tmpl.Sort(in), false)).To(Equal("2;10;1a;"))
			})
		}
	})

	Describe("with every kind of value", func() {
		// -3 and 10 are ints, 1.5 a float, "2" a digit-only string. "1a",
		// "b" and "-1" are text: "-1" has a sign, so it isn't digits only.
		want := "-3;1.5;2;10;-1;1a;b;-;"
		for _, in := range [][]interface{}{
			{"b", 10, absent, "1a", 1.5, "-1", "2", -3},
			{absent, "1a", "2", "b", -3, 10, "-1", 1.5},
			{"2", "-1", 1.5, absent, 10, "b", -3, "1a"},
		} {
			in := in
			It(fmt.Sprintf("sorts %v as numbers, then text, then missing", in), func() {
				Expect(show(tmpl.Sort(items(in), "v"), true)).To(Equal(want))
			})
		}
	})

	It("compares fractions as numbers (guard)", func() {
		in := []interface{}{2, 1.5, "10", 1.25}
		Expect(show(tmpl.Sort(items(in), "v"), true)).To(Equal("1.25;1.5;2;10;"))
	})

	It("keeps equal values in their input order (guard)", func() {
		in := []interface{}{
			map[string]interface{}{"v": "2", "t": "first"},
			map[string]interface{}{"v": 1},
			map[string]interface{}{"v": 2, "t": "second"},
		}
		out := tmpl.Sort(in, "v").([]interface{})
		Expect(out[1].(map[string]interface{})["t"]).To(Equal("first"))
		Expect(out[2].(map[string]interface{})["t"]).To(Equal("second"))
	})
})

// absent marks a value the item doesn't have. It prints as "missing" in
// spec names.
type absentValue struct{}

func (absentValue) String() string { return "missing" }

var absent = absentValue{}
