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
//     included, and strings of digits only, however long;
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

	// A digit-only string too long for a float64 (400 digits) is still a
	// number (issue #1443). strconv.ParseFloat gives +Inf and an error for it.
	It("sorts a 400-digit string as a number, before text and after smaller numbers", func() {
		huge := strings.Repeat("9", 400)
		for _, in := range [][]interface{}{
			{"!", huge, 5},
			{huge, 5, "!"},
		} {
			Expect(show(tmpl.Sort(items(in), "v"), true)).To(Equal("5;" + huge + ";!;"))
			Expect(show(tmpl.Sort(in), false)).To(Equal("5;" + huge + ";!;"))
		}
	})

	// Issue #1443: a digit-only string is a number however long it is, as in
	// a collection's sortBy. 9223372036854775808 is one past the largest
	// int64.
	for _, in := range [][]interface{}{
		{"9223372036854775808", "!"},
		{"!", "9223372036854775808"},
	} {
		in := in
		It(fmt.Sprintf("sorts %v as a number, then text, when the digits don't fit in an int", in), func() {
			Expect(show(tmpl.Sort(items(in), "v"), true)).To(Equal("9223372036854775808;!;"))
			Expect(show(tmpl.Sort(in), false)).To(Equal("9223372036854775808;!;"))
		})
	}

	// Issue #1443: two whole numbers compare exactly, however large. A
	// float64 holds whole numbers exactly only up to 2^53, so 2^53 + 1 and
	// 2^53 came out equal and kept their input order.
	Describe("with whole numbers above 2^53", func() {
		for _, c := range []struct {
			name string
			in   []interface{}
			want string
		}{
			{"ints", []interface{}{int64(9007199254740993), int64(9007199254740992)},
				"9007199254740992;9007199254740993;"},
			// YAML front matter gives a plain int, not an int64.
			{"plain ints", []interface{}{int(9007199254740993), int(9007199254740992)},
				"9007199254740992;9007199254740993;"},
			{"digit-only strings", []interface{}{"9007199254740993", "9007199254740992"},
				"9007199254740992;9007199254740993;"},
			{"an int and a digit-only string", []interface{}{int64(9007199254740993), "9007199254740992"},
				"9007199254740992;9007199254740993;"},
			{"digit-only strings too long for an int", []interface{}{"100000000000000000001", "100000000000000000000"},
				"100000000000000000000;100000000000000000001;"},
			// As text, "1000…" comes before "9999…"; as float64, both are 1e20.
			{"digit-only strings of different lengths", []interface{}{"100000000000000000000", "99999999999999999999"},
				"99999999999999999999;100000000000000000000;"},
		} {
			c := c
			It("sorts "+c.name+" exactly", func() {
				Expect(show(tmpl.Sort(items(c.in), "v"), true)).To(Equal(c.want))
				Expect(show(tmpl.Sort(c.in), false)).To(Equal(c.want))
			})
		}
		It("keeps equal whole numbers in their input order (guard)", func() {
			in := []interface{}{
				map[string]interface{}{"v": "0007", "t": "first"},
				map[string]interface{}{"v": int64(7), "t": "second"},
			}
			out := tmpl.Sort(in, "v").([]interface{})
			Expect(out[0].(map[string]interface{})["t"]).To(Equal("first"))
		})
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
