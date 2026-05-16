package template_test

import (
	"crypto/sha256"
	"crypto/sha512"
	"encoding/base64"
	"encoding/hex"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/zeroedin/alloy/internal/content"
	"github.com/zeroedin/alloy/internal/ordered"
	tmpl "github.com/zeroedin/alloy/internal/template"
)

var _ = Describe("Built-in Filters", func() {

	// ── String filters (table-driven) ──────────────────────────────────

	DescribeTable("String filters",
		func(filterName string, input interface{}, expected interface{}) {
			result := tmpl.ApplyFilter(filterName, input)
			Expect(result).To(Equal(expected))
		},
		Entry("upcase converts to uppercase", "upcase", "hello", "HELLO"),
		Entry("downcase converts to lowercase", "downcase", "HELLO", "hello"),
		Entry("capitalize capitalizes the first letter", "capitalize", "hello world", "Hello world"),
		Entry("slugify converts to URL slug", "slugify", "My First Post", "my-first-post"),
		Entry("strip_html removes HTML tags", "strip_html", "<p>Hello</p>", "Hello"),
		Entry("escape escapes HTML entities", "escape", "<script>", "&lt;script&gt;"),
		Entry("strip removes leading and trailing whitespace", "strip", "  hello  ", "hello"),
	)

	DescribeTable("String filters with one argument",
		func(filterName string, input interface{}, arg interface{}, expected interface{}) {
			result := tmpl.ApplyFilter(filterName, input, arg)
			Expect(result).To(Equal(expected))
		},
		Entry("append appends a string", "append", "hello", " world", "hello world"),
		Entry("prepend prepends a string", "prepend", "world", "hello ", "hello world"),
		Entry("contains checks for substring presence (true)", "contains", "hello world", "world", true),
	)

	DescribeTable("String filters with two arguments",
		func(filterName string, input interface{}, arg1 interface{}, arg2 interface{}, expected interface{}) {
			result := tmpl.ApplyFilter(filterName, input, arg1, arg2)
			Expect(result).To(Equal(expected))
		},
		Entry("replace replaces all occurrences", "replace", "hello world", "world", "go", "hello go"),
		Entry("replace_first replaces only the first occurrence", "replace_first", "aabaa", "a", "b", "babaa"),
	)

	// ── String filters — newline_to_br (ContainSubstring, kept individual) ──

	It("newline_to_br converts newlines to <br>", func() {
		result := tmpl.NewlineToBr("hello\nworld")
		Expect(result.(string)).To(ContainSubstring("<br>"))
	})

	// ── String filters — complex / edge cases (individual) ─────────────

	It("truncate truncates to length with ellipsis", func() {
		result := tmpl.Truncate("Hello World", 5)
		Expect(result).To(Equal("He..."))
	})

	It("truncatewords truncates to word count", func() {
		result := tmpl.TruncateWords("one two three four", 2)
		Expect(result).To(Equal("one two..."))
	})

	It("split splits string into slice", func() {
		result := tmpl.Split("a,b,c", ",")
		Expect(result).To(Equal([]string{"a", "b", "c"}))
	})

	It("join joins slice into string", func() {
		result := tmpl.Join([]interface{}{"a", "b", "c"}, ",")
		Expect(result).To(Equal("a,b,c"))
	})

	It("contains returns false when substring is absent", func() {
		result := tmpl.Contains("hello world", "xyz")
		Expect(result).To(Equal(false),
			"contains must return false when substring is not found")
	})

	// ── Date filters (individual — ContainSubstring) ────────────────────

	It("date formats with strftime-style format string", func() {
		result := tmpl.DateFormat("2024-06-15", "%B %d, %Y")
		s, ok := result.(string)
		Expect(ok).To(BeTrue())
		Expect(s).To(ContainSubstring("June"))
		Expect(s).To(ContainSubstring("2024"))
	})

	// ── Date filter full POSIX compliance (issue #367) ──────────────────
	// DateFormat must support all POSIX strftime directives via
	// lestrrat-go/strftime, not the hand-rolled 13-entry map.

	Context("Date filter POSIX compliance (issue #367)", func() {
		// Fixed reference time: Saturday, June 15, 2024 14:30:45 UTC
		// Using a string input so DateFormat parses it.
		refDate := "2024-06-15T14:30:45Z"

		DescribeTable("POSIX strftime directives",
			func(directive string, expected string) {
				result := tmpl.DateFormat(refDate, directive)
				s, ok := result.(string)
				Expect(ok).To(BeTrue(),
					"DateFormat must return a string for directive "+directive)
				Expect(s).To(Equal(expected),
					"directive "+directive+" must produce correct output — "+
						"if this fails, the directive is not supported by the strftime implementation")
			},
			Entry("%Y — 4-digit year", "%Y", "2024"),
			Entry("%y — 2-digit year", "%y", "24"),
			Entry("%m — zero-padded month", "%m", "06"),
			Entry("%d — zero-padded day", "%d", "15"),
			Entry("%H — 24-hour hour", "%H", "14"),
			Entry("%M — minute", "%M", "30"),
			Entry("%S — second", "%S", "45"),
			Entry("%B — full month name", "%B", "June"),
			Entry("%b — abbreviated month name", "%b", "Jun"),
			Entry("%A — full weekday name", "%A", "Saturday"),
			Entry("%a — abbreviated weekday name", "%a", "Sat"),
			Entry("%p — AM/PM", "%p", "PM"),
			Entry("%I — 12-hour hour", "%I", "02"),
			Entry("%e — space-padded day", "%e", "15"),
			Entry("%j — day of year", "%j", "167"),
			Entry("%C — century", "%C", "20"),
			Entry("%D — mm/dd/yy", "%D", "06/15/24"),
			Entry("%F — YYYY-MM-DD", "%F", "2024-06-15"),
			Entry("%R — HH:MM", "%R", "14:30"),
			Entry("%T — HH:MM:SS", "%T", "14:30:45"),
			Entry("%% — literal percent", "%%", "%"),
			Entry("%n — newline", "%n", "\n"),
			Entry("%t — tab", "%t", "\t"),
		)

		It("format string with no directives passes through unchanged", func() {
			result := tmpl.DateFormat(refDate, "hello world")
			Expect(result).To(Equal("hello world"),
				"format strings without %X directives must pass through unchanged")
		})

		It("format string with multiple directives", func() {
			result := tmpl.DateFormat(refDate, "%Y-%m-%d %H:%M:%S")
			Expect(result).To(Equal("2024-06-15 14:30:45"),
				"multiple directives in a single format string must all resolve")
		})

		It("DateFormat handles time.Time input directly", func() {
			t := time.Date(2024, 6, 15, 14, 30, 45, 0, time.UTC)
			result := tmpl.DateFormat(t, "%Y-%m-%d")
			Expect(result).To(Equal("2024-06-15"),
				"DateFormat must accept time.Time input without string parsing")
		})

		It("DateFormat handles various string date formats", func() {
			// ISO 8601 with timezone
			result := tmpl.DateFormat("2024-06-15T14:30:45Z", "%Y")
			Expect(result).To(Equal("2024"))

			// Date + time without timezone
			result = tmpl.DateFormat("2024-06-15 14:30:45", "%B")
			Expect(result).To(Equal("June"))

			// Date only
			result = tmpl.DateFormat("2024-06-15", "%d")
			Expect(result).To(Equal("15"))
		})

		It("DateFormat with no format argument returns input unchanged", func() {
			result := tmpl.DateFormat("2024-06-15")
			Expect(result).To(Equal("2024-06-15"),
				"no format argument → return input as-is")
		})

		It("DateFormat with unparseable input returns input unchanged", func() {
			result := tmpl.DateFormat("not-a-date", "%Y")
			Expect(result).To(Equal("not-a-date"),
				"unparseable string input must be returned unchanged, not error")
		})
	})

	// ── Array filters (table-driven, simple) ────────────────────────────

	DescribeTable("Array filters (simple)",
		func(filterName string, input interface{}, expected interface{}) {
			result := tmpl.ApplyFilter(filterName, input)
			Expect(result).To(Equal(expected))
		},
		Entry("reverse reverses the array", "reverse", []interface{}{1, 2, 3}, []interface{}{3, 2, 1}),
		Entry("first returns the first element", "first", []interface{}{"a", "b", "c"}, "a"),
		Entry("last returns the last element", "last", []interface{}{"a", "b", "c"}, "c"),
		Entry("size returns the length", "size", []interface{}{1, 2, 3, 4, 5}, 5),
	)

	// uniq and compact use HaveLen, so they stay individual
	It("uniq removes duplicate elements", func() {
		input := []interface{}{1, 2, 2, 3, 3, 3}
		result := tmpl.Uniq(input)
		Expect(result.([]interface{})).To(HaveLen(3))
	})

	It("compact removes nil elements", func() {
		input := []interface{}{"a", nil, "b", nil, "c"}
		result := tmpl.Compact(input)
		Expect(result.([]interface{})).To(HaveLen(3))
	})

	// ── Array filters — complex (individual) ────────────────────────────

	It("sort sorts elements by key", func() {
		input := []interface{}{
			map[string]interface{}{"name": "cherry"},
			map[string]interface{}{"name": "apple"},
			map[string]interface{}{"name": "banana"},
		}
		result := tmpl.Sort(input, "name")
		sorted := result.([]interface{})
		Expect(sorted[0].(map[string]interface{})["name"]).To(Equal("apple"))
	})

	It("where filters by property value", func() {
		input := []interface{}{
			map[string]interface{}{"name": "a", "active": true},
			map[string]interface{}{"name": "b", "active": false},
			map[string]interface{}{"name": "c", "active": true},
		}
		result := tmpl.Where(input, "active", true)
		filtered := result.([]interface{})
		Expect(filtered).To(HaveLen(2))
	})

	It("map extracts a property from each element", func() {
		input := []interface{}{
			map[string]interface{}{"name": "Alice", "age": 30},
			map[string]interface{}{"name": "Bob", "age": 25},
			map[string]interface{}{"name": "Carol", "age": 35},
		}
		result := tmpl.Map(input, "name")
		Expect(result).To(Equal([]interface{}{"Alice", "Bob", "Carol"}),
			"map must extract the named property from each element")
	})

	It("concat joins two arrays into one", func() {
		a := []interface{}{1, 2, 3}
		b := []interface{}{4, 5, 6}
		result := tmpl.Concat(a, b)
		Expect(result).To(Equal([]interface{}{1, 2, 3, 4, 5, 6}),
			"concat must append second array to first")
	})

	It("group_by groups elements by a property value", func() {
		input := []interface{}{
			map[string]interface{}{"name": "Alice", "dept": "eng"},
			map[string]interface{}{"name": "Bob", "dept": "sales"},
			map[string]interface{}{"name": "Carol", "dept": "eng"},
		}
		result := tmpl.GroupBy(input, "dept")

		grouped, ok := result.(map[string]interface{})
		Expect(ok).To(BeTrue(), "group_by must return a map keyed by property value")
		Expect(grouped).To(HaveKey("eng"))
		Expect(grouped).To(HaveKey("sales"))

		engGroup := grouped["eng"].([]interface{})
		Expect(engGroup).To(HaveLen(2),
			"eng group must contain Alice and Carol")
	})

	// ── Set operation filters (individual — two array inputs) ───────────

	It("intersect returns common elements", func() {
		a := []interface{}{1, 2, 3, 4}
		b := []interface{}{3, 4, 5, 6}
		result := tmpl.Intersect(a, b)
		Expect(result.([]interface{})).To(ConsistOf(3, 4))
	})

	It("union returns combined unique elements", func() {
		a := []interface{}{1, 2, 3}
		b := []interface{}{3, 4, 5}
		result := tmpl.Union(a, b)
		Expect(result.([]interface{})).To(ConsistOf(1, 2, 3, 4, 5))
	})

	It("complement returns elements in first but not second", func() {
		a := []interface{}{1, 2, 3, 4}
		b := []interface{}{3, 4, 5}
		result := tmpl.Complement(a, b)
		Expect(result.([]interface{})).To(ConsistOf(1, 2))
	})

	// ── URL filters (table-driven) ──────────────────────────────────────

	DescribeTable("URL filters",
		func(filterName string, input interface{}, expected interface{}) {
			result := tmpl.ApplyFilter(filterName, input)
			Expect(result).To(Equal(expected))
		},
		Entry("url_decode decodes percent-encoded strings", "url_decode", "hello%20world", "hello world"),
	)

	// url_encode kept individual due to dual-acceptable output
	It("url_encode encodes spaces", func() {
		result := tmpl.URLEncode("hello world")
		s := result.(string)
		Expect(s == "hello%20world" || s == "hello+world").To(BeTrue())
	})

	// ── URL filters — absolute_url (individual — two args, special assertions) ──

	It("absolute_url prepends baseURL to a relative path", func() {
		result := tmpl.AbsoluteURL("/blog/my-post/", "https://example.com")
		Expect(result).To(Equal("https://example.com/blog/my-post/"),
			"absolute_url must prepend baseURL to relative path")
	})

	It("absolute_url returns absolute URLs unchanged", func() {
		result := tmpl.AbsoluteURL("https://other.com/page", "https://example.com")
		Expect(result).To(Equal("https://other.com/page"),
			"absolute_url must not modify already-absolute URLs")
	})

	// ── URL filter (relative path resolution) ──────────────────────────

	It("url filter resolves relative asset path", func() {
		result := tmpl.URLFilter("/css/main.css", "https://example.com")
		Expect(result).To(Equal("/css/main.css"),
			"url filter must return the relative path for template references")
	})

	It("url filter handles paths without leading slash", func() {
		result := tmpl.URLFilter("css/main.css", "https://example.com")
		s := result.(string)
		Expect(s).To(HavePrefix("/"),
			"url filter must ensure paths start with /")
	})

	// ── Math filters (table-driven) ─────────────────────────────────────

	DescribeTable("Math filters",
		func(filterName string, a interface{}, b interface{}, expected interface{}) {
			result := tmpl.ApplyFilter(filterName, a, b)
			Expect(result).To(Equal(expected))
		},
		Entry("plus adds two numbers", "plus", 3, 2, 5),
		Entry("minus subtracts two numbers", "minus", 5, 2, 3),
		Entry("times multiplies two numbers", "times", 3, 4, 12),
		Entry("divided_by divides two numbers", "divided_by", 10, 2, 5),
		Entry("modulo returns the remainder", "modulo", 10, 3, 1),
	)

	DescribeTable("Math filters (unary)",
		func(filterName string, input interface{}, expected interface{}) {
			result := tmpl.ApplyFilter(filterName, input)
			Expect(result).To(Equal(expected))
		},
		Entry("ceil rounds up", "ceil", 4.1, 5),
		Entry("floor rounds down", "floor", 4.9, 4),
		Entry("round rounds to nearest integer", "round", 4.5, 5),
		Entry("abs returns absolute value", "abs", -5, 5),
	)

	// ── Math filter edge cases (individual) ─────────────────────────────

	It("divided_by with zero returns error or infinity indicator", func() {
		Expect(func() {
			_ = tmpl.DividedBy(10, 0)
		}).NotTo(Panic(), "division by zero must not panic")

		normalResult := tmpl.DividedBy(10, 2)
		Expect(normalResult).To(Equal(5),
			"guard: normal division must produce correct result")
	})

	It("minus with negative numbers produces correct result", func() {
		result := tmpl.Minus(-3, -5)
		Expect(result).To(Equal(2),
			"(-3) - (-5) must equal 2")
	})

	It("modulo with negative dividend", func() {
		result := tmpl.Modulo(-10, 3)
		Expect(result).To(Equal(-1),
			"-10 mod 3 must follow Go semantics")
	})

	// ── Content filters (individual — ContainSubstring) ─────────────────

	It("markdownify converts markdown to HTML", func() {
		result := tmpl.Markdownify("**bold**")
		Expect(result.(string)).To(ContainSubstring("<strong>"))
	})

	// ── markdownify shared goldmark config (issue #366) ──────────

	It("markdownify renders tables", func() {
		input := "| A | B |\n|---|---|\n| 1 | 2 |"
		result := tmpl.Markdownify(input)
		html := result.(string)
		Expect(html).To(ContainSubstring("<table>"),
			"markdownify must support tables — uses same goldmark extensions as main renderer")
		Expect(html).To(ContainSubstring("<td>1</td>"),
			"table cells must render correctly")
	})

	It("markdownify generates heading IDs", func() {
		result := tmpl.Markdownify("## Getting Started")
		html := result.(string)
		Expect(html).To(ContainSubstring(`id="getting-started"`),
			"markdownify must generate auto heading IDs — "+
				"uses same parser options as main renderer")
	})

	It("markdownify respects heading attributes", func() {
		result := tmpl.Markdownify("## My Section {#custom-id}")
		html := result.(string)
		Expect(html).To(ContainSubstring(`id="custom-id"`),
			"markdownify must support {#custom-id} heading attributes")
	})

	It("markdownify produces consistent output across multiple calls", func() {
		result1 := tmpl.Markdownify("**first**").(string)
		result2 := tmpl.Markdownify("**second**").(string)
		result3 := tmpl.Markdownify("**first**").(string)
		Expect(result1).To(Equal(result3),
			"markdownify must produce identical output for identical input — "+
				"proves shared instance has no mutable state between calls")
		Expect(result2).To(ContainSubstring("<strong>second</strong>"),
			"each call must render independently")
	})

	It("markdownify and RenderMarkdown use the same config-driven goldmark options", func() {
		input := "## Heading\n\n| A | B |\n|---|---|\n| 1 | 2 |"
		markdownifyResult := tmpl.Markdownify(input).(string)
		// TemplateTags: false — markdownify doesn't run template tag
		// protection (it processes already-rendered values).
		opts := content.MarkdownOptions{
			Unsafe:        true,
			Typographer:   true,
			TemplateTags:  false,
			AutoHeadingID: true,
		}
		renderResult, _, err := content.RenderMarkdown([]byte(input), content.CreateGoldmark(opts))
		Expect(err).NotTo(HaveOccurred())
		Expect(markdownifyResult).To(Equal(string(renderResult)),
			"markdownify and RenderMarkdown must use the same config-driven "+
				"goldmark options — if they diverge, a table or heading ID works "+
				"in the page body but not in {{ page.description | markdownify }}")
	})

	// ── Regex filters (individual — complex) ────────────────────────────

	It("findRE returns matching substrings", func() {
		result := tmpl.FindRE(`\d+`, "abc123def456")
		matches := result.([]string)
		Expect(matches).To(ConsistOf("123", "456"))
	})

	It("replaceRE replaces pattern matches", func() {
		result := tmpl.ReplaceRE(`\d+`, "abc123def456", "NUM")
		Expect(result).To(Equal("abcNUMdefNUM"))
	})

	// ── Data filters (individual — special cases) ───────────────────────

	It("json converts map to JSON string", func() {
		input := map[string]interface{}{"key": "value"}
		result := tmpl.JSONFilter(input)
		s := result.(string)
		Expect(s).To(ContainSubstring(`"key"`))
		Expect(s).To(ContainSubstring(`"value"`))
	})

	It("default returns fallback when input is nil", func() {
		result := tmpl.Default(nil, "fallback")
		Expect(result).To(Equal("fallback"))
	})

	It("default returns input when present", func() {
		result := tmpl.Default("present", "fallback")
		Expect(result).To(Equal("present"))
	})

	// ── Asset filters (individual — length assertion) ───────────────────

	It("fingerprint appends hash to asset path", func() {
		result := tmpl.Fingerprint("css/main.css")
		s := result.(string)
		Expect(len(s)).To(BeNumerically(">", len("css/main.css")))
	})

	// ── Output safety filters (individual — type assertion) ─────────────

	It("safeHTML marks content as safe for Go template rendering", func() {
		result := tmpl.SafeHTML("<p>Trusted content</p>")
		s, ok := result.(string)
		Expect(ok).To(BeTrue())
		Expect(s).To(Equal("<p>Trusted content</p>"),
			"safeHTML must return the input unchanged (not escaped)")
	})

	// ── String filter edge cases (individual) ───────────────────────────

	It("slugify handles empty string", func() {
		result := tmpl.Slugify("")
		Expect(result).To(Equal(""),
			"slugify of empty string must return empty string")
	})

	It("upcase handles nil input gracefully", func() {
		Expect(func() {
			_ = tmpl.Upcase(nil)
		}).NotTo(Panic(), "upcase with nil input must not panic")

		normalResult := tmpl.Upcase("hello")
		Expect(normalResult).To(Equal("HELLO"),
			"guard: upcase must work for normal input")
	})

	It("truncate with length longer than input returns full string", func() {
		result := tmpl.Truncate("hi", 100)
		Expect(result).To(Equal("hi"),
			"truncate with length > input must return full string")
	})

	// ── sort numeric awareness (issue #348) ──────────────────────
	// sort must compare whole numbers numerically, not lexicographically.

	It("sort by key compares integers numerically", func() {
		input := []interface{}{
			map[string]interface{}{"title": "C", "order": 10},
			map[string]interface{}{"title": "A", "order": 1},
			map[string]interface{}{"title": "B", "order": 2},
			map[string]interface{}{"title": "D", "order": 20},
		}
		result := tmpl.Sort(input, "order")
		arr := result.([]interface{})
		Expect(arr[0].(map[string]interface{})["title"]).To(Equal("A"))
		Expect(arr[1].(map[string]interface{})["title"]).To(Equal("B"))
		Expect(arr[2].(map[string]interface{})["title"]).To(Equal("C"))
		Expect(arr[3].(map[string]interface{})["title"]).To(Equal("D"),
			"sort must compare integers numerically: 1, 2, 10, 20 — "+
				"not lexicographically: 1, 10, 2, 20")
	})

	It("sort by key compares string digits numerically", func() {
		input := []interface{}{
			map[string]interface{}{"title": "C", "order": "10"},
			map[string]interface{}{"title": "A", "order": "1"},
			map[string]interface{}{"title": "B", "order": "2"},
		}
		result := tmpl.Sort(input, "order")
		arr := result.([]interface{})
		Expect(arr[0].(map[string]interface{})["title"]).To(Equal("A"))
		Expect(arr[1].(map[string]interface{})["title"]).To(Equal("B"))
		Expect(arr[2].(map[string]interface{})["title"]).To(Equal("C"),
			"sort must parse digit-only strings as numbers: \"1\", \"2\", \"10\"")
	})

	It("sort by key handles mixed int and string-digit types", func() {
		input := []interface{}{
			map[string]interface{}{"title": "C", "order": "10"},
			map[string]interface{}{"title": "A", "order": 1},
			map[string]interface{}{"title": "B", "order": "2"},
		}
		result := tmpl.Sort(input, "order")
		arr := result.([]interface{})
		Expect(arr[0].(map[string]interface{})["title"]).To(Equal("A"))
		Expect(arr[1].(map[string]interface{})["title"]).To(Equal("B"))
		Expect(arr[2].(map[string]interface{})["title"]).To(Equal("C"),
			"sort must handle mixed int and string-digit values — "+
				"YAML parses bare 1 as int but quoted \"2\" as string")
	})

	It("sort by key falls back to string for non-numeric values", func() {
		input := []interface{}{
			map[string]interface{}{"title": "Banana"},
			map[string]interface{}{"title": "Apple"},
			map[string]interface{}{"title": "Cherry"},
		}
		result := tmpl.Sort(input, "title")
		arr := result.([]interface{})
		Expect(arr[0].(map[string]interface{})["title"]).To(Equal("Apple"))
		Expect(arr[1].(map[string]interface{})["title"]).To(Equal("Banana"))
		Expect(arr[2].(map[string]interface{})["title"]).To(Equal("Cherry"),
			"non-numeric values must sort as strings (alphabetical)")
	})

	It("sort puts nil/missing values at the end", func() {
		input := []interface{}{
			map[string]interface{}{"title": "B", "order": 2},
			map[string]interface{}{"title": "No Order"},
			map[string]interface{}{"title": "A", "order": 1},
		}
		result := tmpl.Sort(input, "order")
		arr := result.([]interface{})
		Expect(arr[0].(map[string]interface{})["title"]).To(Equal("A"))
		Expect(arr[1].(map[string]interface{})["title"]).To(Equal("B"))
		Expect(arr[2].(map[string]interface{})["title"]).To(Equal("No Order"),
			"items without the sort key must sort to the end")
	})

	// ── ApplyFilter and IsBuiltinFilter dispatch (issue #363) ────────
	// These tests verify all built-in filters are registered and
	// discoverable. The hardcoded list IS the spec — it defines which
	// filters must exist. When a new filter is added to
	// RegisterBuiltinFilters, it must also be added here.

	Context("ApplyFilter dispatch completeness (issue #363)", func() {
		// Canonical list of all built-in filters. This is the spec —
		// every name here must be registered in both ApplyFilter and
		// IsBuiltinFilter. If a new filter is added to the codebase,
		// add it here too.
		allFilterNames := []string{
			"slugify", "upcase", "downcase", "capitalize",
			"truncate", "truncatewords", "strip_html", "escape",
			"replace", "replace_first", "split", "join",
			"strip", "append", "prepend", "newline_to_br",
			"contains", "date", "sort", "reverse",
			"first", "last", "where", "group_by",
			"size", "map", "uniq", "compact", "concat",
			"intersect", "union", "complement",
			"url", "absolute_url", "url_encode", "url_decode",
			"plus", "minus", "times", "divided_by",
			"modulo", "ceil", "floor", "round", "abs",
			"markdownify", "findRE", "replaceRE",
			"json", "default", "fingerprint", "safeHTML",
			"flatten",
			"cachebust", "get_hash", // issue #559: asset fingerprinting
		}

		It("IsBuiltinFilter returns true for all registered filters", func() {
			for _, name := range allFilterNames {
				Expect(tmpl.IsBuiltinFilter(name)).To(BeTrue(),
					"IsBuiltinFilter must return true for %q — "+
						"if missing after promoting to package-level map, "+
						"the filter was dropped from the map", name)
			}
		})

		It("IsBuiltinFilter returns false for unknown filters", func() {
			Expect(tmpl.IsBuiltinFilter("nonexistent_filter")).To(BeFalse(),
				"IsBuiltinFilter must return false for unknown filters")
			Expect(tmpl.IsBuiltinFilter("")).To(BeFalse(),
				"IsBuiltinFilter must return false for empty string")
		})

		It("ApplyFilter returns non-nil for string-producing filters", func() {
			// Filters that should produce a non-nil result given string input "test".
			// This proves the filter function is registered and reachable.
			stringFilters := []string{
				"slugify", "upcase", "downcase", "capitalize",
				"strip_html", "escape", "strip", "newline_to_br",
				"url_encode", "url_decode", "json", "safeHTML",
			}
			for _, name := range stringFilters {
				result := tmpl.ApplyFilter(name, "test")
				Expect(result).NotTo(BeNil(),
					"ApplyFilter(%q, \"test\") must return non-nil — "+
						"if nil, the filter is missing from the dispatch map", name)
			}
		})

		It("ApplyFilter returns nil for unknown filter names", func() {
			result := tmpl.ApplyFilter("nonexistent_filter", "test")
			Expect(result).To(BeNil(),
				"ApplyFilter must return nil for unknown filter names")
		})
	})

	// ── Filters on *ordered.Map items (issue #477) ──────────────────
	// where, sort, group_by, and map must work on arrays of *ordered.Map
	// items from JSON data files. getMapValue must handle *ordered.Map.

	Context("Filters on ordered.Map items (issue #477)", func() {
		// Helper: create an array of *ordered.Map from JSON
		parseJSONArray := func(jsonStr string) []interface{} {
			var arr []interface{}
			result, err := ordered.UnmarshalJSONValue([]byte(jsonStr))
			Expect(err).NotTo(HaveOccurred())
			arr, ok := result.([]interface{})
			Expect(ok).To(BeTrue())
			return arr
		}

		It("where filters ordered.Map items by key=value", func() {
			items := parseJSONArray(`[
				{"tagName":"rh-accordion","kind":"class"},
				{"tagName":"rh-button","kind":"class"},
				{"tagName":"rh-tooltip","kind":"class"}
			]`)

			result := tmpl.Where(items, "tagName", "rh-button")
			arr, ok := result.([]interface{})
			Expect(ok).To(BeTrue())
			Expect(arr).To(HaveLen(1),
				"where must find exactly one match — "+
					"if empty, getMapValue doesn't handle *ordered.Map (issue #477)")

			match, ok := arr[0].(*ordered.Map)
			Expect(ok).To(BeTrue())
			Expect(match.Get("tagName")).To(Equal("rh-button"))
		})

		It("sort orders ordered.Map items by key", func() {
			items := parseJSONArray(`[
				{"name":"Charlie","order":3},
				{"name":"Alice","order":1},
				{"name":"Bob","order":2}
			]`)

			result := tmpl.Sort(items, "name")
			arr, ok := result.([]interface{})
			Expect(ok).To(BeTrue())
			Expect(arr).To(HaveLen(3))

			first, ok := arr[0].(*ordered.Map)
			Expect(ok).To(BeTrue())
			Expect(first.Get("name")).To(Equal("Alice"),
				"sort by name must put Alice first — "+
					"if order is wrong, getMapValue returns nil for *ordered.Map")
		})

		It("groupby groups ordered.Map items by key", func() {
			items := parseJSONArray(`[
				{"name":"Alice","role":"engineer"},
				{"name":"Bob","role":"designer"},
				{"name":"Charlie","role":"engineer"}
			]`)

			result := tmpl.GroupBy(items, "role")
			groups, ok := result.(map[string]interface{})
			Expect(ok).To(BeTrue())
			Expect(groups).To(HaveKey("engineer"),
				"groupby must group by role — "+
					"if no keys, getMapValue returns nil for *ordered.Map")
			Expect(groups).To(HaveKey("designer"))

			engineers, ok := groups["engineer"].([]interface{})
			Expect(ok).To(BeTrue())
			Expect(engineers).To(HaveLen(2))
		})

		It("map plucks field from ordered.Map items", func() {
			items := parseJSONArray(`[
				{"name":"Alice","email":"alice@example.com"},
				{"name":"Bob","email":"bob@example.com"}
			]`)

			result := tmpl.Map(items, "name")
			arr, ok := result.([]interface{})
			Expect(ok).To(BeTrue())
			Expect(arr).To(Equal([]interface{}{"Alice", "Bob"}),
				"map must extract name from each *ordered.Map item — "+
					"if nil values, getMapValue doesn't handle *ordered.Map")
		})

		It("where with nested ordered.Map values", func() {
			// Simulates CEM structure: modules[].declarations[]
			input := `[
				{"path":"accordion.js","declarations":[
					{"tagName":"rh-accordion","slots":[{"name":"default"},{"name":"header"}]},
					{"tagName":"rh-accordion-header","slots":[]}
				]},
				{"path":"button.js","declarations":[
					{"tagName":"rh-button","slots":[{"name":"default"}]}
				]}
			]`
			modules := parseJSONArray(input)

			// Walk modules, find declaration by tagName
			var foundSlots []interface{}
			for _, mod := range modules {
				om, ok := mod.(*ordered.Map)
				if !ok {
					continue
				}
				decls, ok := om.Get("declarations").([]interface{})
				if !ok {
					continue
				}
				matches := tmpl.Where(decls, "tagName", "rh-accordion")
				if arr, ok := matches.([]interface{}); ok && len(arr) > 0 {
					decl := arr[0].(*ordered.Map)
					foundSlots, _ = decl.Get("slots").([]interface{})
					break
				}
			}

			Expect(foundSlots).To(HaveLen(2),
				"must find 2 slots for rh-accordion through nested where — "+
					"this simulates the CEM use case from issue #477")
		})
	})

	// ── Asset fingerprinting filters (issue #559) ──────────────────
	// cachebust and get_hash resolve file paths against source directories
	// (static → assets → content), read file contents, and compute hashes.
	// Fixture: testdata/static/css/main.css contains "body{color:red}".
	// Implementation must register these filters via RegisterAssetFilters
	// (or equivalent) configured to resolve against testdata/ in tests.

	Context("cachebust filter (issue #559)", func() {
		// Known fixture content — must match testdata/static/css/main.css exactly.
		knownContent := []byte("body{color:red}")

		It("returns /<path>?h=<hex12> for an existing file", func() {
			h := sha256.Sum256(knownContent)
			expectedHex12 := hex.EncodeToString(h[:])[:12]

			result := tmpl.ApplyFilter("cachebust", "css/main.css")
			Expect(result).NotTo(BeNil(),
				"cachebust must be registered as a built-in filter — "+
					"ApplyFilter returns nil when the filter is missing from builtinFilters (issue #559)")
			Expect(result).To(Equal("/css/main.css?h="+expectedHex12),
				"cachebust must return /<path>?h=<hex12> where hex12 is the first 12 hex "+
					"chars of the SHA-256 of the file at testdata/static/css/main.css (issue #559)")
		})

		It("returns /<path> without hash when file is not found", func() {
			result := tmpl.ApplyFilter("cachebust", "nonexistent/file.css")
			Expect(result).NotTo(BeNil(),
				"cachebust must be registered — even for missing files it returns "+
					"a value (graceful degradation, not nil) (issue #559)")
			Expect(result).To(Equal("/nonexistent/file.css"),
				"cachebust must prepend / and return path without hash query when the "+
					"file cannot be found in any source directory (issue #559)")
		})
	})

	Context("get_hash filter (issue #559)", func() {
		knownContent := []byte("body{color:red}")

		It("returns SHA-256 base64 digest by default", func() {
			h := sha256.Sum256(knownContent)
			expectedB64 := base64.StdEncoding.EncodeToString(h[:])

			result := tmpl.ApplyFilter("get_hash", "css/main.css")
			Expect(result).NotTo(BeNil(),
				"get_hash must be registered as a built-in filter (issue #559)")
			Expect(result).To(Equal(expectedB64),
				"get_hash with no args must return SHA-256 base64 digest of file "+
					"contents at testdata/static/css/main.css (issue #559)")
		})

		It("returns SHA-384 hex digest with positional args 384, false", func() {
			h := sha512.Sum384(knownContent)
			expectedHex := hex.EncodeToString(h[:])

			result := tmpl.ApplyFilter("get_hash", "css/main.css", 384, false)
			Expect(result).NotTo(BeNil(),
				"get_hash must be registered as a built-in filter (issue #559)")
			Expect(result).To(Equal(expectedHex),
				"get_hash with sha_type=384 and base64=false must return SHA-384 hex "+
					"digest of file contents at testdata/static/css/main.css (issue #559)")
		})

		It("returns empty string when file is not found", func() {
			result := tmpl.ApplyFilter("get_hash", "nonexistent/file.css")
			Expect(result).NotTo(BeNil(),
				"get_hash must be registered — even for missing files it returns "+
					"a value (empty string, not nil) (issue #559)")
			Expect(result).To(Equal(""),
				"get_hash must return empty string when the file cannot be found "+
					"in any source directory (issue #559)")
		})
	})

	// ── Flatten filter (issue #477) ─────────────────────────────────
	// Collapses one level of array nesting. Required for CEM use case
	// where map: "declarations" produces [[decl1, decl2], [decl3]].

	Context("Flatten filter (issue #477)", func() {
		It("collapses one level of array nesting", func() {
			input := []interface{}{
				[]interface{}{"a", "b"},
				[]interface{}{"c", "d"},
			}
			result := tmpl.Flatten(input)
			arr, ok := result.([]interface{})
			Expect(ok).To(BeTrue())
			Expect(arr).To(Equal([]interface{}{"a", "b", "c", "d"}),
				"flatten must collapse [[a,b],[c,d]] into [a,b,c,d]")
		})

		It("flat array is unchanged", func() {
			input := []interface{}{"a", "b", "c"}
			result := tmpl.Flatten(input)
			arr, ok := result.([]interface{})
			Expect(ok).To(BeTrue())
			Expect(arr).To(Equal([]interface{}{"a", "b", "c"}),
				"flatten on a flat array must be a no-op")
		})

		It("nil input returns nil", func() {
			result := tmpl.Flatten(nil)
			Expect(result).To(BeNil())
		})

		It("mixed nested and flat items", func() {
			input := []interface{}{
				[]interface{}{"a", "b"},
				"c",
				[]interface{}{"d"},
			}
			result := tmpl.Flatten(input)
			arr, ok := result.([]interface{})
			Expect(ok).To(BeTrue())
			Expect(arr).To(Equal([]interface{}{"a", "b", "c", "d"}),
				"flatten must handle mixed nested and flat items")
		})

		It("CEM pipeline: map declarations then flatten then where", func() {
			parseJSONArray := func(jsonStr string) []interface{} {
				result, err := ordered.UnmarshalJSONValue([]byte(jsonStr))
				Expect(err).NotTo(HaveOccurred())
				arr, ok := result.([]interface{})
				Expect(ok).To(BeTrue())
				return arr
			}

			modules := parseJSONArray(`[
				{"path":"accordion.js","declarations":[
					{"tagName":"rh-accordion","slots":[{"name":"default"},{"name":"header"}]},
					{"tagName":"rh-accordion-header","slots":[]}
				]},
				{"path":"button.js","declarations":[
					{"tagName":"rh-button","slots":[{"name":"default"}]}
				]}
			]`)

			// Step 1: map "declarations" — produces array of arrays
			mapped := tmpl.Map(modules, "declarations")

			// Step 2: flatten — collapses to flat array of declarations
			flattened := tmpl.Flatten(mapped)
			flatArr, ok := flattened.([]interface{})
			Expect(ok).To(BeTrue())
			Expect(flatArr).To(HaveLen(3),
				"flatten must produce 3 declarations total (2 + 1)")

			// Step 3: where tagName — find the one we want
			matches := tmpl.Where(flatArr, "tagName", "rh-accordion")
			matchArr, ok := matches.([]interface{})
			Expect(ok).To(BeTrue())
			Expect(matchArr).To(HaveLen(1),
				"where must find exactly one rh-accordion declaration — "+
					"this is the full CEM pipeline: map | flatten | where")

			// Step 4: access nested slots
			decl, ok := matchArr[0].(*ordered.Map)
			Expect(ok).To(BeTrue())
			slots, ok := decl.Get("slots").([]interface{})
			Expect(ok).To(BeTrue())
			Expect(slots).To(HaveLen(2),
				"rh-accordion must have 2 slots (default, header)")
		})
	})
})
