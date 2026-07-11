package template_test

import (
	"os"
	"path/filepath"

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
	})

	// ── Issue #880: Extensionless partial fallback removal ───────────
	// resolveInclude must NOT resolve files without an extension.
	// Only .html candidates are tried for the Go template engine.

	Describe("Extensionless partial fallback removal (issue #880)", func() {
		It("does not resolve an extensionless partial file", func() {
			goEngine := tmpl.NewGoEngine()
			setter, ok := goEngine.(interface{ SetIncludesDir(string) })
			Expect(ok).To(BeTrue(),
				"engine must implement SetIncludesDir for include tests")

			tmpDir := GinkgoT().TempDir()
			// Create an extensionless file — no .html suffix
			err := os.WriteFile(
				filepath.Join(tmpDir, "widget"),
				[]byte(`<div>EXTENSIONLESS CONTENT</div>`),
				0644,
			)
			Expect(err).NotTo(HaveOccurred())

			setter.SetIncludesDir(tmpDir)

			tpl, err := goEngine.Parse("test", []byte(
				`{{ include "widget" . }}`))
			Expect(err).NotTo(HaveOccurred())

			_, err = tpl.Render(map[string]interface{}{})
			Expect(err).To(HaveOccurred(),
				"extensionless files must NOT be resolved by resolveInclude — "+
					"only .html candidates should be tried (issue #880)")
			Expect(err.Error()).To(ContainSubstring("widget"),
				"error must reference the requested partial name")
		})

		It("resolves .html file when both extensionless and .html exist", func() {
			goEngine := tmpl.NewGoEngine()
			setter, ok := goEngine.(interface{ SetIncludesDir(string) })
			Expect(ok).To(BeTrue(),
				"engine must implement SetIncludesDir for include tests")

			tmpDir := GinkgoT().TempDir()
			// Extensionless file with distinct content
			err := os.WriteFile(
				filepath.Join(tmpDir, "info"),
				[]byte(`EXTENSIONLESS-INFO`),
				0644,
			)
			Expect(err).NotTo(HaveOccurred())
			// Properly-extensioned .html file
			err = os.WriteFile(
				filepath.Join(tmpDir, "info.html"),
				[]byte(`<p>HTML-INFO</p>`),
				0644,
			)
			Expect(err).NotTo(HaveOccurred())

			setter.SetIncludesDir(tmpDir)

			tpl, err := goEngine.Parse("test", []byte(
				`{{ include "info" . }}`))
			Expect(err).NotTo(HaveOccurred())

			out, err := tpl.Render(map[string]interface{}{})
			Expect(err).NotTo(HaveOccurred())
			Expect(string(out)).To(ContainSubstring("HTML-INFO"),
				".html file must be resolved when both extensionless and .html exist")
			Expect(string(out)).NotTo(ContainSubstring("EXTENSIONLESS-INFO"),
				"extensionless file content must NOT appear in output — "+
					"the .html candidate is found and the extensionless "+
					"fallback must not be in the candidate list (issue #880)")
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
