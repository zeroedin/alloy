package template_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/zeroedin/alloy/internal/content"
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

	It("markdownify and RenderMarkdown produce identical output", func() {
		input := "## Heading\n\n| A | B |\n|---|---|\n| 1 | 2 |"
		markdownifyResult := tmpl.Markdownify(input).(string)
		opts := content.MarkdownOptions{
			Unsafe:        true,
			Typographer:   true,
			TemplateTags:  true,
			AutoHeadingID: true,
		}
		renderResult, _ := content.RenderMarkdown([]byte(input), opts)
		Expect(markdownifyResult).To(Equal(string(renderResult)),
			"markdownify and RenderMarkdown must produce identical output — "+
				"divergent output means different goldmark extensions or parser options")
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
})
