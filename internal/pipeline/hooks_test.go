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

	// ── Template tags in <code> not escaped for HTML content (#352) ─
	// escapeTemplateTagsInCode must only run on .md files, not .html.
})
