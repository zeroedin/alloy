package plugin_test

import (
	"encoding/json"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/zeroedin/alloy/internal/plugin"
)

var _ = Describe("Typed outbound payload structs (issue #529)", func() {

	// ── HookPagePayload ─────────────────────────────────────────────
	// The outbound serialization path must use typed structs with json tags
	// instead of map[string]interface{}. This separates the plugin bridge
	// serialization path from the template rendering path (liquidgo).

	Describe("HookPagePayload struct", func() {
		It("serializes to JSON with correct field names", func() {
			payload := plugin.HookPagePayload{
				Path:        "blog/post.md",
				URL:         "/blog/post/",
				FrontMatter: map[string]interface{}{"title": "Hello", "tags": []string{"go"}},
				Content:     "# Hello\n\nWorld",
				HTML:        "<h1>Hello</h1>\n<p>World</p>",
			}

			data, err := json.Marshal(payload)
			Expect(err).NotTo(HaveOccurred(),
				"HookPagePayload must be JSON-serializable with standard encoding/json — "+
					"sonic.ConfigStd is a superset, so stdlib compat is the baseline (issue #529)")

			var parsed map[string]interface{}
			Expect(json.Unmarshal(data, &parsed)).To(Succeed())

			Expect(parsed).To(HaveKeyWithValue("path", "blog/post.md"),
				"json tag must be 'path' not 'Path' — plugins receive lowercase JSON keys (issue #529)")
			Expect(parsed).To(HaveKeyWithValue("url", "/blog/post/"),
				"json tag must be 'url' — matches existing plugin API contract (issue #529)")
			Expect(parsed).To(HaveKey("frontMatter"),
				"json tag must be 'frontMatter' — camelCase matches existing plugin API (issue #529)")
			Expect(parsed).To(HaveKeyWithValue("content", "# Hello\n\nWorld"),
				"json tag must be 'content' — raw markdown body (issue #529)")
			Expect(parsed).To(HaveKeyWithValue("html", "<h1>Hello</h1>\n<p>World</p>"),
				"json tag must be 'html' — rendered body (issue #529)")
		})

		It("omits content and html when empty", func() {
			payload := plugin.HookPagePayload{
				Path:        "about.md",
				URL:         "/about/",
				FrontMatter: map[string]interface{}{"title": "About"},
			}

			data, err := json.Marshal(payload)
			Expect(err).NotTo(HaveOccurred())

			var parsed map[string]interface{}
			Expect(json.Unmarshal(data, &parsed)).To(Succeed())

			Expect(parsed).NotTo(HaveKey("content"),
				"content must use omitempty — onPagesReady payloads may omit content for read-only pages (issue #529)")
			Expect(parsed).NotTo(HaveKey("html"),
				"html must use omitempty — onPagesReady fires before content rendering, no html exists yet (issue #529)")
		})

		It("serializes nil FrontMatter as null (not omitted)", func() {
			payload := plugin.HookPagePayload{
				Path: "bare.md",
				URL:  "/bare/",
			}

			data, err := json.Marshal(payload)
			Expect(err).NotTo(HaveOccurred())

			var parsed map[string]interface{}
			Expect(json.Unmarshal(data, &parsed)).To(Succeed())

			Expect(parsed).To(HaveKey("frontMatter"),
				"frontMatter must always be present even when nil — plugins expect the key to exist (issue #529)")
		})
	})

	// ── HookTransformPayload ────────────────────────────────────────

	Describe("HookTransformPayload struct", func() {
		It("serializes to JSON with correct field names including toc", func() {
			payload := plugin.HookTransformPayload{
				Path:        "docs/guide.md",
				URL:         "/docs/guide/",
				FrontMatter: map[string]interface{}{"title": "Guide"},
				HTML:        "<h2 id=\"intro\">Intro</h2><p>Text</p>",
				TOC: []plugin.TOCEntry{
					{ID: "intro", Text: "Intro", Level: 2},
				},
			}

			data, err := json.Marshal(payload)
			Expect(err).NotTo(HaveOccurred(),
				"HookTransformPayload must be JSON-serializable (issue #529)")

			var parsed map[string]interface{}
			Expect(json.Unmarshal(data, &parsed)).To(Succeed())

			Expect(parsed).To(HaveKeyWithValue("path", "docs/guide.md"))
			Expect(parsed).To(HaveKeyWithValue("url", "/docs/guide/"))
			Expect(parsed).To(HaveKey("frontMatter"))
			Expect(parsed).To(HaveKeyWithValue("html", "<h2 id=\"intro\">Intro</h2><p>Text</p>"))
			Expect(parsed).To(HaveKey("toc"),
				"json tag must be 'toc' — replaces serializeTOC() manual map construction (issue #529)")

			tocSlice, ok := parsed["toc"].([]interface{})
			Expect(ok).To(BeTrue(), "toc must serialize as a JSON array")
			Expect(tocSlice).To(HaveLen(1))

			tocEntry, ok := tocSlice[0].(map[string]interface{})
			Expect(ok).To(BeTrue())
			Expect(tocEntry).To(HaveKeyWithValue("id", "intro"))
			Expect(tocEntry).To(HaveKeyWithValue("text", "Intro"))
			Expect(tocEntry).To(HaveKeyWithValue("level", float64(2)),
				"level must serialize as a number (issue #529)")
		})

		It("omits toc when nil", func() {
			payload := plugin.HookTransformPayload{
				Path:        "plain.md",
				URL:         "/plain/",
				FrontMatter: map[string]interface{}{"title": "Plain"},
				HTML:        "<p>No headings</p>",
			}

			data, err := json.Marshal(payload)
			Expect(err).NotTo(HaveOccurred())

			var parsed map[string]interface{}
			Expect(json.Unmarshal(data, &parsed)).To(Succeed())

			Expect(parsed).NotTo(HaveKey("toc"),
				"toc must use omitempty — pages with no headings have no TOC entries (issue #529)")
		})
	})

	// ── TOCEntry ────────────────────────────────────────────────────

	Describe("TOCEntry struct", func() {
		It("serializes nested children recursively", func() {
			toc := plugin.TOCEntry{
				ID:    "section",
				Text:  "Section",
				Level: 2,
				Children: []plugin.TOCEntry{
					{ID: "subsection", Text: "Subsection", Level: 3},
				},
			}

			data, err := json.Marshal(toc)
			Expect(err).NotTo(HaveOccurred())

			var parsed map[string]interface{}
			Expect(json.Unmarshal(data, &parsed)).To(Succeed())

			Expect(parsed).To(HaveKey("children"),
				"children must be present when non-empty (issue #529)")

			children, ok := parsed["children"].([]interface{})
			Expect(ok).To(BeTrue())
			Expect(children).To(HaveLen(1))

			child, ok := children[0].(map[string]interface{})
			Expect(ok).To(BeTrue(), "child entry must be a JSON object")
			Expect(child).To(HaveKeyWithValue("id", "subsection"))
			Expect(child).To(HaveKeyWithValue("text", "Subsection"))
			Expect(child).To(HaveKeyWithValue("level", float64(3)))
		})

		It("omits children when empty", func() {
			toc := plugin.TOCEntry{
				ID:    "leaf",
				Text:  "Leaf",
				Level: 3,
			}

			data, err := json.Marshal(toc)
			Expect(err).NotTo(HaveOccurred())

			var parsed map[string]interface{}
			Expect(json.Unmarshal(data, &parsed)).To(Succeed())

			Expect(parsed).NotTo(HaveKey("children"),
				"children must use omitempty — leaf TOC entries have no children (issue #529)")
		})
	})

	// ── HookPagesReadyPayload ───────────────────────────────────────

	Describe("HookPagesReadyPayload struct", func() {
		It("serializes pages array and siteData together", func() {
			payload := plugin.HookPagesReadyPayload{
				Pages: []plugin.HookPagePayload{
					{
						Path:        "index.md",
						URL:         "/",
						FrontMatter: map[string]interface{}{"title": "Home"},
						Content:     "# Home",
					},
				},
				SiteData: map[string]interface{}{
					"elements": []interface{}{
						map[string]interface{}{"tagName": "my-button"},
					},
				},
			}

			data, err := json.Marshal(payload)
			Expect(err).NotTo(HaveOccurred(),
				"HookPagesReadyPayload must be JSON-serializable (issue #529)")

			var parsed map[string]interface{}
			Expect(json.Unmarshal(data, &parsed)).To(Succeed())

			Expect(parsed).To(HaveKey("pages"),
				"json tag must be 'pages' — matches onPagesReady payload contract (issue #529)")
			Expect(parsed).To(HaveKey("siteData"),
				"json tag must be 'siteData' — matches onPagesReady payload contract (issue #529)")

			pages, ok := parsed["pages"].([]interface{})
			Expect(ok).To(BeTrue())
			Expect(pages).To(HaveLen(1))

			siteData, ok := parsed["siteData"].(map[string]interface{})
			Expect(ok).To(BeTrue())
			Expect(siteData).To(HaveKey("elements"))
		})

		It("serializes empty pages as empty array, not null", func() {
			payload := plugin.HookPagesReadyPayload{
				Pages:    []plugin.HookPagePayload{},
				SiteData: map[string]interface{}{},
			}

			data, err := json.Marshal(payload)
			Expect(err).NotTo(HaveOccurred())

			raw := string(data)
			Expect(raw).To(ContainSubstring(`"pages":[]`),
				"empty pages slice must serialize as [] not null — plugins distinguish empty batch from missing field (issue #529)")
			Expect(raw).To(ContainSubstring(`"siteData":{}`),
				"empty siteData must serialize as {} not null (issue #529)")
		})
	})

	// ── HookCascadePayload ──────────────────────────────────────────

	Describe("HookCascadePayload struct", func() {
		It("serializes path and data", func() {
			payload := plugin.HookCascadePayload{
				Path: "blog/post.md",
				Data: map[string]interface{}{"author": "Steven", "category": "tech"},
			}

			data, err := json.Marshal(payload)
			Expect(err).NotTo(HaveOccurred(),
				"HookCascadePayload must be JSON-serializable (issue #529)")

			var parsed map[string]interface{}
			Expect(json.Unmarshal(data, &parsed)).To(Succeed())

			Expect(parsed).To(HaveKeyWithValue("path", "blog/post.md"))
			Expect(parsed).To(HaveKey("data"),
				"json tag must be 'data' — matches onDataCascadeReady payload contract (issue #529)")
		})
	})

	// ── HookAssetPayload ────────────────────────────────────────────

	Describe("HookAssetPayload struct", func() {
		It("serializes path and content", func() {
			payload := plugin.HookAssetPayload{
				Path:    "assets/style.css",
				Content: "body { color: red; }",
			}

			data, err := json.Marshal(payload)
			Expect(err).NotTo(HaveOccurred(),
				"HookAssetPayload must be JSON-serializable (issue #529)")

			var parsed map[string]interface{}
			Expect(json.Unmarshal(data, &parsed)).To(Succeed())

			Expect(parsed).To(HaveKeyWithValue("path", "assets/style.css"))
			Expect(parsed).To(HaveKeyWithValue("content", "body { color: red; }"))
		})
	})
})
