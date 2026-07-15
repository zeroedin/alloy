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
	})

	// ── Template tags in <code> not escaped for HTML content (#352) ─
	// escapeTemplateTagsInCode must only run on .md files, not .html.
})
