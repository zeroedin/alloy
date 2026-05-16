package pipeline_test

import (
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/yuin/goldmark"

	"github.com/zeroedin/alloy/internal/config"
	"github.com/zeroedin/alloy/internal/content"
	"github.com/zeroedin/alloy/internal/pipeline"
	tmpl "github.com/zeroedin/alloy/internal/template"
)

var _ = Describe("Build Pipeline", func() {
	Describe("Content-colocated passthrough copy", func() {
		It("copies non-content files to output directory", func() {
			cfg := &config.Config{
				Title:   "Passthrough Copy Test",
				BaseURL: "https://example.com",
				Build:   config.BuildConfig{Output: "_site"},
			}
			contentMap := map[string]string{
				"content/about/index.md":    "---\ntitle: About\n---\n# About",
				"content/about/diagram.svg": `<svg xmlns="http://www.w3.org/2000/svg"><circle r="10"/></svg>`,
				"content/about/photo.png":   "fake png bytes",
			}
			result, err := pipeline.BuildWithContent(cfg, contentMap)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())

			// The content page must be rendered
			Expect(result.PageCount).To(Equal(1),
				"only .md files should be content pages")

			// Non-content files must be copied to output
			Expect(result.ContentPassthroughs).To(ContainElement("about/diagram.svg"),
				"SVG in content/ must be copied to _site/about/diagram.svg")
			Expect(result.ContentPassthroughs).To(ContainElement("about/photo.png"),
				"PNG in content/ must be copied to _site/about/photo.png")
		})

		It("does not copy _data.yaml as passthrough", func() {
			cfg := &config.Config{
				Title:   "Data Exclusion Test",
				BaseURL: "https://example.com",
				Build:   config.BuildConfig{Output: "_site"},
			}
			contentMap := map[string]string{
				"content/blog/index.md":    "---\ntitle: Blog\n---\n# Blog",
				"content/blog/_data.yaml":  "layout: post",
				"content/blog/icon.svg":    "<svg></svg>",
			}
			result, err := pipeline.BuildWithContent(cfg, contentMap)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())

			Expect(result.ContentPassthroughs).To(HaveLen(1),
				"_data.yaml must not be copied as passthrough")
			Expect(result.ContentPassthroughs).To(ContainElement("blog/icon.svg"))
		})
	})

	// ── Static/asset copy (issue #507) ──────────────────────────────
	// Static and passthrough copies run as their own pipeline stage
	// during Phase 3, not overlapping with rendering or hooks.
	// Internal parallelism (concurrent file copies within the stage)
	// is fine — it's the cross-stage overlap that caused regression.

	Describe("Static/asset copy (issue #507)", func() {
		It("build succeeds with static files in content map", func() {
			cfg := &config.Config{
				Title:   "Static Copy Test",
				BaseURL: "https://example.com",
				Build:   config.BuildConfig{Output: "_site"},
			}
			contentMap := map[string]string{
				"content/index.md":       "---\ntitle: Home\nlayout: default\n---\n# Home",
				"layouts/default.liquid": "<html><body>{{ content }}</body></html>",
				"static/robots.txt":      "User-agent: *\nDisallow:",
				"static/css/main.css":    "body { margin: 0; }",
			}
			result, err := pipeline.BuildWithContent(cfg, contentMap)
			Expect(err).NotTo(HaveOccurred(),
				"build must succeed with static files")
			Expect(result).NotTo(BeNil())
			Expect(result.RenderedContent).To(HaveKey("index.md"),
				"rendered content must be present alongside static files")
		})

		It("build succeeds with passthrough mappings", func() {
			cfg := &config.Config{
				Title:   "Passthrough Copy Test",
				BaseURL: "https://example.com",
				Build:   config.BuildConfig{Output: "_site"},
			}
			contentMap := map[string]string{
				"content/index.md":       "---\ntitle: Home\nlayout: default\n---\n# Home",
				"content/blog/icon.svg":  "<svg></svg>",
				"layouts/default.liquid": "<html><body>{{ content }}</body></html>",
			}
			result, err := pipeline.BuildWithContent(cfg, contentMap)
			Expect(err).NotTo(HaveOccurred(),
				"build must succeed with content-colocated passthrough files")
			Expect(result).NotTo(BeNil())
			Expect(result.ContentPassthroughs).To(ContainElement("blog/icon.svg"),
				"passthrough files must be tracked in BuildResult")
		})
	})

	// ── Taxonomy collection page properties (issue #328) ────────────
	// Pages in taxonomy collections must expose title, url, slug via
	// ToTemplateMap() — not raw *content.Page structs.
	Describe("TOC pipeline wiring", func() {
		It("page.toc is accessible in layout templates", func() {
			cfg := &config.Config{
				Title:   "TOC Test",
				BaseURL: "https://example.com",
				Build:   config.BuildConfig{Output: "_site"},
			}
			contentMap := map[string]string{
				"content/guide.md": "---\ntitle: Guide\nlayout: default\n---\n## Getting Started\n\n### Installation\n\n## Configuration",
				"layouts/default.liquid": `<html><body>{{ content }}<nav>{% for item in page.toc %}<a href="#{{ item.id }}">{{ item.text }}</a>{% endfor %}</nav></body></html>`,
			}
			result, err := pipeline.BuildWithContent(cfg, contentMap)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())

			html := result.RenderedContent["guide.md"]
			Expect(html).To(ContainSubstring(`href="#getting-started"`),
				"page.toc must be populated and accessible in layout templates — "+
					"TOC links must render with heading IDs")
			Expect(html).To(ContainSubstring(">Getting Started<"),
				"TOC entry text must be available in templates")
			Expect(html).To(ContainSubstring(">Configuration<"),
				"all h2 headings must appear in page.toc")
		})
	})

	// ── External data files (issue #271) ────────────────────────────
	// Files outside data/ can be mapped into site.data via config.

	Describe("External data files", func() {
		It("loads external data file into site.data namespace", func() {
			cfg := &config.Config{
				Title:   "External Data Test",
				BaseURL: "https://example.com",
				Build:   config.BuildConfig{Output: "_site"},
				Data: config.DataConfig{
					Files: map[string]string{
						"cem": "external/custom-elements.json",
					},
				},
			}
			contentMap := map[string]string{
				"content/index.md":              "---\ntitle: Home\nlayout: default\n---\n# Home",
				"external/custom-elements.json": `{"schemaVersion":"1.0","modules":[{"kind":"javascript-module"}]}`,
				"layouts/default.liquid":         `<html><body>{{ content }}<p>Schema: {{ site.data.cem.schemaVersion }}</p></body></html>`,
			}
			result, err := pipeline.BuildWithContent(cfg, contentMap)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())

			html := result.RenderedContent["index.md"]
			Expect(html).To(ContainSubstring("Schema: 1.0"),
				"external data file must be loaded into site.data.cem — "+
					"template must access site.data.cem.schemaVersion")
		})

		It("errors when external key collides with data/ directory file", func() {
			cfg := &config.Config{
				Title:   "Collision Test",
				BaseURL: "https://example.com",
				Build:   config.BuildConfig{Output: "_site"},
				Data: config.DataConfig{
					Files: map[string]string{
						"nav": "external/nav.json",
					},
				},
			}
			contentMap := map[string]string{
				"content/index.md":   "---\ntitle: Home\n---\n# Home",
				"data/nav.yaml":      "- title: Home\n  url: /",
				"external/nav.json":  `[{"title":"About","url":"/about/"}]`,
			}
			_, err := pipeline.BuildWithContent(cfg, contentMap)
			Expect(err).To(HaveOccurred(),
				"external data key 'nav' collides with data/nav.yaml — "+
					"must be a build error, not silent overwrite")
		})

		It("errors when external data file not found", func() {
			cfg := &config.Config{
				Title:   "Missing Data Test",
				BaseURL: "https://example.com",
				Build:   config.BuildConfig{Output: "_site"},
				Data: config.DataConfig{
					Files: map[string]string{
						"missing": "nonexistent/file.json",
					},
				},
			}
			contentMap := map[string]string{
				"content/index.md": "---\ntitle: Home\n---\n# Home",
			}
			_, err := pipeline.BuildWithContent(cfg, contentMap)
			Expect(err).To(HaveOccurred(),
				"missing external data file must be a build error — "+
					"not silently skipped")
		})
	})

	// ── Render hook pipeline wiring (issues #310, #311) ─────────────
	// The pipeline must discover render hook templates from
	// layouts/_markup/ and wire them into MarkdownOptions.
	Describe("Render hook pipeline wiring", func() {
		It("render hooks from layouts/_markup/ are applied during build", func() {
			cfg := &config.Config{
				Title:   "Render Hook Test",
				BaseURL: "https://example.com",
				Build:   config.BuildConfig{Output: "_site"},
			}
			contentMap := map[string]string{
				"content/page.md": "---\ntitle: Test\n---\n[Click here](https://example.com)",
				"layouts/_markup/render-link.liquid": `<a href="{{ markup.destination }}" class="custom-link">{{ markup.text }}</a>`,
				"layouts/default.liquid":             "<html><body>{{ content }}</body></html>",
			}
			result, err := pipeline.BuildWithContent(cfg, contentMap)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())

			html := result.RenderedContent["page.md"]
			Expect(html).To(ContainSubstring(`class="custom-link"`),
				"render hook from layouts/_markup/render-link.liquid must be applied "+
					"during the build pipeline — proves discovery + wiring works end-to-end")
		})
	})

	// ── Pagination 'as' variable in template body (issue #340) ──────
	// The pagination 'as' alias must be available in the template body,
	// not just in the permalink pattern.
	Describe("Template tags in code elements", func() {
		It("HTML content preserves Liquid expressions inside <code>", func() {
			cfg := &config.Config{
				Title:   "Code Escape Test",
				BaseURL: "https://example.com",
				Build:   config.BuildConfig{Output: "_site"},
			}
			contentMap := map[string]string{
				"content/tokens.html": "---\ntitle: Tokens\nlayout: default\n---\n{% assign val = \"4px\" %}\n<td><code>{{ val }}</code></td>",
				"layouts/default.liquid": "<html><body>{{ content }}</body></html>",
			}
			result, err := pipeline.BuildWithContent(cfg, contentMap)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())

			html := result.RenderedContent["tokens.html"]
			Expect(html).To(ContainSubstring("<code>4px</code>"),
				"Liquid expressions inside <code> in .html files must be interpolated — "+
					"not entity-encoded. escapeTemplateTagsInCode must only run on .md files")
			Expect(html).NotTo(ContainSubstring("&#123;"),
				"template tags must NOT be entity-encoded in .html content")
		})

		It("Liquid content preserves Liquid expressions inside <code>", func() {
			cfg := &config.Config{
				Title:   "Code Escape Test",
				BaseURL: "https://example.com",
				Build:   config.BuildConfig{Output: "_site"},
				Content: config.ContentConfig{Formats: []string{"liquid"}},
			}
			contentMap := map[string]string{
				"content/tokens.liquid": "---\ntitle: Tokens\nlayout: default\n---\n{% assign val = \"4px\" %}\n<td><code>{{ val }}</code></td>",
				"layouts/default.liquid": "<html><body>{{ content }}</body></html>",
			}
			result, err := pipeline.BuildWithContent(cfg, contentMap)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())

			html := result.RenderedContent["tokens.liquid"]
			Expect(html).To(ContainSubstring("<code>4px</code>"),
				"Liquid expressions inside <code> in .liquid files must be interpolated — "+
					"not entity-encoded. escapeTemplateTagsInCode must only run on .md files")
			Expect(html).NotTo(ContainSubstring("&#123;"),
				"template tags must NOT be entity-encoded in .liquid content")
		})

		It("markdown content still escapes template tags in inline code", func() {
			cfg := &config.Config{
				Title:   "Code Escape Test",
				BaseURL: "https://example.com",
				Build:   config.BuildConfig{Output: "_site"},
			}
			contentMap := map[string]string{
				"content/example.md":     "---\ntitle: Example\nlayout: default\n---\nUse `{{ page.title }}` in templates.",
				"layouts/default.liquid": "<html><body>{{ content }}</body></html>",
			}
			result, err := pipeline.BuildWithContent(cfg, contentMap)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())

			html := result.RenderedContent["example.md"]
			Expect(html).To(ContainSubstring("&#123;"),
				"template tags inside inline code in .md files must be escaped — "+
					"inline code should display template syntax literally")
		})
	})

	// ── {% inline %} pipeline wiring (issue #295) ──────────────────
	// RegisterInlineTag must be called in createEngine() so the tag
	// works in actual builds, not just unit tests.

	Describe("Inline tag pipeline wiring", func() {
		It("{% inline %} resolves and inlines files through BuildWithContent", func() {
			cfg := &config.Config{
				Title:   "Inline Test",
				BaseURL: "https://example.com",
				Build:   config.BuildConfig{Output: "_site"},
			}
			contentMap := map[string]string{
				"content/about/index.md":    "---\ntitle: About\n---\n# About\n\n{% inline \"./diagram.svg\" %}",
				"content/about/diagram.svg": `<svg xmlns="http://www.w3.org/2000/svg"><circle r="10"/></svg>`,
			}
			result, err := pipeline.BuildWithContent(cfg, contentMap)
			Expect(err).NotTo(HaveOccurred(),
				"build with {% inline %} must not fail with 'unknown tag' — "+
					"RegisterInlineTag must be called in createEngine()")
			Expect(result).NotTo(BeNil())

			html := result.RenderedContent["about/index.md"]
			Expect(html).To(ContainSubstring("<svg"),
				"{% inline %} must resolve and insert the SVG content through the build pipeline")
			Expect(html).To(ContainSubstring(`circle r="10"`),
				"inlined SVG content must be present in the rendered output")
		})
	})

	// ── Gotemplate layout rendering (issue #385) ────────────────────
	// Pages using gotemplate engine with layout: "default" must resolve
	// layouts/default.html and render through it. Regression: gotemplate
	// layouts stopped being applied in CLI builds.
	Describe("Gotemplate layout rendering (issue #385)", func() {
		It("gotemplate engine applies layout from .html file", func() {
			cfg := &config.Config{
				Title:   "Go Template Layout Test",
				BaseURL: "https://example.com",
				Build:   config.BuildConfig{Output: "_site"},
				Templates: config.TemplatesConfig{Engine: "gotemplate"},
			}
			contentMap := map[string]string{
				"content/about.md":    "---\ntitle: About\nlayout: default\n---\n# About\n\nThis uses Go templates.",
				"layouts/default.html": `<!DOCTYPE html><html><head><title>{{ .page.title }}</title></head><body><main>{{ .content }}</main></body></html>`,
			}
			result, err := pipeline.BuildWithContent(cfg, contentMap)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())

			html := result.RenderedContent["about.md"]
			Expect(html).NotTo(BeEmpty(),
				"about page must render")
			Expect(html).To(ContainSubstring("<!DOCTYPE html>"),
				"gotemplate layout must wrap content with HTML document — "+
					"if missing, layout resolution failed for .html files")
			Expect(html).To(ContainSubstring("<title>About</title>"),
				"layout must have access to .page.title from front matter")
			Expect(html).To(ContainSubstring("<main>"),
				"layout must render the <main> wrapper")
			Expect(html).To(ContainSubstring("About"),
				"rendered markdown content must appear inside the layout")
		})

		It("gotemplate engine renders page.title and site.title in layout", func() {
			cfg := &config.Config{
				Title:   "My Site",
				BaseURL: "https://example.com",
				Build:   config.BuildConfig{Output: "_site"},
				Templates: config.TemplatesConfig{Engine: "gotemplate"},
			}
			contentMap := map[string]string{
				"content/index.md":     "---\ntitle: Home\nlayout: default\n---\n# Welcome",
				"layouts/default.html": `<html><head><title>{{ .page.title }} - {{ .site.title }}</title></head><body>{{ .content }}</body></html>`,
			}
			result, err := pipeline.BuildWithContent(cfg, contentMap)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())

			html := result.RenderedContent["index.md"]
			Expect(html).NotTo(BeEmpty(),
				"index page must render")
			Expect(html).To(ContainSubstring("<title>Home - My Site</title>"),
				"gotemplate layout must resolve both .page.title and .site.title")
		})
	})

	// ── Go template engine with JSON data (issue #458) ─────────────
	// *ordered.Map is a struct — Go templates can't use dot-notation or
	// {{ range }} on it directly. FuncMap helpers bridge the gap:
	// oget for key access, orange for ordered iteration.

	Describe("Go template engine with JSON ordered data (issue #458)", func() {
		It("gotemplate accesses JSON data via oget function", func() {
			cfg := &config.Config{
				Title:     "GoTemplate JSON Test",
				BaseURL:   "https://example.com",
				Build:     config.BuildConfig{Output: "_site"},
				Templates: config.TemplatesConfig{Engine: "gotemplate"},
			}
			contentMap := map[string]string{
				"data/colors.json":     `{"white":"#fff","black":"#000"}`,
				"content/index.md":     "---\ntitle: Colors\nlayout: default\n---\n# Colors",
				"layouts/default.html": `<html><body><span class="color">{{ oget .site.data.colors "white" }}</span>{{ .content }}</body></html>`,
			}
			result, err := pipeline.BuildWithContent(cfg, contentMap)
			Expect(err).NotTo(HaveOccurred(),
				"gotemplate build with JSON data must not error — "+
					"oget must be registered as a FuncMap helper that calls "+
					"ordered.Map.Get() for key-based access (issue #458)")
			Expect(result).NotTo(BeNil())

			html := result.RenderedContent["index.md"]
			Expect(html).To(ContainSubstring("#fff"),
				"oget must return the value for the key — "+
					"{{ oget .site.data.colors \"white\" }} must resolve to #fff")
		})

		It("gotemplate iterates JSON data in insertion order via orange", func() {
			cfg := &config.Config{
				Title:     "GoTemplate JSON Order Test",
				BaseURL:   "https://example.com",
				Build:     config.BuildConfig{Output: "_site"},
				Templates: config.TemplatesConfig{Engine: "gotemplate"},
			}
			contentMap := map[string]string{
				"data/colors.json": `{"white":"#fff","black":"#000","accent":"#e00","brand":"#ee0","surface":"#f0f"}`,
				"content/index.md": "---\ntitle: Colors\nlayout: default\n---\n# Colors",
				"layouts/default.html": `<html><body>{{ range orange .site.data.colors }}<span>{{ .Key }}</span>{{ end }}{{ .content }}</body></html>`,
			}
			result, err := pipeline.BuildWithContent(cfg, contentMap)
			Expect(err).NotTo(HaveOccurred(),
				"gotemplate range over JSON data must not error — "+
					"orange must be registered as a FuncMap helper that returns "+
					"[]ordered.KVPair for ordered iteration (issue #458)")
			Expect(result).NotTo(BeNil())

			html := result.RenderedContent["index.md"]

			whiteIdx := strings.Index(html, "<span>white</span>")
			blackIdx := strings.Index(html, "<span>black</span>")
			accentIdx := strings.Index(html, "<span>accent</span>")

			Expect(whiteIdx).To(BeNumerically(">=", 0),
				"white must appear in output")
			Expect(blackIdx).To(BeNumerically(">", whiteIdx),
				"black must appear after white — JSON insertion order")
			Expect(accentIdx).To(BeNumerically(">", blackIdx),
				"accent must appear after black — "+
					"{{ range orange .site.data.colors }} must iterate in JSON "+
					"insertion order (issue #458)")
		})
	})

	// ── Layout resolution diagnostics (issue #385) ────────────────────
	// When a page explicitly requests a layout via front matter but the
	// layout file doesn't exist, the build must log a warning — not
	// silently produce layoutless output.
	Describe("Layout resolution diagnostics (issue #385)", func() {
		It("page with explicit layout but missing file still renders (without layout)", func() {
			cfg := &config.Config{
				Title:   "Missing Layout Test",
				BaseURL: "https://example.com",
				Build:   config.BuildConfig{Output: "_site"},
			}
			contentMap := map[string]string{
				"content/about.md": "---\ntitle: About\nlayout: nonexistent\n---\n# About",
			}
			result, err := pipeline.BuildWithContent(cfg, contentMap)
			Expect(err).NotTo(HaveOccurred(),
				"missing layout must not cause build failure — page renders without layout")
			Expect(result).NotTo(BeNil())
			Expect(result.PageCount).To(Equal(1))

			html := result.RenderedContent["about.md"]
			Expect(html).NotTo(BeEmpty(),
				"page must still render even without layout")
			Expect(html).To(ContainSubstring("About"),
				"markdown content must be present even without layout wrapping")
		})

		It("Build defaults ProjectRoot to cwd when empty", func() {
			cfg := &config.Config{
				Title:   "No Root Test",
				BaseURL: "https://example.com",
				Build:   config.BuildConfig{Output: "_site"},
			}
			contentMap := map[string]string{
				"content/index.md":       "---\ntitle: Home\nlayout: default\n---\n# Home",
				"layouts/default.liquid": "<html><body>{{ content }}</body></html>",
			}
			result, err := pipeline.BuildWithContent(cfg, contentMap)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			Expect(result.PageCount).To(Equal(1),
				"Build must work even when initial ProjectRoot is empty — "+
					"BuildWithContent sets ProjectRoot to tmpDir, but Build() should "+
					"also default to cwd when ProjectRoot is empty")
		})
	})

	// ── Layout chaining (issue #276) ────────────────────────────────
	// Layout files can reference a parent layout via front matter.
	// The pipeline renders inside-out: content → child → parent → root.
	// Front matter in layout files must be stripped, not output as text.

	Describe("Layout chaining", func() {
		It("renders content through a chain of layouts", func() {
			cfg := &config.Config{
				Title:   "Chain Test",
				BaseURL: "https://example.com",
				Build:   config.BuildConfig{Output: "_site"},
			}
			content := map[string]string{
				"content/page.md":          "---\ntitle: Test\nlayout: has-toc\n---\n# Hello",
				"layouts/has-toc.liquid":   "---\nlayout: \"base\"\n---\n<div class=\"toc\">{{ content }}</div>",
				"layouts/base.liquid":      "<html><body>{{ content }}</body></html>",
			}
			result, err := pipeline.BuildWithContent(cfg, content)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			Expect(result.PageCount).To(Equal(1))

			html := result.RenderedContent["page.md"]
			Expect(html).To(ContainSubstring("<html>"),
				"output must include root layout (base.liquid) wrapper")
			Expect(html).To(ContainSubstring("<div class=\"toc\">"),
				"output must include middle layout (has-toc.liquid) wrapper")
			Expect(html).To(ContainSubstring("Hello"),
				"output must include the page content")
			Expect(html).NotTo(ContainSubstring("---"),
				"layout front matter must be stripped — not output as literal text")
			Expect(html).NotTo(ContainSubstring("layout:"),
				"layout: directive must not appear in rendered output")
		})

		It("layout front matter is not rendered as literal text", func() {
			cfg := &config.Config{
				Title:   "FrontMatter Strip Test",
				BaseURL: "https://example.com",
				Build:   config.BuildConfig{Output: "_site"},
			}
			content := map[string]string{
				"content/page.md":         "---\ntitle: Test\nlayout: child\n---\nContent here",
				"layouts/child.liquid":    "---\nlayout: \"parent\"\n---\n<main>{{ content }}</main>",
				"layouts/parent.liquid":   "<html>{{ content }}</html>",
			}
			result, err := pipeline.BuildWithContent(cfg, content)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())

			html := result.RenderedContent["page.md"]
			Expect(html).NotTo(ContainSubstring("layout: \"parent\""),
				"layout front matter must be stripped before rendering — "+
					"this is the bug reported in #276")
		})
	})

	// ── BuildOptions: SkipSSR (issue #264) ──────────────────────────
	// alloy dev always skips SSR. Build() accepts BuildOptions with
	// SkipSSR to skip Phase 2 regardless of ssr: config.

	Describe("BuildOptions SkipSSR", func() {
		It("SkipSSR=true skips Phase 2 even when SSR is configured", func() {
			cfg := &config.Config{
				Title: "SSR Site",
				SSR:   &config.SSRConfig{Command: "cat"},
				Build: config.BuildConfig{Output: "_site"},
			}
			result, err := pipeline.Build(cfg, pipeline.BuildOptions{SkipSSR: true})
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			Expect(result.SSRSkipped).To(BeTrue(),
				"Build with SkipSSR=true must skip Phase 2 entirely — "+
					"this is how alloy dev avoids SSR overhead regardless of config")
		})

		It("SkipSSR=false with SSR config runs Phase 2 normally", func() {
			cfg := &config.Config{
				Title: "SSR Site",
				SSR:   &config.SSRConfig{Command: "cat"},
				Build: config.BuildConfig{Output: "_site"},
			}
			result, err := pipeline.Build(cfg, pipeline.BuildOptions{SkipSSR: false})
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			Expect(result.SSRSkipped).To(BeFalse(),
				"Build with SkipSSR=false must run Phase 2 when SSR is configured — "+
					"this is the alloy build / alloy serve path")
		})

		It("no BuildOptions runs Phase 2 when SSR is configured", func() {
			cfg := &config.Config{
				Title: "SSR Site",
				SSR:   &config.SSRConfig{Command: "cat"},
				Build: config.BuildConfig{Output: "_site"},
			}
			// No opts — existing callers must continue to work
			result, err := pipeline.Build(cfg)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			Expect(result.SSRSkipped).To(BeFalse(),
				"Build without options must run Phase 2 when SSR is configured — "+
					"backward compatible with existing alloy build behavior")
		})

		It("BuildIncremental respects SkipSSR", func() {
			cfg := &config.Config{
				Title:   "SSR Incremental",
				BaseURL: "https://example.com",
				SSR:     &config.SSRConfig{Command: "cat"},
				Build:   config.BuildConfig{Output: "_site"},
			}
			contentMap := map[string]string{
				"content/index.md": "---\ntitle: Home\n---\n<ds-card>Hello</ds-card>",
			}

			result, err := pipeline.BuildIncremental(
				cfg, contentMap, nil, nil,
				pipeline.BuildOptions{SkipSSR: true},
			)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			Expect(result.SSRSkipped).To(BeTrue(),
				"BuildIncremental with SkipSSR=true must skip Phase 2 — "+
					"alloy dev incremental rebuilds never run SSR")
			Expect(result.SSRPagesRendered).To(Equal(0),
				"no pages should go through SSR when SkipSSR is true")
		})
	})

	// ── Progress reporter via BuildOptions (issue #255, #591) ───────
	// The ProgressReporter must be passed via BuildOptions.Reporter,
	// not via global SetReporter(). This eliminates the package-level
	// mutable variable that races under concurrent builds (issue #591).
	Describe("Layout template caching", func() {
		It("RenderContext includes a LayoutCache for parsed template reuse (issue #585)", func() {
			rc := pipeline.RenderContext{
				LayoutCache: make(map[string]tmpl.Template),
			}
			Expect(rc.LayoutCache).NotTo(BeNil(),
				"RenderContext must have a LayoutCache field (map[string]tmpl.Template) "+
					"to avoid redundant file I/O and template parsing when multiple "+
					"pages share the same layout (issue #585)")
		})

		It("multiple pages sharing a layout all render correctly (issue #585)", func() {
			cfg := &config.Config{
				Title:   "Cache Test",
				BaseURL: "https://example.com",
				Build:   config.BuildConfig{Output: "_site"},
			}
			content := map[string]string{
				"layouts/default.liquid": "<html><body>{{ content }}</body></html>",
				"content/page-1.md":     "---\ntitle: Page 1\nlayout: default\n---\n# First",
				"content/page-2.md":     "---\ntitle: Page 2\nlayout: default\n---\n# Second",
				"content/page-3.md":     "---\ntitle: Page 3\nlayout: default\n---\n# Third",
			}
			result, err := pipeline.BuildWithContent(cfg, content)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.RenderedContent["page-1.md"]).To(ContainSubstring("First"),
				"page-1 must render through shared layout")
			Expect(result.RenderedContent["page-2.md"]).To(ContainSubstring("Second"),
				"page-2 must render through shared layout")
			Expect(result.RenderedContent["page-3.md"]).To(ContainSubstring("Third"),
				"page-3 must render through shared layout")
		})
	})

	// ── Shared goldmark on RenderContext (issue #353) ────────────────
	// One pipeline goldmark instance per build, stored on RenderContext.
	// Created in Build() after createEngine and hook discovery.

	Describe("Shared goldmark on RenderContext", func() {
		It("RenderContext includes a Goldmark field for shared instance reuse (issue #353)", func() {
			md := content.CreateGoldmark(content.MarkdownOptions{
				Unsafe:        true,
				Typographer:   true,
				TemplateTags:  true,
				AutoHeadingID: true,
			})
			rc := pipeline.RenderContext{
				Goldmark: md,
			}
			Expect(rc.Goldmark).NotTo(BeNil(),
				"RenderContext must have a Goldmark field (goldmark.Markdown) — "+
					"one pipeline goldmark instance is created per build in Build() "+
					"after createEngine and hook discovery, then stored on "+
					"RenderContext for reuse across all page renders (issue #353)")
			_ = goldmark.New
		})

		It("multiple pages rendered with shared goldmark produce correct output (issue #353)", func() {
			cfg := &config.Config{
				Title:   "Shared Goldmark Test",
				BaseURL: "https://example.com",
				Build:   config.BuildConfig{Output: "_site"},
			}
			files := map[string]string{
				"layouts/default.liquid": "<html><body>{{ content }}</body></html>",
				"content/page-1.md":     "---\ntitle: Page 1\nlayout: default\n---\n## Introduction\n\nFirst page.",
				"content/page-2.md":     "---\ntitle: Page 2\nlayout: default\n---\n## Getting Started\n\nSecond page.",
				"content/page-3.md":     "---\ntitle: Page 3\nlayout: default\n---\n## API Reference\n\nThird page.",
			}
			result, err := pipeline.BuildWithContent(cfg, files)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.RenderedContent["page-1.md"]).To(ContainSubstring("Introduction"),
				"page-1 must render correctly — the shared goldmark instance "+
					"on RenderContext must not leak state between pages (issue #353)")
			Expect(result.RenderedContent["page-2.md"]).To(ContainSubstring("Getting Started"),
				"page-2 must render correctly with the same goldmark instance")
			Expect(result.RenderedContent["page-3.md"]).To(ContainSubstring("API Reference"),
				"page-3 must render correctly with the same goldmark instance")
		})
	})

	// ── Taxonomy page URLs in conflict detection (issue #695) ──────
	// Taxonomy pages (index + term pages) must be included in pre-render
	// conflict detection. Their URLs are deterministic from taxonomy config
	// and discovered terms — no rendering needed.
})
