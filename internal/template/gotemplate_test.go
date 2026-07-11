package template_test

import (
	"fmt"
	"sync"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	tmpl "github.com/zeroedin/alloy/internal/template"
)

var _ = Describe("GoEngine", func() {
	var engine tmpl.TemplateEngine

	BeforeEach(func() {
		engine = tmpl.NewGoEngine()
	})

	Describe("Parse and Render", func() {
		It("renders {{ .page.title }} expressions", func() {
			tpl, err := engine.Parse("title", []byte(`<h1>{{ .page.title }}</h1>`))
			Expect(err).NotTo(HaveOccurred())
			Expect(tpl).NotTo(BeNil())

			result, err := tpl.Render(map[string]interface{}{
				"page": map[string]interface{}{
					"title": "Hello World",
				},
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(string(result)).To(Equal("<h1>Hello World</h1>"))
		})

		It("renders {{ if }} conditionals", func() {
			tpl, err := engine.Parse("cond", []byte(`{{ if .show }}visible{{ else }}hidden{{ end }}`))
			Expect(err).NotTo(HaveOccurred())
			Expect(tpl).NotTo(BeNil())

			result, err := tpl.Render(map[string]interface{}{"show": true})
			Expect(err).NotTo(HaveOccurred())
			Expect(string(result)).To(Equal("visible"))

			result, err = tpl.Render(map[string]interface{}{"show": false})
			Expect(err).NotTo(HaveOccurred())
			Expect(string(result)).To(Equal("hidden"))
		})

		It("renders {{ range }} loops", func() {
			tpl, err := engine.Parse("loop", []byte(`{{ range .items }}{{ . }} {{ end }}`))
			Expect(err).NotTo(HaveOccurred())
			Expect(tpl).NotTo(BeNil())

			result, err := tpl.Render(map[string]interface{}{
				"items": []string{"a", "b", "c"},
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(string(result)).To(Equal("a b c "))
		})

		It("renders {{ .content }} in layouts", func() {
			layoutSrc := `<html><body>{{ .content }}</body></html>`
			tpl, err := engine.Parse("layout", []byte(layoutSrc))
			Expect(err).NotTo(HaveOccurred())
			Expect(tpl).NotTo(BeNil())

			result, err := tpl.Render(map[string]interface{}{
				"content": "<p>Page body</p>",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(string(result)).To(ContainSubstring("<p>Page body</p>"))
		})

		It("returns parse error for invalid Go template syntax", func() {
			_, err := engine.Parse("bad", []byte(`{{ if }}`))
			Expect(err).To(HaveOccurred())
			// The error must describe the syntax problem, not be a generic stub error
			Expect(err.Error()).To(
				SatisfyAny(
					ContainSubstring("syntax"),
					ContainSubstring("parse"),
					ContainSubstring("template"),
					ContainSubstring("unexpected"),
				),
				"error should indicate a Go template syntax or parse failure",
			)
		})
	})

	// ── Includes / partials (issue #823, #883) ───────────────────────
	// The Go template engine must support an `include` function that
	// resolves and renders partial templates from the layouts directory.
	// Named `include` for parity with Liquid's {% include %} (issue #883).
	//
	// The developer must:
	// 1. Add a SetIncludesDir method to goEngine (like liquidEngine has)
	// 2. Register an "include" FuncMap function that:
	//    - Takes a path string and optional context argument
	//    - Resolves from the layouts directory (path + ".html", then raw path)
	//    - Reads and parses the file as a Go template (with the engine's FuncMap)
	//    - Renders with the provided context (or current dot if no context given)
	//    - Returns the rendered output as template.HTML (safe, unescaped)
	// 3. Track nesting depth, error at 100 ("Nesting too deep")
	// 4. Guard against path traversal outside the layouts root

	Describe("Includes (issue #823, #883)", func() {
		var includeEngine tmpl.TemplateEngine

		BeforeEach(func() {
			includeEngine = tmpl.NewGoEngine()
			if setter, ok := includeEngine.(interface{ SetIncludesDir(string) }); ok {
				setter.SetIncludesDir("testdata/layouts")
			}
		})

		It("renders a partial with {{ include \"path\" }}", func() {
			tpl, err := includeEngine.Parse("layout", []byte(
				`<html>{{ include "partials/header" }}<body>{{ .content }}</body></html>`))
			Expect(err).NotTo(HaveOccurred())

			result, err := tpl.Render(map[string]interface{}{
				"content": "<p>Body</p>",
				"site":    map[string]interface{}{"title": "My Site"},
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(string(result)).To(ContainSubstring("<header>My Site</header>"),
				"include must render the partial template with the current context — "+
					"this is the core use case replacing {{ template }} for cross-file "+
					"includes (issue #823)")
		})

		It("uses current context when no argument is given", func() {
			tpl, err := includeEngine.Parse("layout", []byte(
				`{{ include "partials/header" }}`))
			Expect(err).NotTo(HaveOccurred())

			result, err := tpl.Render(map[string]interface{}{
				"site": map[string]interface{}{"title": "Implicit Context"},
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(string(result)).To(ContainSubstring("<header>Implicit Context</header>"),
				"include with no context argument must inherit the current dot — "+
					"symmetric with Liquid's {% include %} which shares scope")
		})

		It("accepts explicit context argument", func() {
			tpl, err := includeEngine.Parse("layout", []byte(
				`{{ include "partials/greeting" (dict "name" "Alice") }}`))
			Expect(err).NotTo(HaveOccurred())

			result, err := tpl.Render(map[string]interface{}{})
			Expect(err).NotTo(HaveOccurred())
			Expect(string(result)).To(ContainSubstring("Hello, Alice!"),
				"include must render with the explicitly passed context map — "+
					"this enables narrowing context for component-like partials")
		})

		It("returns output usable in variable assignment", func() {
			tpl, err := includeEngine.Parse("layout", []byte(
				`{{ $h := include "partials/header" }}{{ if $h }}GOT:{{ $h }}{{ end }}`))
			Expect(err).NotTo(HaveOccurred())

			result, err := tpl.Render(map[string]interface{}{
				"site": map[string]interface{}{"title": "Captured"},
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(string(result)).To(ContainSubstring("GOT:<header>Captured</header>"),
				"include must be a function (not an action) so its output can be "+
					"captured in a variable — unlike {{ template }}, which writes "+
					"directly to the output stream")
		})

		It("renders nested includes", func() {
			tpl, err := includeEngine.Parse("layout", []byte(
				`{{ include "partials/nav" }}`))
			Expect(err).NotTo(HaveOccurred())

			result, err := tpl.Render(map[string]interface{}{})
			Expect(err).NotTo(HaveOccurred())
			Expect(string(result)).To(ContainSubstring("<nav><a href=\"/\">Home</a></nav>"),
				"includes must be able to call other includes — "+
					"nav.html contains {{ include \"partials/nav-links\" }}")
		})

		It("errors on circular inclusion", func() {
			tpl, err := includeEngine.Parse("layout", []byte(
				`{{ include "partials/circular-a" }}`))
			Expect(err).NotTo(HaveOccurred())

			_, err = tpl.Render(map[string]interface{}{})
			Expect(err).To(HaveOccurred(),
				"circular includes must produce a build error, not infinite recursion")
			Expect(err.Error()).To(SatisfyAny(
				ContainSubstring("too deep"),
				ContainSubstring("nesting"),
				ContainSubstring("depth"),
			),
				"error message must indicate nesting depth exceeded — "+
					"matches Ruby/Go Liquid's StackLevelError behavior (max depth 100)")
		})

		It("errors on missing include", func() {
			tpl, err := includeEngine.Parse("layout", []byte(
				`{{ include "partials/nonexistent" }}`))
			Expect(err).NotTo(HaveOccurred())

			_, err = tpl.Render(map[string]interface{}{})
			Expect(err).To(HaveOccurred(),
				"referencing a file that doesn't exist must be a build error, "+
					"not silent empty output — matches Liquid's {% include %} behavior")
		})

		It("rejects path traversal outside layouts root", func() {
			tpl, err := includeEngine.Parse("layout", []byte(
				`{{ include "../../../etc/passwd" }}`))
			Expect(err).NotTo(HaveOccurred())

			_, err = tpl.Render(map[string]interface{}{})
			Expect(err).To(HaveOccurred(),
				"include paths that traverse outside the layouts directory must "+
					"be rejected — same sandboxing as Liquid's ReadTemplateFile")
			Expect(err.Error()).To(SatisfyAny(
				ContainSubstring("traversal"),
				ContainSubstring("outside"),
				ContainSubstring("illegal"),
				ContainSubstring("sandbox"),
			),
				"error must explicitly indicate path traversal rejection, not just "+
					"file-not-found — a missing-file error would pass without any "+
					"sandboxing implemented")
		})

		It("renders include output unescaped", func() {
			tpl, err := includeEngine.Parse("layout", []byte(
				`{{ include "partials/header" }}`))
			Expect(err).NotTo(HaveOccurred())

			result, err := tpl.Render(map[string]interface{}{
				"site": map[string]interface{}{"title": "Test"},
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(string(result)).To(ContainSubstring("<header>"),
				"include output must not be HTML-escaped — it returns template.HTML, "+
					"not a raw string, so Go's html/template does not double-escape it")
		})

		It("makes filters available inside includes", func() {
			tmpl.RegisterBuiltinFilters(includeEngine)
			tpl, err := includeEngine.Parse("layout", []byte(
				`{{ include "partials/footer" }}`))
			Expect(err).NotTo(HaveOccurred())

			result, err := tpl.Render(map[string]interface{}{
				"site": map[string]interface{}{"title": "FilterTest"},
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(string(result)).To(ContainSubstring("© FILTERTEST"),
				"FuncMap functions (filters) registered on the engine must be "+
					"available inside includes — footer.html calls {{ upcase .site.title }}, "+
					"which requires the upcase filter in the included file's FuncMap")
		})

		// ── Issue #884: Context passing in range/with blocks ───────
		// The parse-time rewrite (injectIncludeDot) must inject the
		// current dot — inside {{ range }} that's the loop element,
		// inside {{ with }} that's the narrowed scope. A naive
		// implementation that captures root context would pass the
		// wrong value to the partial. Disambiguation: root context
		// has name: "ROOT" but range/with elements have different
		// names — only correct dot passing produces the expected output.

		It("passes current dot to include inside {{ range }}", func() {
			tpl, err := includeEngine.Parse("range-include", []byte(
				`{{ range .items }}{{ include "partials/greeting" }}{{ end }}`))
			Expect(err).NotTo(HaveOccurred())

			result, err := tpl.Render(map[string]interface{}{
				"name": "ROOT",
				"items": []interface{}{
					map[string]interface{}{"name": "Alice"},
					map[string]interface{}{"name": "Bob"},
				},
			})
			Expect(err).NotTo(HaveOccurred())
			output := string(result)
			Expect(output).To(ContainSubstring("Hello, Alice!"),
				"first range element must pass its own dot to the partial — "+
					"greeting.html reads {{ .name }} which must be the element's name")
			Expect(output).To(ContainSubstring("Hello, Bob!"),
				"second range element must also pass its own dot")
			Expect(output).NotTo(ContainSubstring("Hello, ROOT!"),
				"root context must NOT leak into range-scoped includes — "+
					"the parse-time rewrite (injectIncludeDot) must inject "+
					"the current dot, not the root dot")
		})

		It("passes current dot to include inside {{ with }}", func() {
			tpl, err := includeEngine.Parse("with-include", []byte(
				`{{ with .author }}{{ include "partials/greeting" }}{{ end }}`))
			Expect(err).NotTo(HaveOccurred())

			result, err := tpl.Render(map[string]interface{}{
				"name":   "ROOT",
				"author": map[string]interface{}{"name": "Carol"},
			})
			Expect(err).NotTo(HaveOccurred())
			output := string(result)
			Expect(output).To(ContainSubstring("Hello, Carol!"),
				"{{ with .author }} rebinds dot to .author — the include "+
					"must receive .author as context, not the root map")
			Expect(output).NotTo(ContainSubstring("Hello, ROOT!"),
				"root context must NOT leak into with-scoped includes — "+
					"the parse-time rewrite must inject the current dot")
		})
	})

	// ── Issue #884: dict helper error handling ────────────────────────
	// The dict function accepts key-value pairs and returns a map.
	// An odd number of arguments is an error — the function cannot
	// form complete key-value pairs.

	Describe("dict helper function (issue #884)", func() {
		It("returns an error when called with an odd number of arguments (3 args)", func() {
			tpl, err := engine.Parse("dict-odd-3", []byte(
				`{{ dict "key1" "val1" "orphan" }}`))
			Expect(err).NotTo(HaveOccurred(),
				"dict with odd args must parse successfully — the error "+
					"is at render time, not parse time")

			_, err = tpl.Render(map[string]interface{}{})
			Expect(err).To(HaveOccurred(),
				"dict with 3 arguments (odd) must produce a render error")
			Expect(err.Error()).To(ContainSubstring("even number"),
				"error message must indicate that dict requires an even "+
					"number of arguments — not a generic render failure")
		})

		It("returns an error when called with a single argument", func() {
			tpl, err := engine.Parse("dict-odd-1", []byte(
				`{{ dict "lonely" }}`))
			Expect(err).NotTo(HaveOccurred(),
				"dict with 1 arg must parse successfully")

			_, err = tpl.Render(map[string]interface{}{})
			Expect(err).To(HaveOccurred(),
				"dict with 1 argument (odd) must produce a render error")
			Expect(err.Error()).To(ContainSubstring("even number"),
				"error message must match the multi-arg odd case — "+
					"same validation, same error format")
		})

		It("returns an empty map when called with zero arguments", func() {
			tpl, err := engine.Parse("dict-zero", []byte(
				`{{ $d := dict }}{{ len $d }}`))
			Expect(err).NotTo(HaveOccurred(),
				"dict with 0 args must parse successfully")

			result, err := tpl.Render(map[string]interface{}{})
			Expect(err).NotTo(HaveOccurred(),
				"dict with 0 arguments (even) must succeed — "+
					"zero is a valid even number of key-value pairs")
			Expect(string(result)).To(Equal("0"),
				"dict with no arguments must return an empty map (length 0)")
		})
	})

	// ── Issue #884: Concurrent rendering safety ──────────────────────
	// The goEngine uses atomic.Int32 for depth tracking and sync.Map
	// for include caching. This test verifies that multiple goroutines
	// can call Render on the same engine instance simultaneously
	// without data races or incorrect output. The -race flag (used by
	// the CI test command) catches memory safety issues; these
	// assertions verify functional correctness under concurrency.

	Describe("Concurrent rendering (issue #884)", func() {
		It("produces correct output from concurrent Render calls on the same engine", func() {
			concurrentEngine := tmpl.NewGoEngine()
			if setter, ok := concurrentEngine.(interface{ SetIncludesDir(string) }); ok {
				setter.SetIncludesDir("testdata/layouts")
			}

			tpl, err := concurrentEngine.Parse("concurrent", []byte(
				`{{ include "partials/greeting" }}`))
			Expect(err).NotTo(HaveOccurred())

			const n = 10
			type renderResult struct {
				output string
				err    error
			}
			results := make([]renderResult, n)
			var wg sync.WaitGroup
			wg.Add(n)

			for i := 0; i < n; i++ {
				go func(idx int) {
					defer wg.Done()
					name := fmt.Sprintf("Concurrent%d", idx)
					out, err := tpl.Render(map[string]interface{}{
						"name": name,
					})
					results[idx] = renderResult{
						output: string(out),
						err:    err,
					}
				}(i)
			}

			wg.Wait()

			for i := 0; i < n; i++ {
				name := fmt.Sprintf("Concurrent%d", i)
				Expect(results[i].err).NotTo(HaveOccurred(),
					fmt.Sprintf("goroutine %d must render without error", i))
				Expect(results[i].output).To(
					ContainSubstring(fmt.Sprintf("Hello, %s!", name)),
					fmt.Sprintf("goroutine %d must render with its own context — "+
						"context must not leak between concurrent renders", i))
			}
		})
	})

	// ── Layout inheritance ────────────────────────────────────────────

	Describe("Layout inheritance", func() {
		It("supports {{ block }} / {{ define }} layout inheritance", func() {
			baseSrc := `<html>{{ block "content" . }}default{{ end }}</html>`
			tpl, err := engine.Parse("base", []byte(baseSrc))
			Expect(err).NotTo(HaveOccurred())
			Expect(tpl).NotTo(BeNil())

			result, err := tpl.Render(map[string]interface{}{})
			Expect(err).NotTo(HaveOccurred())
			Expect(string(result)).To(ContainSubstring("default"),
				"block must render default content when not overridden")
		})

		It("isolates scope between content and layout rendering", func() {
			layoutSrc := `<html><body>{{ .content }}</body></html>`
			tpl, err := engine.Parse("layout", []byte(layoutSrc))
			Expect(err).NotTo(HaveOccurred())
			Expect(tpl).NotTo(BeNil())

			// Content rendering should not leak layout variables
			result, err := tpl.Render(map[string]interface{}{
				"content":     "<p>Body</p>",
				"layout_only": "should not appear in content",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(string(result)).To(ContainSubstring("<p>Body</p>"))
			Expect(string(result)).NotTo(ContainSubstring("should not appear in content"),
				"layout-only variables must not leak into content area")
		})
	})
})
