package pipeline_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/zeroedin/alloy/internal/config"
	"github.com/zeroedin/alloy/internal/pipeline"
)

var _ = Describe("Build Pipeline", func() {
	Describe("onContentTransformed page object payload (issue #448)", func() {
		It("onContentTransformed receives page object with toc and frontMatter", func() {
			cfg := &config.Config{
				Title:   "Hook Payload Test",
				BaseURL: "https://example.com",
				Build:   config.BuildConfig{Output: "_site"},
			}
			contentMap := map[string]string{
				"content/about.md": "---\ntitle: About\nlayout: default\n---\n## Section One\n\nContent here.\n\n## Section Two\n\nMore content.",
				"layouts/default.liquid": "<html><body>{{ content }}</body></html>",
				"plugins/toc-check.js": "export default function(alloy) {\n  alloy.hook('onContentTransformed', {}, (page) => {\n    if (typeof page === 'string') throw new Error('payload must be object, got string');\n    if (!page.html) throw new Error('page.html missing');\n    if (!page.path) throw new Error('page.path missing');\n    if (!page.frontMatter) throw new Error('page.frontMatter missing');\n    return page;\n  });\n}",
			}
			result, err := pipeline.BuildWithContent(cfg, contentMap)
			Expect(err).NotTo(HaveOccurred(),
				"onContentTransformed must receive a page object, not a string — "+
					"if this fails with 'payload must be object, got string', "+
					"the hook still sends string(page.RenderedBody) instead of "+
					"the page object {html, toc, path, url, frontMatter} (issue #448)")
			Expect(result).NotTo(BeNil())
		})

		It("onContentTransformed can mutate page.toc", func() {
			cfg := &config.Config{
				Title:   "TOC Mutation Test",
				BaseURL: "https://example.com",
				Build:   config.BuildConfig{Output: "_site"},
			}
			contentMap := map[string]string{
				"content/index.html": "---\ntitle: Index\nlayout: default\n---\n<h2 id=\"custom\">Custom Heading</h2>\n<p>No goldmark TOC for HTML content.</p>",
				"layouts/default.liquid": "<html><body>{% for entry in page.toc %}<a href=\"#{{ entry.id }}\">{{ entry.text }}</a>{% endfor %}{{ content }}</body></html>",
				"plugins/toc-builder.js": "export default function(alloy) {\n  alloy.hook('onContentTransformed', {}, (page) => {\n    if (!page.toc || page.toc.length === 0) {\n      page.toc = [{id: 'custom', text: 'Custom Heading', level: 2}];\n    }\n    return page;\n  });\n}",
			}
			result, err := pipeline.BuildWithContent(cfg, contentMap)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())

			html := result.RenderedContent["index.html"]
			Expect(html).To(ContainSubstring("Custom Heading</a>"),
				"plugin-built TOC must be available in layout via page.toc — "+
					"the onContentTransformed hook must be able to set page.toc "+
					"for non-markdown pages that don't go through goldmark (issue #448)")
		})
	})

	// ── Hook return values applied to pipeline state (issue #494) ───
	// Hooks documented as mutators (onDataFetched, onContentLoaded) must
	// apply their return values back to the pipeline. Currently both
	// discard returns with `_`. RunWithTimeout already chains results
	// correctly — callers need to stop discarding them.

	Describe("onDataFetched return value applied to siteData (issue #494)", func() {
		It("plugin-injected data via onDataFetched is accessible in templates", func() {
			cfg := &config.Config{
				Title:   "Data Hook Test",
				BaseURL: "https://example.com",
				Build:   config.BuildConfig{Output: "_site"},
			}
			contentMap := map[string]string{
				"data/site.json":         `{"name":"test"}`,
				"content/index.md":       "---\ntitle: Home\nlayout: default\n---\n# Home",
				"layouts/default.liquid": "<html><body>{{ content }}<p>Count: {{ site.data.demos | size }}</p></body></html>",
				"plugins/inject-data.js": `export default function(alloy) {
  alloy.hook('onDataFetched', { data: ["*"] }, (data) => {
    data.demos = [
      { name: 'button', slug: 'button' },
      { name: 'card', slug: 'card' },
      { name: 'dialog', slug: 'dialog' }
    ];
    return data;
  });
}`,
			}
			result, err := pipeline.BuildWithContent(cfg, contentMap)
			Expect(err).NotTo(HaveOccurred(),
				"onDataFetched hook must not error when returning modified siteData")
			Expect(result).NotTo(BeNil())

			html := result.RenderedContent["index.md"]
			Expect(html).To(ContainSubstring("Count: 3"),
				"onDataFetched return value must be applied back to siteData — "+
					"currently the return is discarded with _ at build.go:285 (issue #494)")
		})

		It("plugin can modify existing data keys via onDataFetched", func() {
			cfg := &config.Config{
				Title:   "Data Modify Test",
				BaseURL: "https://example.com",
				Build:   config.BuildConfig{Output: "_site"},
			}
			contentMap := map[string]string{
				"data/team.json":         `[{"name":"Alice"},{"name":"Bob"}]`,
				"content/index.md":       "---\ntitle: Home\nlayout: default\n---\n# Home",
				"layouts/default.liquid": "<html><body>{{ content }}<p>Team: {{ site.data.team | size }}</p></body></html>",
				"plugins/enrich-data.js": `export default function(alloy) {
  alloy.hook('onDataFetched', { data: ["*"] }, (data) => {
    if (data.team) {
      data.team.push({ name: 'Charlie' });
    }
    return data;
  });
}`,
			}
			result, err := pipeline.BuildWithContent(cfg, contentMap)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())

			html := result.RenderedContent["index.md"]
			Expect(html).To(ContainSubstring("Team: 3"),
				"onDataFetched must allow plugins to modify existing data keys — "+
					"team array should have 3 members after plugin appends one (issue #494)")
		})
	})

	// ── Ordered map type preservation through hook serialization (#571) ─
	// When *ordered.Map values pass through the plugin serialization
	// boundary (JSON round-trip), they must be restored as *ordered.Map
	// so Each() iteration and insertion order are preserved.

	Describe("ordered map type preservation through hook serialization (issue #571)", func() {
		It("ordered map data survives onDataFetched round-trip with insertion order", func() {
			cfg := &config.Config{
				Title:   "Ordered Map Test",
				BaseURL: "https://example.com",
				Build:   config.BuildConfig{Output: "_site"},
			}
			contentMap := map[string]string{
				"data/tokens.json":       `{"red":"#f00","green":"#0f0","blue":"#00f"}`,
				"content/index.md":       "---\ntitle: Home\nlayout: default\n---\n# Home",
				"layouts/default.liquid": `<html><body>{{ content }}{% for pair in site.data.tokens %}{{ pair[0] }}:{% endfor %}</body></html>`,
				"plugins/passthrough.js": "export default function(alloy) {\n  alloy.hook('onDataFetched', { data: [\"*\"] }, (data) => {\n    return data;\n  });\n}",
			}
			result, err := pipeline.BuildWithContent(cfg, contentMap)
			Expect(err).NotTo(HaveOccurred(),
				"onDataFetched passthrough hook must not error")
			Expect(result).NotTo(BeNil())

			html := result.RenderedContent["index.md"]
			Expect(html).To(ContainSubstring("red:green:blue:"),
				"ordered map data must survive onDataFetched hook round-trip with insertion "+
					"order preserved — the JSON serialization boundary currently converts "+
					"*ordered.Map to map[string]interface{}, losing Each() support and key "+
					"order. Fix: deserialize hook results through ordered.UnmarshalJSONValue "+
					"instead of standard json.Unmarshal (issue #571)")
		})

		It("nested ordered map survives onDataFetched round-trip", func() {
			cfg := &config.Config{
				Title:   "Nested Map Test",
				BaseURL: "https://example.com",
				Build:   config.BuildConfig{Output: "_site"},
			}
			contentMap := map[string]string{
				"data/tokens.json":       `{"color":{"red":{"name":"Red","value":"#f00"},"blue":{"name":"Blue","value":"#00f"}}}`,
				"content/index.md":       "---\ntitle: Home\nlayout: default\n---\n# Home",
				"layouts/default.liquid": `<html><body>{{ content }}{% for pair in site.data.tokens.color %}{{ pair[0] }}:{% endfor %}</body></html>`,
				"plugins/passthrough.js": "export default function(alloy) {\n  alloy.hook('onDataFetched', { data: [\"*\"] }, (data) => {\n    return data;\n  });\n}",
			}
			result, err := pipeline.BuildWithContent(cfg, contentMap)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())

			html := result.RenderedContent["index.md"]
			Expect(html).To(ContainSubstring("red:blue:"),
				"nested ordered maps must also survive hook round-trip — "+
					"site.data.tokens.color is a nested *ordered.Map that must retain "+
					"Each() after JSON serialization/deserialization (issue #571)")
		})
	})

	Describe("onContentLoaded return value applied to pages (issue #494)", func() {
		It("plugin can modify page front matter via onContentLoaded", func() {
			cfg := &config.Config{
				Title:   "Content Hook Test",
				BaseURL: "https://example.com",
				Build:   config.BuildConfig{Output: "_site"},
			}
			contentMap := map[string]string{
				"content/index.md":       "---\ntitle: Home\nlayout: default\n---\n# Home",
				"layouts/default.liquid": "<html><body><h1>{{ page.title }}</h1>{{ content }}</body></html>",
				"plugins/enrich-pages.js": `export default function(alloy) {
  alloy.hook('onContentLoaded', { pages: true, pageFields: ["*"] }, function(pages) {
    for (var i = 0; i < pages.length; i++) {
      pages[i].frontMatter.title = pages[i].frontMatter.title + ' (enriched)';
    }
    return pages;
  });
}`,
			}
			result, err := pipeline.BuildWithContent(cfg, contentMap)
			Expect(err).NotTo(HaveOccurred(),
				"onContentLoaded hook must not error when returning modified pages")
			Expect(result).NotTo(BeNil())

			html := result.RenderedContent["index.md"]
			Expect(html).To(ContainSubstring("Home (enriched)"),
				"onContentLoaded return value must be applied back to pages — "+
					"currently the return is discarded with _ at build.go:479 (issue #494)")
		})
	})

	// ── onContentLoaded rejects virtual page injection (issues #518, #525, #521) ─────
	// Virtual page injection has moved exclusively to onPagesReady (#525).
	// onContentLoaded is limited to modifying existing pages — if the
	// returned array is longer than the input, the pipeline produces
	// a validation error. This also resolves #521 (virtual pages appended
	// to wrong language batch) since onContentLoaded no longer handles injection.

	Describe("onContentLoaded rejects virtual page injection (issues #518, #525, #521)", func() {
		It("onContentLoaded returning extra pages produces a validation error", func() {
			cfg := &config.Config{
				Title:   "Reject Virtual Test",
				BaseURL: "https://example.com",
				Build:   config.BuildConfig{Output: "_site"},
			}
			contentMap := map[string]string{
				"content/index.md":       "---\ntitle: Home\nlayout: default\n---\n# Home",
				"layouts/default.liquid": "<html><body>{{ content }}</body></html>",
				"plugins/inject-rejected.js": `export default function(alloy) {
  alloy.hook('onContentLoaded', { pages: true, pageFields: ["*"] }, function(pages) {
    pages.push({
      path: 'demos/button.html',
      url: '/demos/button/',
      frontMatter: { title: 'Button Demo', layout: 'default' },
      html: '<h1>Button Demo</h1>'
    });
    return pages;
  });
}`,
			}
			_, err := pipeline.BuildWithContent(cfg, contentMap)
			Expect(err).To(HaveOccurred(),
				"onContentLoaded must reject virtual page injection — "+
					"returned array length exceeds input length. "+
					"Virtual pages belong in onPagesReady (#525). "+
					"This also prevents the wrong-batch routing bug (#521)")
		})

		It("onContentLoaded can still modify existing page front matter", func() {
			cfg := &config.Config{
				Title:   "Modify Only Test",
				BaseURL: "https://example.com",
				Build:   config.BuildConfig{Output: "_site"},
			}
			contentMap := map[string]string{
				"content/index.md":       "---\ntitle: Home\nlayout: default\n---\n# Home",
				"layouts/default.liquid": "<html><body><h1>{{ page.title }}</h1>{{ content }}</body></html>",
				"plugins/modify-only.js": `export default function(alloy) {
  alloy.hook('onContentLoaded', { pages: true, pageFields: ["*"] }, function(pages) {
    for (var i = 0; i < pages.length; i++) {
      pages[i].frontMatter.title = pages[i].frontMatter.title + ' (modified)';
    }
    return pages;
  });
}`,
			}
			result, err := pipeline.BuildWithContent(cfg, contentMap)
			Expect(err).NotTo(HaveOccurred(),
				"onContentLoaded modifying existing pages must not error — "+
					"same-length return is valid (#525)")
			Expect(result).NotTo(BeNil())

			html := result.RenderedContent["index.md"]
			Expect(html).To(ContainSubstring("Home (modified)"),
				"onContentLoaded must still apply front matter modifications to existing pages — "+
					"only virtual page injection is removed, not mutation (#525)")
		})
	})

	// ── onDataCascadeReady return value applied to cascade (issue #520) ───────
	// onDataCascadeReady fires after cascade resolution with the full pages
	// array. Payload shape is [{ path, data: { ... } }] per HookCascadePayload
	// (payload.go:39-42). Return must be same shape, same length — plugins can
	// modify cascade data values but cannot inject or remove pages.
	// The return value must be deserialized and applied back to page state,
	// same pattern as onContentLoaded (build.go:501-536).

	Describe("onDataCascadeReady return value applied to cascade (issue #520)", func() {
		It("plugin can enrich page cascade data via onDataCascadeReady", func() {
			cfg := &config.Config{
				Title:   "Cascade Hook Test",
				BaseURL: "https://example.com",
				Build:   config.BuildConfig{Output: "_site"},
			}
			contentMap := map[string]string{
				"content/index.md":       "---\ntitle: Home\nlayout: default\n---\n# Home",
				"layouts/default.liquid": "<html><body><h1>{{ page.title }}</h1><span>{{ page.enriched }}</span>{{ content }}</body></html>",
				"plugins/cascade-enrich.js": `export default function(alloy) {
  alloy.hook('onDataCascadeReady', { pages: true }, function(pages) {
    for (var i = 0; i < pages.length; i++) {
      pages[i].data.enriched = 'cascade-injected';
    }
    return pages;
  });
}`,
			}
			result, err := pipeline.BuildWithContent(cfg, contentMap)
			Expect(err).NotTo(HaveOccurred(),
				"onDataCascadeReady hook must not error when returning modified cascade data")
			Expect(result).NotTo(BeNil())

			html := result.RenderedContent["index.md"]
			Expect(html).To(ContainSubstring("cascade-injected"),
				"onDataCascadeReady return value must be applied back to page cascade — "+
					"currently the return is discarded in the onDataCascadeReady "+
					"RunWithTimeout call. The payload shape is { path, data } per "+
					"HookCascadePayload (payload.go), and data mutations must be "+
					"written back to page.FrontMatter like onContentLoaded does "+
					"(issue #520)")
		})

		It("onDataCascadeReady returning extra entries produces a validation error", func() {
			cfg := &config.Config{
				Title:   "Cascade Reject Test",
				BaseURL: "https://example.com",
				Build:   config.BuildConfig{Output: "_site"},
			}
			contentMap := map[string]string{
				"content/index.md":       "---\ntitle: Home\nlayout: default\n---\n# Home",
				"layouts/default.liquid": "<html><body>{{ content }}</body></html>",
				"plugins/cascade-inject.js": `export default function(alloy) {
  alloy.hook('onDataCascadeReady', { pages: true }, function(pages) {
    pages.push({ path: 'fake/page.md', data: { title: 'Fake' } });
    return pages;
  });
}`,
			}
			_, err := pipeline.BuildWithContent(cfg, contentMap)
			Expect(err).To(HaveOccurred(),
				"onDataCascadeReady must reject virtual page injection — "+
					"returned array length exceeds input length. "+
					"Same constraint as onContentLoaded (issue #520)")
		})

		It("onDataCascadeReady returning fewer entries produces a validation error", func() {
			cfg := &config.Config{
				Title:   "Cascade Remove Test",
				BaseURL: "https://example.com",
				Build:   config.BuildConfig{Output: "_site"},
			}
			contentMap := map[string]string{
				"content/index.md":       "---\ntitle: Home\nlayout: default\n---\n# Home",
				"content/about.md":      "---\ntitle: About\nlayout: default\n---\n# About",
				"layouts/default.liquid": "<html><body>{{ content }}</body></html>",
				"plugins/cascade-remove.js": `export default function(alloy) {
  alloy.hook('onDataCascadeReady', { pages: true }, function(pages) {
    return pages.slice(0, 1);
  });
}`,
			}
			_, err := pipeline.BuildWithContent(cfg, contentMap)
			Expect(err).To(HaveOccurred(),
				"onDataCascadeReady must reject page removal — "+
					"returned array length less than input length. "+
					"Same constraint as onContentLoaded (issue #520)")
		})
	})

	// ── onPagesReady hook for pre-taxonomy virtual page injection (issue #525) ─────
	// onPagesReady fires after data cascade but before taxonomy collection.
	// Virtual pages injected here participate in taxonomy collections —
	// unlike onContentLoaded which fires after taxonomies are built.
	// Payload: { pages: [...], siteData: {...} }. No html field.
	// Virtual pages provide raw content (markdown) that flows through
	// the content rendering pipeline.

	Describe("onPagesReady pre-taxonomy virtual page injection (issue #525)", func() {
		It("plugin can inject a virtual page via onPagesReady that appears in build output", func() {
			cfg := &config.Config{
				Title:   "PagesReady Test",
				BaseURL: "https://example.com",
				Build:   config.BuildConfig{Output: "_site"},
			}
			contentMap := map[string]string{
				"content/index.md":       "---\ntitle: Home\nlayout: default\n---\n# Home",
				"layouts/default.liquid": "<html><body>{{ content }}</body></html>",
				"plugins/inject-pages.js": `export default function(alloy) {
  alloy.hook('onPagesReady', { pages: true, pageFields: ["*"] }, function(payload) {
    payload.pages.push({
      path: 'demos/button.md',
      url: '/demos/button/',
      frontMatter: { title: 'Button Demo', layout: 'default' },
      content: '# Button\n\nA button component.'
    });
    return payload;
  });
}`,
			}
			result, err := pipeline.BuildWithContent(cfg, contentMap)
			Expect(err).NotTo(HaveOccurred(),
				"onPagesReady hook must not error when returning virtual pages (issue #525)")
			Expect(result).NotTo(BeNil())

			Expect(result.PageCount).To(Equal(2),
				"1 real + 1 virtual page injected via onPagesReady = 2 total (issue #525)")
			Expect(result.RenderedContent).To(HaveKey("demos/button.md"),
				"virtual page injected via onPagesReady must appear in RenderedContent (issue #525)")
		})

		It("virtual page injected via onPagesReady participates in taxonomy collections", func() {
			renderFalse := false
			cfg := &config.Config{
				Title:   "PagesReady Taxonomy Test",
				BaseURL: "https://example.com",
				Build:   config.BuildConfig{Output: "_site"},
				Taxonomies: map[string]*config.TaxonomyConfig{
					"tags": {Render: &renderFalse},
				},
			}
			contentMap := map[string]string{
				"content/index.md": "---\ntitle: Home\nlayout: default\ntags: [\"core\"]\n---\n{% for p in taxonomies.tags.demo %}<span class=\"injected\">{{ p.title }}</span>{% endfor %}",
				"layouts/default.liquid": "<html><body>{{ content }}</body></html>",
				"plugins/inject-tagged.js": `export default function(alloy) {
  alloy.hook('onPagesReady', { pages: true, pageFields: ["*"] }, function(payload) {
    payload.pages.push({
      path: 'demos/accordion.md',
      url: '/demos/accordion/',
      frontMatter: {
        title: 'Accordion Demo',
        layout: 'default',
        tags: ['demo']
      },
      content: '# Accordion'
    });
    payload.pages.push({
      path: 'demos/tabs.md',
      url: '/demos/tabs/',
      frontMatter: {
        title: 'Tabs Demo',
        layout: 'default',
        tags: ['demo']
      },
      content: '# Tabs'
    });
    return payload;
  });
}`,
			}
			result, err := pipeline.BuildWithContent(cfg, contentMap)
			Expect(err).NotTo(HaveOccurred(),
				"onPagesReady with taxonomy terms must not error (issue #525)")
			Expect(result).NotTo(BeNil())

			html := result.RenderedContent["index.md"]
			Expect(html).To(ContainSubstring(`class="injected"`),
				"taxonomies.tags.demo must be iterable in templates — "+
					"if empty, onPagesReady virtual pages did not participate in taxonomy collection (issue #525)")
			Expect(html).To(ContainSubstring("Accordion Demo"),
				"virtual page 'Accordion Demo' tagged 'demo' must appear in taxonomies.tags.demo — "+
					"this is the core value of onPagesReady over onContentLoaded (issue #525)")
			Expect(html).To(ContainSubstring("Tabs Demo"),
				"virtual page 'Tabs Demo' tagged 'demo' must appear in taxonomies.tags.demo (issue #525)")
		})

		It("virtual page raw content is rendered through the markdown pipeline", func() {
			cfg := &config.Config{
				Title:   "PagesReady Content Render Test",
				BaseURL: "https://example.com",
				Build:   config.BuildConfig{Output: "_site"},
			}
			contentMap := map[string]string{
				"content/index.md":       "---\ntitle: Home\nlayout: default\n---\n# Home",
				"layouts/default.liquid": "<html><body>{{ content }}</body></html>",
				"plugins/inject-md.js": `export default function(alloy) {
  alloy.hook('onPagesReady', { pages: true, pageFields: ["*"] }, function(payload) {
    payload.pages.push({
      path: 'demos/button.md',
      url: '/demos/button/',
      frontMatter: { title: 'Button', layout: 'default' },
      content: '## Button Component\n\nA **bold** button.'
    });
    return payload;
  });
}`,
			}
			result, err := pipeline.BuildWithContent(cfg, contentMap)
			Expect(err).NotTo(HaveOccurred(),
				"onPagesReady with markdown content must not error (issue #525)")
			Expect(result).NotTo(BeNil())

			html := result.RenderedContent["demos/button.md"]
			Expect(html).To(ContainSubstring("<h2"),
				"markdown ## heading must be rendered to <h2> — "+
					"onPagesReady virtual pages with raw content must flow through content rendering (issue #525)")
			Expect(html).To(ContainSubstring("<strong>bold</strong>"),
				"markdown **bold** must be rendered to <strong> — "+
					"raw content from onPagesReady must be processed by goldmark (issue #525)")
		})

		It("virtual page with layout: false skips layout wrapping", func() {
			cfg := &config.Config{
				Title:   "PagesReady No Layout Test",
				BaseURL: "https://example.com",
				Build:   config.BuildConfig{Output: "_site"},
			}
			contentMap := map[string]string{
				"content/index.md":       "---\ntitle: Home\nlayout: default\n---\n# Home",
				"layouts/default.liquid": "<html><body>{{ content }}</body></html>",
				"plugins/raw-page.js": `export default function(alloy) {
  alloy.hook('onPagesReady', { pages: true, pageFields: ["*"] }, function(payload) {
    payload.pages.push({
      path: 'embed/widget.html',
      url: '/embed/widget/',
      frontMatter: { title: 'Widget', layout: false },
      content: '<div class="widget">Embeddable widget</div>'
    });
    return payload;
  });
}`,
			}
			result, err := pipeline.BuildWithContent(cfg, contentMap)
			Expect(err).NotTo(HaveOccurred(),
				"onPagesReady with layout: false must not error (issue #525)")
			Expect(result).NotTo(BeNil())

			html := result.RenderedContent["embed/widget.html"]
			Expect(html).To(ContainSubstring("Embeddable widget"),
				"virtual page with layout: false must appear in output (issue #525)")
			Expect(html).NotTo(ContainSubstring("<html>"),
				"virtual page with layout: false must NOT be wrapped in a layout — "+
					"content should be written as-is (issue #525)")
		})

		It("output-path collision between onPagesReady virtual page and real page produces error", func() {
			cfg := &config.Config{
				Title:   "PagesReady Collision Test",
				BaseURL: "https://example.com",
				Build:   config.BuildConfig{Output: "_site"},
			}
			contentMap := map[string]string{
				"content/index.md":       "---\ntitle: Home\nlayout: default\n---\n# Home",
				"layouts/default.liquid": "<html><body>{{ content }}</body></html>",
				"plugins/collide.js": `export default function(alloy) {
  alloy.hook('onPagesReady', { pages: true, pageFields: ["*"] }, function(payload) {
    payload.pages.push({
      path: 'virtual-index.md',
      url: '/',
      frontMatter: { title: 'Collision', layout: 'default' },
      content: '# Collision'
    });
    return payload;
  });
}`,
			}
			_, err := pipeline.BuildWithContent(cfg, contentMap)
			Expect(err).To(HaveOccurred(),
				"output-path collision between a virtual page and a real page must produce a build error — "+
					"silent overwrites would cause data loss (issue #525)")
		})

		It("onPagesReady virtual page missing path or url produces validation error", func() {
			cfg := &config.Config{
				Title:   "PagesReady Validation Test",
				BaseURL: "https://example.com",
				Build:   config.BuildConfig{Output: "_site"},
			}
			contentMap := map[string]string{
				"content/index.md":       "---\ntitle: Home\nlayout: default\n---\n# Home",
				"layouts/default.liquid": "<html><body>{{ content }}</body></html>",
				"plugins/bad-page.js": `export default function(alloy) {
  alloy.hook('onPagesReady', { pages: true, pageFields: ["*"] }, function(payload) {
    payload.pages.push({
      frontMatter: { title: 'No Path' },
      content: '# Missing fields'
    });
    return payload;
  });
}`,
			}
			_, err := pipeline.BuildWithContent(cfg, contentMap)
			Expect(err).To(HaveOccurred(),
				"a virtual page with no path/url cannot be routed — "+
					"must produce a validation error (issue #525)")
		})

		It("onPagesReady payload includes siteData for data-driven page generation", func() {
			cfg := &config.Config{
				Title:   "PagesReady SiteData Test",
				BaseURL: "https://example.com",
				Build:   config.BuildConfig{Output: "_site"},
			}
			contentMap := map[string]string{
				"data/elements.json":     `[{"name":"Button","slug":"button"},{"name":"Card","slug":"card"},{"name":"Dialog","slug":"dialog"}]`,
				"content/index.md":       "---\ntitle: Home\nlayout: default\n---\n# Home",
				"layouts/default.liquid": "<html><body>{{ content }}</body></html>",
				"plugins/data-pages.js": `export default function(alloy) {
  alloy.hook('onPagesReady', { pages: true, pageFields: ["*"], data: ["*"] }, function(payload) {
    var elements = payload.siteData.elements || [];
    for (var i = 0; i < elements.length; i++) {
      var el = elements[i];
      payload.pages.push({
        path: 'demos/' + el.slug + '.md',
        url: '/demos/' + el.slug + '/',
        frontMatter: { title: el.name + ' Demo', layout: 'default' },
        content: '# ' + el.name
      });
    }
    return payload;
  });
}`,
			}
			result, err := pipeline.BuildWithContent(cfg, contentMap)
			Expect(err).NotTo(HaveOccurred(),
				"onPagesReady with siteData-driven page generation must not error (issue #525)")
			Expect(result).NotTo(BeNil())

			Expect(result.PageCount).To(Equal(4),
				"1 real page + 3 data-driven virtual pages = 4 total (issue #525)")
			Expect(result.RenderedContent).To(HaveKey("demos/button.md"),
				"data-driven virtual page 'button' must appear in output (issue #525)")
			Expect(result.RenderedContent).To(HaveKey("demos/card.md"),
				"data-driven virtual page 'card' must appear in output (issue #525)")
			Expect(result.RenderedContent).To(HaveKey("demos/dialog.md"),
				"data-driven virtual page 'dialog' must appear in output (issue #525)")
		})
	})

	// ── SetSiteData pipeline wiring (issue #339) ────────────────────
	// Build() must call rt.SetSiteData(siteData) for each plugin runtime
	// after data loading so alloy.data is available in plugins.

	Describe("SetSiteData pipeline wiring", func() {
		It("plugin filter can access site.data via alloy.data during build", func() {
			cfg := &config.Config{
				Title:   "SiteData Wiring Test",
				BaseURL: "https://example.com",
				Build:   config.BuildConfig{Output: "_site"},
			}
			contentMap := map[string]string{
				"content/index.md":        "---\ntitle: Home\nlayout: default\n---\n{{ \"space\" | tokenType }}",
				"data/tokens.json":        `{"space":{"type":"dimension","value":"16px"}}`,
				"plugins/token-reader.js": "export default function(alloy) { alloy.filter('tokenType', function(name) { return alloy.data.tokens[name].type; }); }",
				"layouts/default.liquid":  "<html><body>{{ content }}</body></html>",
			}
			result, err := pipeline.BuildWithContent(cfg, contentMap)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())

			html := result.RenderedContent["index.md"]
			Expect(html).To(ContainSubstring("dimension"),
				"plugin filter must access alloy.data.tokens.space.type — "+
					"proves SetSiteData is called in the pipeline after data loading")
		})
	})

	// ── addPages return shape for onPagesReady (issue #971) ──────────────────
	// onPagesReady supports two return shapes: { pages: [...] } (existing full-array
	// behavior) and { addPages: [...] } (injection-only, no round-trip of existing
	// pages). When pages: false is set in the scope, the payload omits pages from
	// serialization — only siteData is sent. The two return shapes are mutually
	// exclusive. Unrecognized return shapes produce an error.

	Describe("addPages return shape for onPagesReady (issue #971)", func() {

		It("addPages with pages: false injects virtual pages without receiving existing pages", func() {
			cfg := &config.Config{
				Title:   "addPages Injection Test",
				BaseURL: "https://example.com",
				Build:   config.BuildConfig{Output: "_site"},
			}
			contentMap := map[string]string{
				"data/elements.json": `[{"name":"Button","slug":"button"},{"name":"Card","slug":"card"}]`,
				"content/index.md":   "---\ntitle: Home\nlayout: default\n---\n# Home",
				"layouts/default.liquid": "<html><body>{{ content }}</body></html>",
				// Plugin declares pages: false — must NOT receive existing pages in payload.
				// If pages are sent despite pages: false, the plugin throws an error.
				// Returns { addPages: [...] } to inject virtual pages.
				"plugins/add-pages.js": `export default function(alloy) {
  alloy.hook('onPagesReady', { data: ["elements"], pages: false }, function(payload) {
    if (payload.pages && payload.pages.length > 0) {
      throw new Error('pages: false must not send pages to plugin, got ' + payload.pages.length);
    }
    var elements = payload.siteData.elements || [];
    var newPages = [];
    for (var i = 0; i < elements.length; i++) {
      var el = elements[i];
      newPages.push({
        path: 'demos/' + el.slug + '.md',
        url: '/demos/' + el.slug + '/',
        frontMatter: { title: el.name + ' Demo', layout: 'default' },
        content: '# ' + el.name
      });
    }
    return { addPages: newPages };
  });
}`,
			}
			result, err := pipeline.BuildWithContent(cfg, contentMap)
			Expect(err).NotTo(HaveOccurred(),
				"addPages with pages: false must not error — "+
					"if this fails with 'pages: false must not send pages', "+
					"runOnPagesReady is ignoring Pages.Mode and serializing "+
					"all pages even when scope is PagesScopeNone (issue #971)")
			Expect(result).NotTo(BeNil())

			Expect(result.PageCount).To(Equal(3),
				"1 real page + 2 data-driven virtual pages via addPages = 3 total (issue #971)")
			Expect(result.RenderedContent).To(HaveKey("demos/button.md"),
				"virtual page 'button' injected via addPages must appear in output (issue #971)")
			Expect(result.RenderedContent).To(HaveKey("demos/card.md"),
				"virtual page 'card' injected via addPages must appear in output (issue #971)")
		})

		It("addPages virtual pages participate in taxonomy collection", func() {
			renderFalse := false
			cfg := &config.Config{
				Title:   "addPages Taxonomy Test",
				BaseURL: "https://example.com",
				Build:   config.BuildConfig{Output: "_site"},
				Taxonomies: map[string]*config.TaxonomyConfig{
					"tags": {Render: &renderFalse},
				},
			}
			contentMap := map[string]string{
				"content/index.md": "---\ntitle: Home\nlayout: default\ntags: [\"core\"]\n---\n{% for p in taxonomies.tags.component %}<span class=\"vp\">{{ p.title }}</span>{% endfor %}",
				"layouts/default.liquid": "<html><body>{{ content }}</body></html>",
				"plugins/add-tagged.js": `export default function(alloy) {
  alloy.hook('onPagesReady', { pages: false }, function(payload) {
    return { addPages: [
      {
        path: 'demos/accordion.md',
        url: '/demos/accordion/',
        frontMatter: { title: 'Accordion', layout: 'default', tags: ['component'] },
        content: '# Accordion'
      },
      {
        path: 'demos/tabs.md',
        url: '/demos/tabs/',
        frontMatter: { title: 'Tabs', layout: 'default', tags: ['component'] },
        content: '# Tabs'
      }
    ]};
  });
}`,
			}
			result, err := pipeline.BuildWithContent(cfg, contentMap)
			Expect(err).NotTo(HaveOccurred(),
				"addPages with taxonomy terms must not error (issue #971)")
			Expect(result).NotTo(BeNil())

			html := result.RenderedContent["index.md"]
			Expect(html).To(ContainSubstring(`class="vp"`),
				"taxonomies.tags.component must be iterable — "+
					"addPages virtual pages must participate in taxonomy collection (issue #971)")
			Expect(html).To(ContainSubstring("Accordion"),
				"virtual page 'Accordion' tagged 'component' must appear in taxonomy collection (issue #971)")
			Expect(html).To(ContainSubstring("Tabs"),
				"virtual page 'Tabs' tagged 'component' must appear in taxonomy collection (issue #971)")
		})

		It("addPages virtual page content is rendered through the markdown pipeline", func() {
			cfg := &config.Config{
				Title:   "addPages Markdown Test",
				BaseURL: "https://example.com",
				Build:   config.BuildConfig{Output: "_site"},
			}
			contentMap := map[string]string{
				"content/index.md":       "---\ntitle: Home\nlayout: default\n---\n# Home",
				"layouts/default.liquid": "<html><body>{{ content }}</body></html>",
				"plugins/add-md.js": `export default function(alloy) {
  alloy.hook('onPagesReady', { pages: false }, function(payload) {
    return { addPages: [{
      path: 'demos/widget.md',
      url: '/demos/widget/',
      frontMatter: { title: 'Widget', layout: 'default' },
      content: '## Widget Component\n\nA **bold** widget with [a link](https://example.com).'
    }]};
  });
}`,
			}
			result, err := pipeline.BuildWithContent(cfg, contentMap)
			Expect(err).NotTo(HaveOccurred(),
				"addPages with markdown content must not error (issue #971)")
			Expect(result).NotTo(BeNil())

			html := result.RenderedContent["demos/widget.md"]
			Expect(html).To(ContainSubstring("<h2"),
				"markdown ## heading must be rendered to <h2> — "+
					"addPages virtual pages must flow through content rendering (issue #971)")
			Expect(html).To(ContainSubstring("<strong>bold</strong>"),
				"markdown **bold** must be rendered to <strong> — "+
					"raw content from addPages must be processed by goldmark (issue #971)")
		})

		It("addPages works with pages: true for read-then-inject pattern", func() {
			cfg := &config.Config{
				Title:   "addPages Read-Inject Test",
				BaseURL: "https://example.com",
				Build:   config.BuildConfig{Output: "_site"},
			}
			contentMap := map[string]string{
				"content/index.md":       "---\ntitle: Home\nlayout: default\n---\n# Home",
				"content/about.md":      "---\ntitle: About\nlayout: default\n---\n# About",
				"layouts/default.liquid": "<html><body><h1>{{ page.title }}</h1>{{ content }}</body></html>",
				// Plugin with pages: true reads existing pages and uses addPages to inject.
				// This proves addPages is not gated by pages: false — it works with any scope.
				"plugins/read-inject.js": `export default function(alloy) {
  alloy.hook('onPagesReady', { pages: true, pageFields: ["*"] }, function(payload) {
    if (!payload.pages || payload.pages.length === 0) {
      throw new Error('pages: true must send pages to plugin');
    }
    return { addPages: [{
      path: 'generated/sitemap-data.md',
      url: '/sitemap-data/',
      frontMatter: { title: 'Sitemap Data (' + payload.pages.length + ' pages)', layout: 'default' },
      content: '# Sitemap'
    }]};
  });
}`,
			}
			result, err := pipeline.BuildWithContent(cfg, contentMap)
			Expect(err).NotTo(HaveOccurred(),
				"addPages with pages: true must work — addPages is not gated "+
					"by pages: false (issue #971)")
			Expect(result).NotTo(BeNil())

			Expect(result.PageCount).To(Equal(3),
				"2 real pages + 1 addPages virtual page = 3 total (issue #971)")
			Expect(result.RenderedContent).To(HaveKey("generated/sitemap-data.md"),
				"virtual page from addPages with pages: true must appear in output (issue #971)")

			html := result.RenderedContent["generated/sitemap-data.md"]
			Expect(html).To(ContainSubstring("Sitemap Data (2 pages)"),
				"plugin with pages: true must receive existing pages and use the count — "+
					"if title says '0 pages' then pages were not sent despite pages: true (issue #971)")
		})

		It("addPages with empty array is a no-op", func() {
			cfg := &config.Config{
				Title:   "addPages Empty Test",
				BaseURL: "https://example.com",
				Build:   config.BuildConfig{Output: "_site"},
			}
			contentMap := map[string]string{
				"content/index.md":       "---\ntitle: Home\nlayout: default\n---\n# Home",
				"layouts/default.liquid": "<html><body>{{ content }}</body></html>",
				"plugins/empty-add.js": `export default function(alloy) {
  alloy.hook('onPagesReady', { pages: false }, function(payload) {
    return { addPages: [] };
  });
}`,
			}
			result, err := pipeline.BuildWithContent(cfg, contentMap)
			Expect(err).NotTo(HaveOccurred(),
				"addPages with empty array must not error — "+
					"an empty addPages is a valid no-op (issue #971)")
			Expect(result).NotTo(BeNil())

			Expect(result.PageCount).To(Equal(1),
				"no virtual pages added — PageCount must be 1 (issue #971)")
		})

		It("addPages URL collision with existing page produces error", func() {
			cfg := &config.Config{
				Title:   "addPages Collision Test",
				BaseURL: "https://example.com",
				Build:   config.BuildConfig{Output: "_site"},
			}
			contentMap := map[string]string{
				"content/index.md":       "---\ntitle: Home\nlayout: default\n---\n# Home",
				"layouts/default.liquid": "<html><body>{{ content }}</body></html>",
				"plugins/collide-add.js": `export default function(alloy) {
  alloy.hook('onPagesReady', { pages: false }, function(payload) {
    return { addPages: [{
      path: 'virtual-index.md',
      url: '/',
      frontMatter: { title: 'Collision', layout: 'default' },
      content: '# Collision'
    }]};
  });
}`,
			}
			_, err := pipeline.BuildWithContent(cfg, contentMap)
			Expect(err).To(HaveOccurred(),
				"addPages virtual page URL collision with existing page must produce error (issue #971)")
			Expect(err.Error()).To(ContainSubstring("collide"),
				"collision error message must mention the conflict — "+
					"helps plugin authors diagnose which virtual page URL conflicts (issue #971)")
		})

		It("addPages virtual page missing path produces validation error", func() {
			cfg := &config.Config{
				Title:   "addPages Validation Test",
				BaseURL: "https://example.com",
				Build:   config.BuildConfig{Output: "_site"},
			}
			contentMap := map[string]string{
				"content/index.md":       "---\ntitle: Home\nlayout: default\n---\n# Home",
				"layouts/default.liquid": "<html><body>{{ content }}</body></html>",
				"plugins/bad-add.js": `export default function(alloy) {
  alloy.hook('onPagesReady', { pages: false }, function(payload) {
    return { addPages: [{
      frontMatter: { title: 'No Path' },
      content: '# Missing fields'
    }]};
  });
}`,
			}
			_, err := pipeline.BuildWithContent(cfg, contentMap)
			Expect(err).To(HaveOccurred(),
				"addPages virtual page without path/url must produce a validation error — "+
					"same validation as the pages return path (issue #971)")
			Expect(err.Error()).To(ContainSubstring("path"),
				"validation error must mention the missing field (issue #971)")
		})

		It("returning both pages and addPages produces an error", func() {
			cfg := &config.Config{
				Title:   "Both Keys Test",
				BaseURL: "https://example.com",
				Build:   config.BuildConfig{Output: "_site"},
			}
			contentMap := map[string]string{
				"content/index.md":       "---\ntitle: Home\nlayout: default\n---\n# Home",
				"layouts/default.liquid": "<html><body>{{ content }}</body></html>",
				"plugins/both-keys.js": `export default function(alloy) {
  alloy.hook('onPagesReady', { pages: true, pageFields: ["*"] }, function(payload) {
    return {
      pages: payload.pages,
      addPages: [{
        path: 'extra.md',
        url: '/extra/',
        frontMatter: { title: 'Extra', layout: 'default' },
        content: '# Extra'
      }]
    };
  });
}`,
			}
			_, err := pipeline.BuildWithContent(cfg, contentMap)
			Expect(err).To(HaveOccurred(),
				"returning both 'pages' and 'addPages' must produce an error — "+
					"the two return shapes are mutually exclusive (issue #971)")
			Expect(err.Error()).To(ContainSubstring("addPages"),
				"error message must mention 'addPages' so plugin authors know which "+
					"return shape to use (issue #971)")
		})

		It("unrecognized return shape produces an error", func() {
			cfg := &config.Config{
				Title:   "Unrecognized Shape Test",
				BaseURL: "https://example.com",
				Build:   config.BuildConfig{Output: "_site"},
			}
			contentMap := map[string]string{
				"content/index.md":       "---\ntitle: Home\nlayout: default\n---\n# Home",
				"layouts/default.liquid": "<html><body>{{ content }}</body></html>",
				"plugins/wrong-key.js": `export default function(alloy) {
  alloy.hook('onPagesReady', { pages: false }, function(payload) {
    return { newPages: [{
      path: 'demos/button.md',
      url: '/demos/button/',
      frontMatter: { title: 'Button' },
      content: '# Button'
    }]};
  });
}`,
			}
			_, err := pipeline.BuildWithContent(cfg, contentMap)
			Expect(err).To(HaveOccurred(),
				"returning a map with no recognized keys (neither 'pages' nor 'addPages') "+
					"must produce an error — silent no-op causes data loss (issue #971)")
			Expect(err.Error()).To(ContainSubstring("addPages"),
				"error message must mention 'addPages' as one of the expected keys — "+
					"guides the plugin author to the correct return shape (issue #971)")
		})

		It("addPages virtual-to-virtual URL collision within same array produces error", func() {
			cfg := &config.Config{
				Title:   "addPages V2V Collision Test",
				BaseURL: "https://example.com",
				Build:   config.BuildConfig{Output: "_site"},
			}
			contentMap := map[string]string{
				"content/index.md":       "---\ntitle: Home\nlayout: default\n---\n# Home",
				"layouts/default.liquid": "<html><body>{{ content }}</body></html>",
				"plugins/v2v-collide.js": `export default function(alloy) {
  alloy.hook('onPagesReady', { pages: false }, function(payload) {
    return { addPages: [
      { path: 'demos/button-a.md', url: '/demos/button/', frontMatter: { title: 'Button A', layout: 'default' }, content: '# A' },
      { path: 'demos/button-b.md', url: '/demos/button/', frontMatter: { title: 'Button B', layout: 'default' }, content: '# B' }
    ]};
  });
}`,
			}
			_, err := pipeline.BuildWithContent(cfg, contentMap)
			Expect(err).To(HaveOccurred(),
				"two addPages entries sharing the same URL must produce a collision error — "+
					"the urlIndex must be updated per entry, not just seeded from existing pages (issue #971)")
			Expect(err.Error()).To(ContainSubstring("collide"),
				"collision error must mention the conflict (issue #971)")
		})

		It("non-array addPages value produces an error", func() {
			cfg := &config.Config{
				Title:   "addPages Type Error Test",
				BaseURL: "https://example.com",
				Build:   config.BuildConfig{Output: "_site"},
			}
			contentMap := map[string]string{
				"content/index.md":       "---\ntitle: Home\nlayout: default\n---\n# Home",
				"layouts/default.liquid": "<html><body>{{ content }}</body></html>",
				"plugins/bad-type.js": `export default function(alloy) {
  alloy.hook('onPagesReady', { pages: false }, function(payload) {
    return { addPages: "not-an-array" };
  });
}`,
			}
			_, err := pipeline.BuildWithContent(cfg, contentMap)
			Expect(err).To(HaveOccurred(),
				"addPages with a non-array value must produce an error — "+
					"string, number, or object values are not valid page lists (issue #971)")
		})

		It("addPages virtual page missing url (but with path) produces validation error", func() {
			cfg := &config.Config{
				Title:   "addPages Missing URL Test",
				BaseURL: "https://example.com",
				Build:   config.BuildConfig{Output: "_site"},
			}
			contentMap := map[string]string{
				"content/index.md":       "---\ntitle: Home\nlayout: default\n---\n# Home",
				"layouts/default.liquid": "<html><body>{{ content }}</body></html>",
				"plugins/no-url.js": `export default function(alloy) {
  alloy.hook('onPagesReady', { pages: false }, function(payload) {
    return { addPages: [{
      path: 'demos/button.md',
      frontMatter: { title: 'No URL' },
      content: '# Missing URL'
    }]};
  });
}`,
			}
			_, err := pipeline.BuildWithContent(cfg, contentMap)
			Expect(err).To(HaveOccurred(),
				"addPages virtual page with path but no url must produce a validation error — "+
					"both fields are independently required (issue #971)")
			Expect(err.Error()).To(ContainSubstring("url"),
				"validation error must mention the missing url field (issue #971)")
		})

		It("hook with pages: false returning payload unchanged is a no-op", func() {
			cfg := &config.Config{
				Title:   "addPages Echo Payload Test",
				BaseURL: "https://example.com",
				Build:   config.BuildConfig{Output: "_site"},
			}
			contentMap := map[string]string{
				"content/index.md":       "---\ntitle: Home\nlayout: default\n---\n# Home",
				"layouts/default.liquid": "<html><body>{{ content }}</body></html>",
				// A side-effect-only hook naturally does `return payload`.
				// With pages: false, payload is {pages: null, siteData: {...}}.
				// The null-valued "pages" key must be treated as absent, not
				// as the full-array "pages" return shape.
				"plugins/echo.js": `export default function(alloy) {
  alloy.hook('onPagesReady', { pages: false }, function(payload) {
    return payload;
  });
}`,
			}
			result, err := pipeline.BuildWithContent(cfg, contentMap)
			Expect(err).NotTo(HaveOccurred(),
				"hook with pages: false returning payload unchanged must be a no-op — "+
					"payload contains {pages: null, siteData: {...}}, and a null-valued "+
					"'pages' key must be treated as absent (issue #971)")
			Expect(result).NotTo(BeNil())
			Expect(result.PageCount).To(Equal(1),
				"echo payload must not add or remove pages (issue #971)")
		})

		It("multiple hooks returning addPages both inject their virtual pages", func() {
			cfg := &config.Config{
				Title:   "addPages Multi-Hook Test",
				BaseURL: "https://example.com",
				Build:   config.BuildConfig{Output: "_site"},
			}
			contentMap := map[string]string{
				"content/index.md":       "---\ntitle: Home\nlayout: default\n---\n# Home",
				"layouts/default.liquid": "<html><body>{{ content }}</body></html>",
				// Two plugins both register onPagesReady with addPages.
				// Each hook must receive the canonical {pages, siteData} payload,
				// NOT the previous hook's return value. RunWithTimeout chains
				// results (current = result), but addPages return shape != payload
				// shape, so the pipeline must invoke hooks individually and rebuild
				// the canonical payload between invocations.
				"plugins/hook-a.js": `export default function(alloy) {
  alloy.hook('onPagesReady', { data: ["*"], pages: false, priority: 10 }, function(payload) {
    if (!payload.siteData) {
      throw new Error('hook A: siteData missing — received previous hook return instead of canonical payload');
    }
    return { addPages: [{
      path: 'from-a.md',
      url: '/from-a/',
      frontMatter: { title: 'From Hook A', layout: 'default' },
      content: '# From A'
    }]};
  });
}`,
				"plugins/hook-b.js": `export default function(alloy) {
  alloy.hook('onPagesReady', { data: ["*"], pages: false, priority: 50 }, function(payload) {
    if (!payload.siteData) {
      throw new Error('hook B: siteData missing — received previous hook return instead of canonical payload');
    }
    return { addPages: [{
      path: 'from-b.md',
      url: '/from-b/',
      frontMatter: { title: 'From Hook B', layout: 'default' },
      content: '# From B'
    }]};
  });
}`,
			}
			result, err := pipeline.BuildWithContent(cfg, contentMap)
			Expect(err).NotTo(HaveOccurred(),
				"multiple hooks returning addPages must not error — "+
					"each hook receives the canonical payload, not the previous "+
					"hook's return value (issue #971)")
			Expect(result).NotTo(BeNil())

			Expect(result.PageCount).To(Equal(3),
				"1 real + 1 from hook A + 1 from hook B = 3 total (issue #971)")
			Expect(result.RenderedContent).To(HaveKey("from-a.md"),
				"virtual page from hook A must appear in output (issue #971)")
			Expect(result.RenderedContent).To(HaveKey("from-b.md"),
				"virtual page from hook B must appear in output (issue #971)")
		})
	})

	// ── onConfig return value applied to pipeline config (issue #973) ────────
	// onConfig is documented as mutable ("Plugin mutates config") but the
	// return value is silently discarded at the Build() call site. The pipeline
	// must apply the returned config back to cfg for the mutable allowlist:
	//   build.output, build.clean, structure.content, structure.layouts,
	//   structure.assets, structure.static, structure.data, passthrough,
	//   plugins.workers, plugins.timeout
	// Fields outside the allowlist (e.g. title, baseURL) are NOT applied.
	// Non-object returns produce a build error.

	Describe("onConfig return value applied to pipeline config (issue #973)", func() {

		It("plugin can change build.output via onConfig and pipeline writes to the new directory", func() {
			cfg := &config.Config{
				Title:   "Config Mutation Test",
				BaseURL: "https://example.com",
				Build:   config.BuildConfig{Output: "_site"},
			}
			contentMap := map[string]string{
				"content/index.md":       "---\ntitle: Home\nlayout: default\n---\n# Home",
				"layouts/default.liquid": "<html><body>{{ content }}</body></html>",
				"plugins/change-output.js": `export default function(alloy) {
  alloy.hook('onConfig', {}, (config) => {
    config.build.output = "custom_dist";
    return config;
  });
}`,
			}
			result, err := pipeline.BuildWithContent(cfg, contentMap)
			Expect(err).NotTo(HaveOccurred(),
				"onConfig hook must not error when returning modified config (issue #973)")
			Expect(result).NotTo(BeNil())
			Expect(result.OutputDir).To(Equal("custom_dist"),
				"onConfig return value must be applied — build.output should be "+
					"'custom_dist', not the original '_site'. Currently the return "+
					"value is discarded at the Build() call site (issue #973)")
		})

		It("plugin can redirect structure.content via onConfig", func() {
			cfg := &config.Config{
				Title:   "Content Redirect Test",
				BaseURL: "https://example.com",
				Build:   config.BuildConfig{Output: "_site"},
			}
			// Content files are under "pages/", not the default "content/".
			// Without the onConfig mutation, the pipeline looks in "content/"
			// and finds nothing → 0 pages.
			contentMap := map[string]string{
				"pages/index.md":         "---\ntitle: From Pages Dir\nlayout: default\n---\n# From Pages Dir",
				"layouts/default.liquid": "<html><body>{{ content }}</body></html>",
				"plugins/redirect-content.js": `export default function(alloy) {
  alloy.hook('onConfig', {}, (config) => {
    config.structure.content = "pages";
    return config;
  });
}`,
			}
			result, err := pipeline.BuildWithContent(cfg, contentMap)
			Expect(err).NotTo(HaveOccurred(),
				"onConfig structure.content redirection must not error (issue #973)")
			Expect(result).NotTo(BeNil())
			Expect(result.PageCount).To(Equal(1),
				"structure.content redirected to 'pages/' — pipeline must find "+
					"index.md there. If PageCount is 0, the onConfig mutation "+
					"was not applied and the pipeline looked in 'content/' (issue #973)")

			html := result.RenderedContent["index.md"]
			Expect(html).To(ContainSubstring("From Pages Dir"),
				"content from the redirected directory must be rendered (issue #973)")
		})

		It("plugin can redirect structure.layouts via onConfig", func() {
			cfg := &config.Config{
				Title:   "Layouts Redirect Test",
				BaseURL: "https://example.com",
				Build:   config.BuildConfig{Output: "_site"},
			}
			// Both "layouts/" and "templates/" have default.liquid with different markers.
			// If the onConfig mutation is applied, the pipeline uses "templates/" → "REDIRECTED".
			// If not applied, it uses "layouts/" → "ORIGINAL".
			contentMap := map[string]string{
				"content/index.md":            "---\ntitle: Home\nlayout: default\n---\n# Home",
				"layouts/default.liquid":      "<html><body>ORIGINAL_LAYOUT {{ content }}</body></html>",
				"templates/default.liquid":    "<html><body>REDIRECTED_LAYOUT {{ content }}</body></html>",
				"plugins/redirect-layouts.js": `export default function(alloy) {
  alloy.hook('onConfig', {}, (config) => {
    config.structure.layouts = "templates";
    return config;
  });
}`,
			}
			result, err := pipeline.BuildWithContent(cfg, contentMap)
			Expect(err).NotTo(HaveOccurred(),
				"onConfig structure.layouts redirection must not error (issue #973)")
			Expect(result).NotTo(BeNil())

			html := result.RenderedContent["index.md"]
			Expect(html).To(ContainSubstring("REDIRECTED_LAYOUT"),
				"structure.layouts redirected to 'templates/' — pipeline must use "+
					"layouts from the new directory. If output contains 'ORIGINAL_LAYOUT', "+
					"the onConfig mutation was not applied (issue #973)")
			Expect(html).NotTo(ContainSubstring("ORIGINAL_LAYOUT"),
				"layouts from the original directory must NOT be used after "+
					"structure.layouts is redirected via onConfig (issue #973)")
		})

		It("onConfig mutations outside the mutable allowlist are not applied", func() {
			cfg := &config.Config{
				Title:   "Original Title",
				BaseURL: "https://example.com",
				Build:   config.BuildConfig{Output: "_site"},
			}
			// Plugin attempts to change title (not in the mutable allowlist).
			// site.title in templates must still show the original value.
			contentMap := map[string]string{
				"content/index.md":       "---\ntitle: Home\nlayout: default\n---\n# Home",
				"layouts/default.liquid": "<html><body><span class=\"site-title\">{{ site.title }}</span>{{ content }}</body></html>",
				"plugins/change-title.js": `export default function(alloy) {
  alloy.hook('onConfig', {}, (config) => {
    config.title = "Mutated Title";
    return config;
  });
}`,
			}
			result, err := pipeline.BuildWithContent(cfg, contentMap)
			Expect(err).NotTo(HaveOccurred(),
				"onConfig with immutable field mutation must not error — "+
					"immutable changes are silently ignored or produce a warning, "+
					"not an error (issue #973)")
			Expect(result).NotTo(BeNil())

			html := result.RenderedContent["index.md"]
			Expect(html).To(ContainSubstring("Original Title"),
				"site.title must still be 'Original Title' — title is outside "+
					"the mutable allowlist and must not be applied back to cfg (issue #973)")
			Expect(html).NotTo(ContainSubstring("Mutated Title"),
				"mutated title must NOT appear in output — if it does, the "+
					"pipeline is applying all returned fields without filtering "+
					"through the mutable allowlist (issue #973)")
		})

		It("multiple onConfig hooks from separate plugins chain mutations correctly", func() {
			cfg := &config.Config{
				Title:   "Chain Test",
				BaseURL: "https://example.com",
				Build:   config.BuildConfig{Output: "_site"},
			}
			// Two separate plugins each register an onConfig hook.
			// The bridge uses hooks[name] = fn per plugin (one handler per event per plugin),
			// but the hook registry chains results across plugins in priority order.
			// Plugin A (priority 10) runs first, sets build.output to "step_one".
			// Plugin B (priority 50) runs second, receives the mutated config and appends "_step_two".
			// The final result must be "step_one_step_two".
			contentMap := map[string]string{
				"content/index.md":       "---\ntitle: Home\nlayout: default\n---\n# Home",
				"layouts/default.liquid": "<html><body>{{ content }}</body></html>",
				"plugins/aaa-step-one.js": `export default function(alloy) {
  alloy.hook('onConfig', { priority: 10 }, (config) => {
    config.build.output = "step_one";
    return config;
  });
}`,
				"plugins/bbb-step-two.js": `export default function(alloy) {
  alloy.hook('onConfig', { priority: 50 }, (config) => {
    config.build.output = config.build.output + "_step_two";
    return config;
  });
}`,
			}
			result, err := pipeline.BuildWithContent(cfg, contentMap)
			Expect(err).NotTo(HaveOccurred(),
				"chained onConfig hooks from separate plugins must not error (issue #973)")
			Expect(result).NotTo(BeNil())
			Expect(result.OutputDir).To(Equal("step_one_step_two"),
				"multiple onConfig hooks must chain — plugin A sets 'step_one', "+
					"plugin B appends '_step_two'. If OutputDir is '_site', "+
					"neither hook's return was applied. If 'step_one', only the "+
					"last hook's return was applied without chaining (issue #973)")
		})

		It("onConfig hook returning a non-object produces a build error", func() {
			cfg := &config.Config{
				Title:   "Non-Object Return Test",
				BaseURL: "https://example.com",
				Build:   config.BuildConfig{Output: "_site"},
			}
			// Plugin returns a string instead of a config object.
			// The pipeline must reject this with an error, not silently ignore it.
			contentMap := map[string]string{
				"content/index.md":       "---\ntitle: Home\nlayout: default\n---\n# Home",
				"layouts/default.liquid": "<html><body>{{ content }}</body></html>",
				"plugins/bad-return.js": `export default function(alloy) {
  alloy.hook('onConfig', {}, (config) => {
    return "not-a-config-object";
  });
}`,
			}
			_, err := pipeline.BuildWithContent(cfg, contentMap)
			Expect(err).To(HaveOccurred(),
				"onConfig returning a non-object must produce a build error — "+
					"the pipeline must type-check the return value and reject "+
					"strings, numbers, arrays, and null. Currently the return "+
					"is discarded silently (issue #973)")
			Expect(err.Error()).To(ContainSubstring("onConfig"),
				"error message must identify onConfig as the source — "+
					"helps plugin authors locate the problematic hook (issue #973)")
		})

		// ── Remaining mutable allowlist fields (issue #999) ────────────────
		// PR #997 covered build.output, structure.content, structure.layouts,
		// title exclusion, chaining, and non-object error. The following tests
		// cover the remaining mutable allowlist fields and edge cases.

		It("plugin can set build.clean to false via onConfig and chained hook sees the mutation", func() {
			cfg := &config.Config{
				Title:   "Clean Mutation Test",
				BaseURL: "https://example.com",
				Build:   config.BuildConfig{Output: "_site"},
			}
			// Two plugins in a chain: first sets build.clean = false,
			// second verifies it was applied by throwing if clean is not false.
			// This proves the bool → *bool serialization is correct in the
			// hook chain and that build.clean is in the mutable allowlist.
			contentMap := map[string]string{
				"content/index.md":       "---\ntitle: Home\nlayout: default\n---\n# Home",
				"layouts/default.liquid": "<html><body>{{ content }}</body></html>",
				"plugins/aaa-set-clean.js": `export default function(alloy) {
  alloy.hook('onConfig', { priority: 10 }, (config) => {
    config.build.clean = false;
    return config;
  });
}`,
				"plugins/bbb-verify-clean.js": `export default function(alloy) {
  alloy.hook('onConfig', { priority: 50 }, (config) => {
    if (config.build.clean !== false) {
      throw new Error('build.clean was not applied: expected false, got ' + JSON.stringify(config.build.clean));
    }
    return config;
  });
}`,
			}
			result, err := pipeline.BuildWithContent(cfg, contentMap)
			Expect(err).NotTo(HaveOccurred(),
				"onConfig must support build.clean mutation — if this fails with "+
					"'build.clean was not applied', the field is not in the mutable "+
					"allowlist or the bool→*bool serialization is broken (issue #999)")
			Expect(result).NotTo(BeNil())
			Expect(result.PageCount).To(Equal(1),
				"build must complete with clean=false mutation (issue #999)")
		})

		It("plugin can redirect structure.assets via onConfig and asset filters resolve from the new directory", func() {
			cfg := &config.Config{
				Title:   "Assets Redirect Test",
				BaseURL: "https://example.com",
				Build:   config.BuildConfig{Output: "_site"},
			}
			// Put a CSS file in "my_assets/" (not the default "assets/").
			// The plugin redirects structure.assets to "my_assets".
			// The template uses {{ "style.css" | cachebust }} which resolves
			// asset files from the configured assets directory.
			// If the mutation is applied, cachebust finds the file and appends ?h=<hash>.
			// If not applied, cachebust looks in "assets/" (empty), fails, and returns "/style.css".
			contentMap := map[string]string{
				"content/index.md":       "---\ntitle: Home\nlayout: default\n---\n# Home",
				"layouts/default.liquid": `<html><body>{{ content }}<link href="{{ "style.css" | cachebust }}" rel="stylesheet"></body></html>`,
				"my_assets/style.css":    "body { color: red; }",
				"plugins/redirect-assets.js": `export default function(alloy) {
  alloy.hook('onConfig', {}, (config) => {
    config.structure.assets = "my_assets";
    return config;
  });
}`,
			}
			result, err := pipeline.BuildWithContent(cfg, contentMap)
			Expect(err).NotTo(HaveOccurred(),
				"onConfig structure.assets redirection must not error (issue #999)")
			Expect(result).NotTo(BeNil())

			html := result.RenderedContent["index.md"]
			Expect(html).To(ContainSubstring("?h="),
				"cachebust must find style.css in the redirected 'my_assets/' directory "+
					"and append a content hash. If output contains '/style.css' without ?h=, "+
					"the structure.assets mutation was not applied and the pipeline looked "+
					"in the default 'assets/' directory (issue #999)")
		})

		It("plugin can redirect structure.static via onConfig and static files are resolved from the new directory", func() {
			cfg := &config.Config{
				Title:   "Static Redirect Test",
				BaseURL: "https://example.com",
				Build:   config.BuildConfig{Output: "_site"},
			}
			// Put a static file in "public/" (not the default "static/").
			// The plugin redirects structure.static to "public".
			// The template uses {{ "robots.txt" | cachebust }} which resolves
			// files from the configured static directory.
			// If the mutation is applied, cachebust finds the file and appends ?h=<hash>.
			// If not applied, it looks in "static/" (empty) and returns "/robots.txt".
			contentMap := map[string]string{
				"content/index.md":       "---\ntitle: Home\nlayout: default\n---\n# Home",
				"layouts/default.liquid": `<html><body>{{ content }}<a href="{{ "robots.txt" | cachebust }}">robots</a></body></html>`,
				"public/robots.txt":      "User-agent: *\nDisallow: /admin/",
				"plugins/redirect-static.js": `export default function(alloy) {
  alloy.hook('onConfig', {}, (config) => {
    config.structure.static = "public";
    return config;
  });
}`,
			}
			result, err := pipeline.BuildWithContent(cfg, contentMap)
			Expect(err).NotTo(HaveOccurred(),
				"onConfig structure.static redirection must not error (issue #999)")
			Expect(result).NotTo(BeNil())

			html := result.RenderedContent["index.md"]
			Expect(html).To(ContainSubstring("?h="),
				"cachebust must find robots.txt in the redirected 'public/' directory "+
					"and append a content hash. If output contains '/robots.txt' without ?h=, "+
					"the structure.static mutation was not applied and the pipeline looked "+
					"in the default 'static/' directory (issue #999)")
		})

		It("plugin can redirect structure.data via onConfig and data loads from the new directory", func() {
			cfg := &config.Config{
				Title:   "Data Redirect Test",
				BaseURL: "https://example.com",
				Build:   config.BuildConfig{Output: "_site"},
			}
			// Data file is in "my_data/" (not the default "data/").
			// Without the onConfig mutation, the pipeline looks in "data/"
			// and finds nothing → site.data.info is nil → template outputs nothing.
			contentMap := map[string]string{
				"content/index.md":       "---\ntitle: Home\nlayout: default\n---\n# Home",
				"layouts/default.liquid": `<html><body>{{ content }}<span class="site-name">{{ site.data.info.name }}</span></body></html>`,
				"my_data/info.json":      `{"name":"Redirected Data Works"}`,
				"plugins/redirect-data.js": `export default function(alloy) {
  alloy.hook('onConfig', {}, (config) => {
    config.structure.data = "my_data";
    return config;
  });
}`,
			}
			result, err := pipeline.BuildWithContent(cfg, contentMap)
			Expect(err).NotTo(HaveOccurred(),
				"onConfig structure.data redirection must not error (issue #999)")
			Expect(result).NotTo(BeNil())

			html := result.RenderedContent["index.md"]
			Expect(html).To(ContainSubstring("Redirected Data Works"),
				"structure.data redirected to 'my_data/' — pipeline must load data "+
					"from the new directory. If site.data.info.name is empty, the "+
					"onConfig mutation was not applied and the pipeline looked in "+
					"'data/' (issue #999)")
		})

		It("plugin can set passthrough mappings via onConfig", func() {
			cfg := &config.Config{
				Title:   "Passthrough Mutation Test",
				BaseURL: "https://example.com",
				Build:   config.BuildConfig{Output: "_site"},
			}
			// Plugin adds a passthrough mapping via onConfig.
			// The pipeline should process the passthrough after onConfig applies it.
			// A chained hook verifies the passthrough was set correctly.
			contentMap := map[string]string{
				"content/index.md":       "---\ntitle: Home\nlayout: default\n---\n# Home",
				"layouts/default.liquid": "<html><body>{{ content }}</body></html>",
				// Source directories that the passthrough mappings reference must
				// exist in the temp dir — Build() copies them during Phase 3.
				"vendor/fonts/bold.woff2":          "fake-font-data",
				"node_modules/icons/dist/icon.svg": "<svg/>",
				"node_modules/icons/dist/icon.svg.map": "source-map-data",
				"plugins/aaa-set-passthrough.js": `export default function(alloy) {
  alloy.hook('onConfig', { priority: 10 }, (config) => {
    config.passthrough = [
      { from: "vendor/fonts", to: "assets/fonts" },
      { from: "node_modules/icons/dist", to: "assets/icons", exclude: ["*.map"] }
    ];
    return config;
  });
}`,
				"plugins/bbb-verify-passthrough.js": `export default function(alloy) {
  alloy.hook('onConfig', { priority: 50 }, (config) => {
    if (!config.passthrough || !Array.isArray(config.passthrough)) {
      throw new Error('passthrough was not set: got ' + JSON.stringify(config.passthrough));
    }
    if (config.passthrough.length !== 2) {
      throw new Error('expected 2 passthrough entries, got ' + config.passthrough.length);
    }
    if (config.passthrough[0].from !== 'vendor/fonts') {
      throw new Error('passthrough[0].from mismatch: ' + config.passthrough[0].from);
    }
    if (config.passthrough[1].to !== 'assets/icons') {
      throw new Error('passthrough[1].to mismatch: ' + config.passthrough[1].to);
    }
    if (!config.passthrough[1].exclude || config.passthrough[1].exclude[0] !== '*.map') {
      throw new Error('passthrough[1].exclude mismatch: ' + JSON.stringify(config.passthrough[1].exclude));
    }
    return config;
  });
}`,
			}
			result, err := pipeline.BuildWithContent(cfg, contentMap)
			Expect(err).NotTo(HaveOccurred(),
				"onConfig must support passthrough mutation — if this fails with a "+
					"verification error from bbb-verify-passthrough.js, the passthrough "+
					"array was not correctly deserialized or applied (issue #999)")
			Expect(result).NotTo(BeNil())
		})

		It("passthrough entries with empty from are skipped during onConfig application", func() {
			cfg := &config.Config{
				Title:   "Passthrough Empty From Test",
				BaseURL: "https://example.com",
				Build:   config.BuildConfig{Output: "_site"},
			}
			// Plugin sets passthrough with one valid and one empty-from entry.
			// The empty-from entry must be filtered out — only the valid entry
			// should be applied.
			contentMap := map[string]string{
				"content/index.md":       "---\ntitle: Home\nlayout: default\n---\n# Home",
				"layouts/default.liquid": "<html><body>{{ content }}</body></html>",
				// Source directory for the valid passthrough entry must exist.
				"vendor/valid/data.txt": "valid-passthrough-content",
				"plugins/aaa-set-passthrough.js": `export default function(alloy) {
  alloy.hook('onConfig', { priority: 10 }, (config) => {
    config.passthrough = [
      { from: "", to: "should-be-skipped" },
      { from: "vendor/valid", to: "assets/valid" }
    ];
    return config;
  });
}`,
				"plugins/bbb-verify-passthrough.js": `export default function(alloy) {
  alloy.hook('onConfig', { priority: 50 }, (config) => {
    // After Go-side application and re-serialization, the empty-from
    // entry should have been filtered out. But in the JS hook chain,
    // both entries are visible (filtering happens at applyOnConfigResult).
    // This test verifies the hook chain receives what was set.
    if (!config.passthrough || config.passthrough.length !== 2) {
      throw new Error('hook chain should see both entries (filtering is Go-side): got ' + JSON.stringify(config.passthrough));
    }
    return config;
  });
}`,
			}
			result, err := pipeline.BuildWithContent(cfg, contentMap)
			Expect(err).NotTo(HaveOccurred(),
				"passthrough with empty-from entries must not error — "+
					"empty-from entries are silently skipped during Go-side "+
					"application (issue #999)")
			Expect(result).NotTo(BeNil())
		})

		It("plugin can set plugins.workers via onConfig and chained hook sees the mutation", func() {
			cfg := &config.Config{
				Title:   "Workers Mutation Test",
				BaseURL: "https://example.com",
				Build:   config.BuildConfig{Output: "_site"},
			}
			// Plugin sets plugins.workers to 2. The chained hook verifies the
			// value was applied. The pipeline uses this value to size the
			// worker pool (after onConfig, before batch hooks).
			contentMap := map[string]string{
				"content/index.md":       "---\ntitle: Home\nlayout: default\n---\n# Home",
				"layouts/default.liquid": "<html><body>{{ content }}</body></html>",
				"plugins/aaa-set-workers.js": `export default function(alloy) {
  alloy.hook('onConfig', { priority: 10 }, (config) => {
    config.plugins.workers = 2;
    return config;
  });
}`,
				"plugins/bbb-verify-workers.js": `export default function(alloy) {
  alloy.hook('onConfig', { priority: 50 }, (config) => {
    if (config.plugins.workers !== 2) {
      throw new Error('plugins.workers was not applied: expected 2, got ' + JSON.stringify(config.plugins.workers));
    }
    return config;
  });
}`,
			}
			result, err := pipeline.BuildWithContent(cfg, contentMap)
			Expect(err).NotTo(HaveOccurred(),
				"onConfig must support plugins.workers mutation — if this fails "+
					"with 'plugins.workers was not applied', the field is not in the "+
					"mutable allowlist (issue #999)")
			Expect(result).NotTo(BeNil())
			Expect(result.PageCount).To(Equal(1),
				"build must complete with workers=2 mutation (issue #999)")
		})

		It("plugin can set plugins.timeout via onConfig and chained hook sees the mutation", func() {
			cfg := &config.Config{
				Title:   "Timeout Mutation Test",
				BaseURL: "https://example.com",
				Build:   config.BuildConfig{Output: "_site"},
			}
			// Plugin A (onConfig) reduces timeout to 250.
			// Plugin B (onConfig, lower priority) verifies the value was applied
			// by throwing if it doesn't match. This proves plugins.timeout is in
			// the mutable allowlist and flows through the hook chain.
			//
			// NOTE: This test verifies the onConfig mutation pathway but does NOT
			// trigger an actual timeout scenario. Triggering a timeout creates an
			// orphaned goroutine inside the QuickJS WASM engine that races with
			// registry.Close() at Build() return, causing a WASM trap.
			// QuickJSRuntime needs a mutex to make Close() safe against in-flight
			// operations (issue #1025). Timeout enforcement itself is covered by
			// unit tests in internal/plugin/hooks_test.go.
			contentMap := map[string]string{
				"content/index.md":       "---\ntitle: Home\nlayout: default\n---\n# Home",
				"layouts/default.liquid": "<html><body>{{ content }}</body></html>",
				"plugins/aaa-reduce-timeout.js": `export default function(alloy) {
  alloy.hook('onConfig', { priority: 10 }, (config) => {
    config.plugins.timeout = 250;
    return config;
  });
}`,
				"plugins/bbb-verify-timeout.js": `export default function(alloy) {
  alloy.hook('onConfig', { priority: 50 }, (config) => {
    if (config.plugins.timeout !== 250) {
      throw new Error('plugins.timeout was not applied: expected 250, got ' + JSON.stringify(config.plugins.timeout));
    }
    return config;
  });
}`,
			}
			result, err := pipeline.BuildWithContent(cfg, contentMap)
			Expect(err).NotTo(HaveOccurred(),
				"onConfig must support plugins.timeout mutation — if this fails "+
					"with 'plugins.timeout was not applied', the field is not in the "+
					"mutable allowlist or the int serialization is broken (issue #999)")
			Expect(result).NotTo(BeNil())
			Expect(result.PageCount).To(Equal(1),
				"build must complete with timeout mutation (issue #999)")
		})

		// ── Edge cases for onConfig (issue #999) ──────────────────────────

		It("onConfig hook returning null/nil produces a build error", func() {
			cfg := &config.Config{
				Title:   "Nil Return Test",
				BaseURL: "https://example.com",
				Build:   config.BuildConfig{Output: "_site"},
			}
			contentMap := map[string]string{
				"content/index.md":       "---\ntitle: Home\nlayout: default\n---\n# Home",
				"layouts/default.liquid": "<html><body>{{ content }}</body></html>",
				"plugins/null-return.js": `export default function(alloy) {
  alloy.hook('onConfig', {}, (config) => {
    return null;
  });
}`,
			}
			_, err := pipeline.BuildWithContent(cfg, contentMap)
			Expect(err).To(HaveOccurred(),
				"onConfig returning null must produce a build error — "+
					"null return means the plugin forgot to return the config "+
					"object, causing silent data loss (issue #999)")
			Expect(err.Error()).To(ContainSubstring("onConfig"),
				"error message must identify onConfig as the source (issue #999)")
		})

		It("applyOnConfigResult is a no-op when it receives the original *config.Config (timeout case)", func() {
			// When an onConfig hook times out, RunWithTimeout returns the
			// pre-hook payload — the original *config.Config pointer.
			// applyOnConfigResult must detect *config.Config and return nil
			// (no-op), preserving the original config unchanged.
			//
			// This test exercises the no-mutation path: a hook that returns
			// the config object with no changes. The pipeline treats this
			// identically to the timeout case (a map whose values match the
			// original config produces no effective mutation).
			//
			// NOTE: A full timeout-triggered test is deferred until
			// QuickJSRuntime has a mutex (issue #1025). Without it,
			// triggering a timeout on a WASM-backed hook creates an orphaned
			// goroutine that races with registry.Close().
			cfg := &config.Config{
				Title:   "Timeout Preservation Test",
				BaseURL: "https://example.com",
				Build:   config.BuildConfig{Output: "_site"},
			}
			contentMap := map[string]string{
				"content/index.md":       "---\ntitle: Home\nlayout: default\n---\n# Home",
				"layouts/default.liquid": "<html><body>{{ content }}</body></html>",
				"plugins/noop-config.js": `export default function(alloy) {
  alloy.hook('onConfig', {}, (config) => {
    return config;
  });
}`,
			}
			result, err := pipeline.BuildWithContent(cfg, contentMap)
			Expect(err).NotTo(HaveOccurred(),
				"onConfig hook returning unmodified config must not error (issue #999)")
			Expect(result).NotTo(BeNil())
			Expect(result.OutputDir).To(Equal("_site"),
				"onConfig hook that does not mutate must preserve the original "+
					"config — OutputDir must remain '_site' (issue #999)")
		})

		It("onConfig setting plugins.timeout to zero preserves the original timeout", func() {
			cfg := &config.Config{
				Title:   "Zero Timeout Test",
				BaseURL: "https://example.com",
				Build:   config.BuildConfig{Output: "_site"},
				Plugins: config.PluginsConfig{Timeout: 500}, // explicit baseline
			}
			// Plugin sets plugins.timeout to 0. The implementation only applies
			// timeout values > 0, so 0 should be treated as "keep original".
			// The verification hook runs a 25ms delay — if 0 is incorrectly
			// applied as a literal 0ms timeout, the hook always times out.
			// With the correct 500ms baseline preserved, 25ms completes easily.
			contentMap := map[string]string{
				"content/index.md":       "---\ntitle: Home\nlayout: default\n---\n# Home",
				"layouts/default.liquid": "<html><body>{{ content }}</body></html>",
				"plugins/aaa-zero-timeout.js": `export default function(alloy) {
  alloy.hook('onConfig', { priority: 10 }, (config) => {
    config.plugins.timeout = 0;
    return config;
  });
}`,
				"plugins/bbb-verify-hook-runs.js": `export default function(alloy) {
  alloy.hook('onContentTransformed', {}, (page) => {
    var start = Date.now();
    while (Date.now() - start < 25) {}
    page.html = page.html + '<!-- HOOK_RAN -->';
    return page;
  });
}`,
			}
			result, err := pipeline.BuildWithContent(cfg, contentMap)
			Expect(err).NotTo(HaveOccurred(),
				"plugins.timeout=0 must not cause a build error — zero is "+
					"treated as 'keep original timeout' (issue #999)")
			Expect(result).NotTo(BeNil())

			html := result.RenderedContent["index.md"]
			Expect(html).To(ContainSubstring("HOOK_RAN"),
				"subsequent hooks must still run under the original timeout — "+
					"if HOOK_RAN is missing, setting timeout=0 may have been applied "+
					"as a literal 0ms timeout, causing all hooks to time out (issue #999)")
		})

		It("onConfig setting plugins.timeout to negative preserves the original timeout", func() {
			cfg := &config.Config{
				Title:   "Negative Timeout Test",
				BaseURL: "https://example.com",
				Build:   config.BuildConfig{Output: "_site"},
				Plugins: config.PluginsConfig{Timeout: 500}, // explicit baseline
			}
			// Plugin sets plugins.timeout to -1. Like zero, negative values
			// must not be applied — the original timeout is preserved.
			// The verification hook runs a 25ms delay — if -1 is incorrectly
			// applied, the hook times out. With 500ms baseline, 25ms completes.
			contentMap := map[string]string{
				"content/index.md":       "---\ntitle: Home\nlayout: default\n---\n# Home",
				"layouts/default.liquid": "<html><body>{{ content }}</body></html>",
				"plugins/aaa-negative-timeout.js": `export default function(alloy) {
  alloy.hook('onConfig', { priority: 10 }, (config) => {
    config.plugins.timeout = -1;
    return config;
  });
}`,
				"plugins/bbb-verify-hook-runs.js": `export default function(alloy) {
  alloy.hook('onContentTransformed', {}, (page) => {
    var start = Date.now();
    while (Date.now() - start < 25) {}
    page.html = page.html + '<!-- HOOK_RAN -->';
    return page;
  });
}`,
			}
			result, err := pipeline.BuildWithContent(cfg, contentMap)
			Expect(err).NotTo(HaveOccurred(),
				"plugins.timeout=-1 must not cause a build error — negative "+
					"values are treated as 'keep original timeout' (issue #999)")
			Expect(result).NotTo(BeNil())

			html := result.RenderedContent["index.md"]
			Expect(html).To(ContainSubstring("HOOK_RAN"),
				"subsequent hooks must still run under the original timeout — "+
					"if HOOK_RAN is missing, setting timeout=-1 may have been applied "+
					"as a literal negative timeout (issue #999)")
		})

		It("plugins.workers can be set to string 'auto' via onConfig", func() {
			cfg := &config.Config{
				Title:   "Workers Auto Test",
				BaseURL: "https://example.com",
				Build:   config.BuildConfig{Output: "_site"},
			}
			// Plugin sets plugins.workers to "auto" (string).
			// The Workers field is interface{} — accepts both "auto" and int.
			// The chained hook verifies the string value propagates.
			contentMap := map[string]string{
				"content/index.md":       "---\ntitle: Home\nlayout: default\n---\n# Home",
				"layouts/default.liquid": "<html><body>{{ content }}</body></html>",
				"plugins/aaa-set-workers-auto.js": `export default function(alloy) {
  alloy.hook('onConfig', { priority: 10 }, (config) => {
    config.plugins.workers = "auto";
    return config;
  });
}`,
				"plugins/bbb-verify-workers-auto.js": `export default function(alloy) {
  alloy.hook('onConfig', { priority: 50 }, (config) => {
    if (config.plugins.workers !== "auto") {
      throw new Error('plugins.workers was not applied: expected "auto", got ' + JSON.stringify(config.plugins.workers));
    }
    return config;
  });
}`,
			}
			result, err := pipeline.BuildWithContent(cfg, contentMap)
			Expect(err).NotTo(HaveOccurred(),
				"onConfig must support plugins.workers='auto' — the Workers "+
					"field is interface{} and accepts both string and int (issue #999)")
			Expect(result).NotTo(BeNil())
		})
	})

	// ── onContentLoaded html merge-back (issue #976) ───────────────────
	// onContentLoaded documents `html` as mutable alongside `frontMatter`,
	// but the merge-back block only applies `frontMatter` — `html` mutations
	// are silently dropped. The fix: after merging frontMatter, check for
	// pageMap["html"] and call page.SetRenderedBody([]byte(html)), same
	// pattern as onContentTransformed (hooks.go) and onPageRendered (build.go).

	Describe("onContentLoaded html merge-back (issue #976)", func() {

		It("plugin can modify page html via onContentLoaded and the change appears in rendered output", func() {
			cfg := &config.Config{
				Title:   "HTML Merge-Back Test",
				BaseURL: "https://example.com",
				Build:   config.BuildConfig{Output: "_site"},
			}
			contentMap := map[string]string{
				"content/index.md":       "---\ntitle: Home\nlayout: default\n---\n# Home\n\nOriginal content paragraph.",
				"layouts/default.liquid": "<html><body>{{ content }}</body></html>",
				"plugins/rewrite-html.js": `export default function(alloy) {
  alloy.hook('onContentLoaded', { pages: true, pageFields: ["*"] }, function(pages) {
    for (var i = 0; i < pages.length; i++) {
      pages[i].html = pages[i].html + '<footer class="injected">Plugin Footer</footer>';
    }
    return pages;
  });
}`,
			}
			result, err := pipeline.BuildWithContent(cfg, contentMap)
			Expect(err).NotTo(HaveOccurred(),
				"onContentLoaded with html mutation must not error (issue #976)")
			Expect(result).NotTo(BeNil())

			html := result.RenderedContent["index.md"]
			Expect(html).To(ContainSubstring("Original content paragraph"),
				"original content must still be present after html merge-back (issue #976)")
			Expect(html).To(ContainSubstring(`<footer class="injected">Plugin Footer</footer>`),
				"onContentLoaded html mutation must be applied back to the page — "+
					"currently the merge-back block only reads pageMap[\"frontMatter\"] "+
					"and silently drops html changes. The fix: after merging frontMatter, "+
					"check for pageMap[\"html\"] and call page.SetRenderedBody([]byte(html)), "+
					"same pattern as onContentTransformed (issue #976)")
		})

		It("html replacement (not just append) is applied via onContentLoaded", func() {
			cfg := &config.Config{
				Title:   "HTML Replace Test",
				BaseURL: "https://example.com",
				Build:   config.BuildConfig{Output: "_site"},
			}
			contentMap := map[string]string{
				"content/index.md":       "---\ntitle: Home\nlayout: default\n---\n# Home\n\nOriginal body text.",
				"layouts/default.liquid": "<html><body>{{ content }}</body></html>",
				"plugins/replace-html.js": `export default function(alloy) {
  alloy.hook('onContentLoaded', { pages: true, pageFields: ["*"] }, function(pages) {
    for (var i = 0; i < pages.length; i++) {
      pages[i].html = '<div class="replaced">Completely Replaced Content</div>';
    }
    return pages;
  });
}`,
			}
			result, err := pipeline.BuildWithContent(cfg, contentMap)
			Expect(err).NotTo(HaveOccurred(),
				"onContentLoaded html replacement must not error (issue #976)")
			Expect(result).NotTo(BeNil())

			html := result.RenderedContent["index.md"]
			Expect(html).To(ContainSubstring(`<div class="replaced">Completely Replaced Content</div>`),
				"onContentLoaded html replacement must land in the final output — "+
					"the plugin replaces the entire html body, not just appends. "+
					"If this fails, html mutations are silently dropped (issue #976)")
			Expect(html).NotTo(ContainSubstring("Original body text"),
				"original content must NOT appear after full html replacement — "+
					"if it does, the html mutation was not applied and the original "+
					"RenderedBody was used instead (issue #976)")
		})

		It("batch html rewrite across multiple pages via onContentLoaded", func() {
			cfg := &config.Config{
				Title:   "Batch HTML Rewrite Test",
				BaseURL: "https://example.com",
				Build:   config.BuildConfig{Output: "_site"},
			}
			// Use page.path to inject a unique per-page marker so we can
			// verify each page gets its own mutation, not a cloned one.
			contentMap := map[string]string{
				"content/index.md":       "---\ntitle: Home\nlayout: default\n---\n# Home",
				"content/about.md":      "---\ntitle: About\nlayout: default\n---\n# About",
				"content/contact.md":    "---\ntitle: Contact\nlayout: default\n---\n# Contact",
				"layouts/default.liquid": "<html><body>{{ content }}</body></html>",
				"plugins/batch-rewrite.js": `export default function(alloy) {
  alloy.hook('onContentLoaded', { pages: true, pageFields: ["*"] }, function(pages) {
    for (var i = 0; i < pages.length; i++) {
      pages[i].html = pages[i].html + '<nav class="batch-nav" data-page="' + pages[i].path + '">Nav for ' + pages[i].path + '</nav>';
    }
    return pages;
  });
}`,
			}
			result, err := pipeline.BuildWithContent(cfg, contentMap)
			Expect(err).NotTo(HaveOccurred(),
				"batch html rewrite must not error (issue #976)")
			Expect(result).NotTo(BeNil())

			// Each page must have a nav with its own path, proving per-page merge-back
			htmlIndex := result.RenderedContent["index.md"]
			Expect(htmlIndex).To(ContainSubstring(`data-page="index.md"`),
				"index.md must have a nav with its own path — "+
					"if missing or wrong path, html merge-back is not per-page (issue #976)")

			htmlAbout := result.RenderedContent["about.md"]
			Expect(htmlAbout).To(ContainSubstring(`data-page="about.md"`),
				"about.md must have a nav with its own path (issue #976)")

			htmlContact := result.RenderedContent["contact.md"]
			Expect(htmlContact).To(ContainSubstring(`data-page="contact.md"`),
				"contact.md must have a nav with its own path (issue #976)")
		})

		It("html and frontMatter mutations are both applied in the same onContentLoaded call", func() {
			cfg := &config.Config{
				Title:   "HTML + FrontMatter Test",
				BaseURL: "https://example.com",
				Build:   config.BuildConfig{Output: "_site"},
			}
			contentMap := map[string]string{
				"content/index.md":       "---\ntitle: Home\nlayout: default\n---\n# Home",
				"layouts/default.liquid": "<html><body><h1>{{ page.title }}</h1>{{ content }}</body></html>",
				"plugins/mutate-both.js": `export default function(alloy) {
  alloy.hook('onContentLoaded', { pages: true, pageFields: ["*"] }, function(pages) {
    for (var i = 0; i < pages.length; i++) {
      pages[i].frontMatter.title = pages[i].frontMatter.title + ' (enriched)';
      pages[i].html = pages[i].html + '<span class="watermark">Processed</span>';
    }
    return pages;
  });
}`,
			}
			result, err := pipeline.BuildWithContent(cfg, contentMap)
			Expect(err).NotTo(HaveOccurred(),
				"simultaneous html and frontMatter mutation must not error (issue #976)")
			Expect(result).NotTo(BeNil())

			html := result.RenderedContent["index.md"]
			Expect(html).To(ContainSubstring("Home (enriched)"),
				"frontMatter.title mutation must be applied (issue #976)")
			Expect(html).To(ContainSubstring(`<span class="watermark">Processed</span>`),
				"html mutation must ALSO be applied in the same hook call — "+
					"both frontMatter and html are documented as mutable and must "+
					"both be merged back from the return value (issue #976)")
		})

		It("onContentLoaded html mutation uses content before layout wrapping", func() {
			cfg := &config.Config{
				Title:   "Pre-Layout HTML Test",
				BaseURL: "https://example.com",
				Build:   config.BuildConfig{Output: "_site"},
			}
			// The layout wraps content in a distinctive marker.
			// The plugin rewrites html (content body) — the rewrite should appear
			// INSIDE the layout wrapper, proving that onContentLoaded operates on
			// content html before layout rendering.
			contentMap := map[string]string{
				"content/index.md":       "---\ntitle: Home\nlayout: default\n---\n# Home\n\nBody text here.",
				"layouts/default.liquid": "<div class=\"layout-wrapper\">{{ content }}</div>",
				"plugins/pre-layout.js": `export default function(alloy) {
  alloy.hook('onContentLoaded', { pages: true, pageFields: ["*"] }, function(pages) {
    for (var i = 0; i < pages.length; i++) {
      pages[i].html = '<article class="rewritten">' + pages[i].html + '</article>';
    }
    return pages;
  });
}`,
			}
			result, err := pipeline.BuildWithContent(cfg, contentMap)
			Expect(err).NotTo(HaveOccurred(),
				"pre-layout html mutation must not error (issue #976)")
			Expect(result).NotTo(BeNil())

			html := result.RenderedContent["index.md"]
			// Verify nesting order: layout wrapper must contain the rewritten article.
			// If html mutation were applied AFTER layout rendering, the article
			// would be outside the wrapper or the wrapper would be absent.
			Expect(html).To(ContainSubstring(`<div class="layout-wrapper"><article class="rewritten">`),
				"rewritten article must be nested inside the layout wrapper — "+
					"proves onContentLoaded html mutation is applied before layout "+
					"rendering. If the article appears outside the wrapper, the "+
					"merge-back runs too late in the pipeline (issue #976)")
		})

		It("html-only return entries (no frontMatter) are applied via onContentLoaded", func() {
			cfg := &config.Config{
				Title:   "HTML-Only Return Test",
				BaseURL: "https://example.com",
				Build:   config.BuildConfig{Output: "_site"},
			}
			// Plugin returns sparse entries: only path + html, no frontMatter key.
			// This catches implementations that gate html merge-back on
			// frontMatter presence (the current bug's code structure nests
			// returnedPath extraction inside the frontMatter block).
			contentMap := map[string]string{
				"content/index.md":       "---\ntitle: Home\nlayout: default\n---\n# Home\n\nOriginal body.",
				"layouts/default.liquid": "<html><body>{{ content }}</body></html>",
				"plugins/html-only.js": `export default function(alloy) {
  alloy.hook('onContentLoaded', { pages: true, pageFields: ["*"] }, function(pages) {
    var result = [];
    for (var i = 0; i < pages.length; i++) {
      result.push({
        path: pages[i].path,
        html: pages[i].html + '<div class="html-only-marker">Injected without frontMatter</div>'
      });
    }
    return result;
  });
}`,
			}
			result, err := pipeline.BuildWithContent(cfg, contentMap)
			Expect(err).NotTo(HaveOccurred(),
				"onContentLoaded with html-only return (no frontMatter) must not error (issue #976)")
			Expect(result).NotTo(BeNil())

			html := result.RenderedContent["index.md"]
			Expect(html).To(ContainSubstring(`<div class="html-only-marker">Injected without frontMatter</div>`),
				"html-only return entries must be applied — if this fails, the "+
					"merge-back implementation gates html application on frontMatter "+
					"presence. The fix must extract returnedPath and apply html "+
					"independently of frontMatter (issue #976)")
		})
	})

	// ── Template tags in <code> not escaped for HTML content (#352) ─
	// escapeTemplateTagsInCode must only run on .md files, not .html.

	// ── onBeforeValidation payload and return contract (issue #975) ───────
	// onBeforeValidation fires immediately before conflict detection with
	// { outputPaths: ["about/index.html", ...] }. Return shape: { addOutputs:
	// { "path": "source" } } — additive only. Added paths feed into
	// DetectConflicts(). Currently broken: fires too early with trivial
	// one-entry map, return discarded.

	Describe("onBeforeValidation payload and return contract (issue #975)", func() {
		It("receives outputPaths array containing computed page output paths", func() {
			cfg := &config.Config{
				Title:   "BeforeValidation Payload Test",
				BaseURL: "https://example.com",
				Build:   config.BuildConfig{Output: "_site"},
			}
			contentMap := map[string]string{
				"content/about.md":       "---\ntitle: About\nlayout: default\n---\n# About",
				"content/index.md":       "---\ntitle: Home\nlayout: default\n---\n# Home",
				"layouts/default.liquid": "<html><body>{{ content }}</body></html>",
				"plugins/check-payload.js": `export default function(alloy) {
  alloy.hook('onBeforeValidation', {}, function(payload) {
    if (!payload || typeof payload !== 'object') {
      throw new Error('payload must be an object, got ' + typeof payload);
    }
    if (!Array.isArray(payload.outputPaths)) {
      throw new Error('payload.outputPaths must be an array, got ' + typeof payload.outputPaths);
    }
    if (payload.outputPaths.length < 2) {
      throw new Error('outputPaths must contain at least 2 entries (about + index), got ' + payload.outputPaths.length);
    }
    var hasAbout = false;
    for (var i = 0; i < payload.outputPaths.length; i++) {
      if (payload.outputPaths[i].indexOf('about') !== -1) {
        hasAbout = true;
      }
    }
    if (!hasAbout) {
      throw new Error('outputPaths must include about page path, got: ' + JSON.stringify(payload.outputPaths));
    }
  });
}`,
			}
			result, err := pipeline.BuildWithContent(cfg, contentMap)
			Expect(err).NotTo(HaveOccurred(),
				"onBeforeValidation must receive { outputPaths: [...] } with actual "+
					"computed page output paths — not the trivial one-entry map "+
					"{\"<output-dir>\": \"build output\"} that the current implementation "+
					"sends. If this fails with 'payload.outputPaths must be an array', "+
					"the hook still sends the wrong payload shape (issue #975)")
			Expect(result).NotTo(BeNil())
			Expect(result.PageCount).To(Equal(2),
				"both pages must be built after onBeforeValidation runs (issue #975)")
		})

		It("addOutputs entries that conflict with page output paths produce conflict error", func() {
			cfg := &config.Config{
				Title:   "BeforeValidation Conflict Test",
				BaseURL: "https://example.com",
				Build:   config.BuildConfig{Output: "_site"},
			}
			contentMap := map[string]string{
				"content/about.md":       "---\ntitle: About\nlayout: default\n---\n# About",
				"layouts/default.liquid": "<html><body>{{ content }}</body></html>",
				"plugins/add-conflict.js": `export default function(alloy) {
  alloy.hook('onBeforeValidation', {}, function(payload) {
    return {
      addOutputs: {
        "about/index.html": "plugin:conflict-test"
      }
    };
  });
}`,
			}
			_, err := pipeline.BuildWithContent(cfg, contentMap)
			Expect(err).To(HaveOccurred(),
				"onBeforeValidation addOutputs entry 'about/index.html' conflicts with "+
					"the real about page — DetectConflicts must catch this. If this passes, "+
					"addOutputs is silently discarded and the return value is not being "+
					"processed (issue #975)")
			Expect(err.Error()).To(ContainSubstring("conflict"),
				"error must indicate an output path conflict (issue #975)")
			Expect(err.Error()).To(ContainSubstring("about/index.html"),
				"conflict error must identify the conflicting output path (issue #975)")
		})

		It("addOutputs with unique path does not produce conflict", func() {
			cfg := &config.Config{
				Title:   "BeforeValidation No Conflict Test",
				BaseURL: "https://example.com",
				Build:   config.BuildConfig{Output: "_site"},
			}
			contentMap := map[string]string{
				"content/index.md":       "---\ntitle: Home\nlayout: default\n---\n# Home",
				"layouts/default.liquid": "<html><body>{{ content }}</body></html>",
				"plugins/add-unique.js": `export default function(alloy) {
  alloy.hook('onBeforeValidation', {}, function(payload) {
    return {
      addOutputs: {
        "_redirects": "plugin:netlify-redirects",
        "_headers": "plugin:netlify-headers"
      }
    };
  });
}`,
			}
			result, err := pipeline.BuildWithContent(cfg, contentMap)
			Expect(err).NotTo(HaveOccurred(),
				"onBeforeValidation addOutputs with unique paths must not produce "+
					"conflict errors — _redirects and _headers do not collide with "+
					"any page output path (issue #975)")
			Expect(result).NotTo(BeNil())
		})

		It("rejects addOutputs when value is not a map", func() {
			cfg := &config.Config{
				Title:   "BeforeValidation Type Error Test",
				BaseURL: "https://example.com",
				Build:   config.BuildConfig{Output: "_site"},
			}
			contentMap := map[string]string{
				"content/index.md":       "---\ntitle: Home\nlayout: default\n---\n# Home",
				"layouts/default.liquid": "<html><body>{{ content }}</body></html>",
				"plugins/bad-type.js": `export default function(alloy) {
  alloy.hook('onBeforeValidation', {}, function(payload) {
    return { addOutputs: ["_redirects", "_headers"] };
  });
}`,
			}
			_, err := pipeline.BuildWithContent(cfg, contentMap)
			Expect(err).To(HaveOccurred(),
				"onBeforeValidation addOutputs must be a map (path → source label), "+
					"not an array. A map is required so conflict error messages include "+
					"the plugin source label (issue #975)")
			Expect(err.Error()).To(ContainSubstring("addOutputs"),
				"error must reference 'addOutputs' so plugin authors know which "+
					"return field has the wrong type (issue #975)")
		})

		It("rejects unrecognized return keys", func() {
			cfg := &config.Config{
				Title:   "BeforeValidation Unrecognized Key Test",
				BaseURL: "https://example.com",
				Build:   config.BuildConfig{Output: "_site"},
			}
			contentMap := map[string]string{
				"content/index.md":       "---\ntitle: Home\nlayout: default\n---\n# Home",
				"layouts/default.liquid": "<html><body>{{ content }}</body></html>",
				"plugins/bad-shape.js": `export default function(alloy) {
  alloy.hook('onBeforeValidation', {}, function(payload) {
    return { paths: payload.outputPaths, extra: true };
  });
}`,
			}
			_, err := pipeline.BuildWithContent(cfg, contentMap)
			Expect(err).To(HaveOccurred(),
				"onBeforeValidation must reject unrecognized return keys — "+
					"only 'addOutputs' is recognized. Returning 'paths' or other "+
					"keys is a plugin author mistake that should be caught early, "+
					"matching the addPages pattern for onPagesReady (issue #975)")
			Expect(err.Error()).To(ContainSubstring("onBeforeValidation"),
				"error must identify onBeforeValidation as the source hook (issue #975)")
		})

		It("no-op return from observation-only plugin does not error", func() {
			cfg := &config.Config{
				Title:   "BeforeValidation No-Op Test",
				BaseURL: "https://example.com",
				Build:   config.BuildConfig{Output: "_site"},
			}
			contentMap := map[string]string{
				"content/index.md":       "---\ntitle: Home\nlayout: default\n---\n# Home",
				"layouts/default.liquid": "<html><body>{{ content }}</body></html>",
				"plugins/observe-only.js": `export default function(alloy) {
  alloy.hook('onBeforeValidation', {}, function(payload) {
    // observation only — no return value
  });
}`,
			}
			result, err := pipeline.BuildWithContent(cfg, contentMap)
			Expect(err).NotTo(HaveOccurred(),
				"onBeforeValidation with no return value (observation-only plugin) "+
					"must not error — undefined/null return is a valid no-op (issue #975)")
			Expect(result).NotTo(BeNil())
			Expect(result.PageCount).To(Equal(1),
				"pages must still be built after observation-only onBeforeValidation (issue #975)")
		})

		It("conflict error message includes plugin source label from addOutputs", func() {
			cfg := &config.Config{
				Title:   "BeforeValidation Source Label Test",
				BaseURL: "https://example.com",
				Build:   config.BuildConfig{Output: "_site"},
			}
			contentMap := map[string]string{
				"content/about.md":       "---\ntitle: About\nlayout: default\n---\n# About",
				"layouts/default.liquid": "<html><body>{{ content }}</body></html>",
				"plugins/add-labeled.js": `export default function(alloy) {
  alloy.hook('onBeforeValidation', {}, function(payload) {
    return {
      addOutputs: {
        "about/index.html": "plugin:netlify-redirects"
      }
    };
  });
}`,
			}
			_, err := pipeline.BuildWithContent(cfg, contentMap)
			Expect(err).To(HaveOccurred(),
				"addOutputs entry conflicting with about page must produce error (issue #975)")
			Expect(err.Error()).To(ContainSubstring("plugin:netlify-redirects"),
				"conflict error must include the source label from addOutputs — "+
					"this is why addOutputs is a map (path → source) rather than an "+
					"array: the source label appears in conflict error messages so "+
					"plugin authors can identify which plugin registered the conflicting "+
					"output path (issue #975)")
		})
	})

	// ── onAfterValidation payload and return contract (issue #975) ───────
	// onAfterValidation fires immediately after conflict detection passes
	// with { outputPaths: [...], cascade: { ...siteData... } }. Cascade
	// mutations are applied back to siteData. Currently broken: fires too
	// early with trivial one-entry map, return discarded, no cascade.

	Describe("onAfterValidation payload and return contract (issue #975)", func() {
		It("receives outputPaths array and cascade object in payload", func() {
			cfg := &config.Config{
				Title:   "AfterValidation Payload Test",
				BaseURL: "https://example.com",
				Build:   config.BuildConfig{Output: "_site"},
			}
			contentMap := map[string]string{
				"content/about.md":       "---\ntitle: About\nlayout: default\n---\n# About",
				"content/index.md":       "---\ntitle: Home\nlayout: default\n---\n# Home",
				"layouts/default.liquid": "<html><body>{{ content }}</body></html>",
				"plugins/check-payload.js": `export default function(alloy) {
  alloy.hook('onAfterValidation', {}, function(payload) {
    if (!payload || typeof payload !== 'object') {
      throw new Error('payload must be an object, got ' + typeof payload);
    }
    if (!Array.isArray(payload.outputPaths)) {
      throw new Error('payload.outputPaths must be an array, got ' + typeof payload.outputPaths);
    }
    if (payload.outputPaths.length < 2) {
      throw new Error('outputPaths must contain at least 2 entries (about + index), got ' + payload.outputPaths.length);
    }
    if (!payload.cascade || typeof payload.cascade !== 'object') {
      throw new Error('payload.cascade must be an object, got ' + typeof payload.cascade);
    }
  });
}`,
			}
			result, err := pipeline.BuildWithContent(cfg, contentMap)
			Expect(err).NotTo(HaveOccurred(),
				"onAfterValidation must receive { outputPaths: [...], cascade: {...} } "+
					"with actual computed output paths and the site data cascade — "+
					"not the trivial one-entry map that the current implementation sends. "+
					"If this fails with 'payload.outputPaths must be an array' or "+
					"'payload.cascade must be an object', the hook still sends the "+
					"wrong payload shape (issue #975)")
			Expect(result).NotTo(BeNil())
			Expect(result.PageCount).To(Equal(2),
				"both pages must be built after onAfterValidation runs (issue #975)")
		})

		It("cascade mutation is applied to site data for templates", func() {
			cfg := &config.Config{
				Title:   "AfterValidation Cascade Test",
				BaseURL: "https://example.com",
				Build:   config.BuildConfig{Output: "_site"},
			}
			contentMap := map[string]string{
				"content/index.md":       "---\ntitle: Home\nlayout: default\n---\n# Home",
				"layouts/default.liquid": "<html><body>{{ content }}<p>Injected: {{ site.data.injectedValue }}</p></body></html>",
				"plugins/inject-cascade.js": `export default function(alloy) {
  alloy.hook('onAfterValidation', {}, function(payload) {
    payload.cascade.injectedValue = 'cascade-from-plugin';
    return payload;
  });
}`,
			}
			result, err := pipeline.BuildWithContent(cfg, contentMap)
			Expect(err).NotTo(HaveOccurred(),
				"onAfterValidation cascade mutation must not error (issue #975)")
			Expect(result).NotTo(BeNil())

			html := result.RenderedContent["index.md"]
			Expect(html).To(ContainSubstring("Injected: cascade-from-plugin"),
				"onAfterValidation cascade mutations must be applied back to siteData — "+
					"the plugin set cascade.injectedValue = 'cascade-from-plugin' and "+
					"the template renders {{ site.data.injectedValue }}. If this fails, "+
					"the return value is being discarded (issue #975)")
		})

		It("outputPaths changes in return are ignored", func() {
			cfg := &config.Config{
				Title:   "AfterValidation ReadOnly Paths Test",
				BaseURL: "https://example.com",
				Build:   config.BuildConfig{Output: "_site"},
			}
			contentMap := map[string]string{
				"content/index.md":       "---\ntitle: Home\nlayout: default\n---\n# Home",
				"layouts/default.liquid": "<html><body>{{ content }}</body></html>",
				"plugins/modify-paths.js": `export default function(alloy) {
  alloy.hook('onAfterValidation', {}, function(payload) {
    payload.outputPaths.push('fake/injected-path.html');
    return payload;
  });
}`,
			}
			result, err := pipeline.BuildWithContent(cfg, contentMap)
			Expect(err).NotTo(HaveOccurred(),
				"onAfterValidation must ignore outputPaths changes in the return — "+
					"validation has already passed, no further path modifications are "+
					"allowed. Modifying outputPaths in the return should not error "+
					"(it is silently ignored) (issue #975)")
			Expect(result).NotTo(BeNil())
			Expect(result.PageCount).To(Equal(1),
				"only the real page should exist — the fake path added to "+
					"outputPaths in the return must not create a page (issue #975)")
		})

		It("observation-only plugin with no return does not error", func() {
			cfg := &config.Config{
				Title:   "AfterValidation Observe Test",
				BaseURL: "https://example.com",
				Build:   config.BuildConfig{Output: "_site"},
			}
			contentMap := map[string]string{
				"content/index.md":       "---\ntitle: Home\nlayout: default\n---\n# Home",
				"layouts/default.liquid": "<html><body>{{ content }}</body></html>",
				"plugins/observe-only.js": `export default function(alloy) {
  alloy.hook('onAfterValidation', {}, function(payload) {
    // observation only — log but do not return
  });
}`,
			}
			result, err := pipeline.BuildWithContent(cfg, contentMap)
			Expect(err).NotTo(HaveOccurred(),
				"onAfterValidation with no return value (observation-only plugin) "+
					"must not error — undefined/null return is a valid no-op (issue #975)")
			Expect(result).NotTo(BeNil())
			Expect(result.PageCount).To(Equal(1),
				"pages must still be built after observation-only onAfterValidation (issue #975)")
		})

		It("cascade mutations from multiple hooks are merged into site data", func() {
			cfg := &config.Config{
				Title:   "AfterValidation Multi-Hook Test",
				BaseURL: "https://example.com",
				Build:   config.BuildConfig{Output: "_site"},
			}
			contentMap := map[string]string{
				"content/index.md":       "---\ntitle: Home\nlayout: default\n---\n# Home",
				"layouts/default.liquid": "<html><body>{{ content }}<p>A: {{ site.data.keyFromHookA }}</p><p>B: {{ site.data.keyFromHookB }}</p></body></html>",
				"plugins/multi-hook.js": `export default function(alloy) {
  alloy.hook('onAfterValidation', {}, function(payload) {
    payload.cascade.keyFromHookA = 'alpha';
    return payload;
  });
  alloy.hook('onAfterValidation', {}, function(payload) {
    payload.cascade.keyFromHookB = 'beta';
    return payload;
  });
}`,
			}
			result, err := pipeline.BuildWithContent(cfg, contentMap)
			Expect(err).NotTo(HaveOccurred(),
				"multiple onAfterValidation hooks must not error (issue #975)")
			Expect(result).NotTo(BeNil())

			html := result.RenderedContent["index.md"]
			Expect(html).To(ContainSubstring("A: alpha"),
				"first hook's cascade mutation (keyFromHookA = 'alpha') must be "+
					"applied to site data — if missing, only the last hook's return "+
					"was applied instead of merging all hooks (issue #975)")
			Expect(html).To(ContainSubstring("B: beta"),
				"second hook's cascade mutation (keyFromHookB = 'beta') must be "+
					"applied to site data — both hooks' cascade changes must be "+
					"independently merged into siteData (issue #975)")
		})

		It("rejects cascade returned as non-map", func() {
			cfg := &config.Config{
				Title:   "AfterValidation Cascade Type Test",
				BaseURL: "https://example.com",
				Build:   config.BuildConfig{Output: "_site"},
			}
			contentMap := map[string]string{
				"content/index.md":       "---\ntitle: Home\nlayout: default\n---\n# Home",
				"layouts/default.liquid": "<html><body>{{ content }}</body></html>",
				"plugins/bad-cascade.js": `export default function(alloy) {
  alloy.hook('onAfterValidation', {}, function(payload) {
    return { cascade: 42 };
  });
}`,
			}
			_, err := pipeline.BuildWithContent(cfg, contentMap)
			Expect(err).To(HaveOccurred(),
				"onAfterValidation cascade must be a map — returning a number "+
					"is a type error that should be caught (issue #975)")
			Expect(err.Error()).To(ContainSubstring("cascade"),
				"error must reference 'cascade' so plugin authors know which "+
					"return field has the wrong type (issue #975)")
		})

		It("outputPaths includes plugin-added paths from onBeforeValidation", func() {
			cfg := &config.Config{
				Title:   "AfterValidation Includes Plugin Paths Test",
				BaseURL: "https://example.com",
				Build:   config.BuildConfig{Output: "_site"},
			}
			contentMap := map[string]string{
				"content/index.md":       "---\ntitle: Home\nlayout: default\n---\n# Home",
				"layouts/default.liquid": "<html><body>{{ content }}</body></html>",
				"plugins/cross-hook.js": `export default function(alloy) {
  alloy.hook('onBeforeValidation', {}, function(payload) {
    return {
      addOutputs: {
        "_redirects": "plugin:netlify-redirects"
      }
    };
  });
  alloy.hook('onAfterValidation', {}, function(payload) {
    var hasRedirects = false;
    for (var i = 0; i < payload.outputPaths.length; i++) {
      if (payload.outputPaths[i] === '_redirects') {
        hasRedirects = true;
      }
    }
    if (!hasRedirects) {
      throw new Error('outputPaths must include _redirects added by onBeforeValidation, got: ' + JSON.stringify(payload.outputPaths));
    }
  });
}`,
			}
			result, err := pipeline.BuildWithContent(cfg, contentMap)
			Expect(err).NotTo(HaveOccurred(),
				"onAfterValidation must receive outputPaths that includes paths "+
					"added by onBeforeValidation's addOutputs — the _redirects path "+
					"was registered before conflict detection and must appear in the "+
					"validated manifest. If this fails with 'outputPaths must include "+
					"_redirects', the onBeforeValidation addOutputs are not being "+
					"carried forward to onAfterValidation (issue #975)")
			Expect(result).NotTo(BeNil())
		})
	})
})
