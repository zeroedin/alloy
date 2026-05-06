package pipeline_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/zeroedin/alloy/internal/config"
	"github.com/zeroedin/alloy/internal/pipeline"
)

var _ = Describe("Scope-aware pipeline integration (issue #539)", func() {

	It("onPagesReady with data: [\"elements\"] receives only elements key in siteData", func() {
		cfg := &config.Config{
			Title:   "Scoped SiteData Test",
			BaseURL: "https://example.com",
			Build:   config.BuildConfig{Output: "_site"},
		}
		contentMap := map[string]string{
			"data/elements.json":     `[{"name":"Button"}]`,
			"data/tokens.json":       `{"color":"red"}`,
			"content/index.md":       "---\ntitle: Home\nlayout: default\n---\n# Home",
			"layouts/default.liquid": "<html><body>{{ content }}</body></html>",
			"plugins/scoped-data.js": `export default function(alloy) {
  alloy.hook('onPagesReady', { data: ["elements"], pages: true, pageFields: ["*"] }, function(payload) {
    if (payload.siteData.tokens) {
      throw new Error('siteData must not include tokens when data scope is ["elements"]');
    }
    if (!payload.siteData.elements) {
      throw new Error('siteData must include elements when data scope is ["elements"]');
    }
    return payload;
  });
}`,
		}
		result, err := pipeline.BuildWithContent(cfg, contentMap)
		Expect(err).NotTo(HaveOccurred(),
			"onPagesReady with data: [\"elements\"] must filter siteData to only "+
				"include the elements key — if this fails with 'siteData must not include tokens', "+
				"the pipeline is sending unfiltered siteData (issue #539)")
		Expect(result).NotTo(BeNil())
	})

	It("onPagesReady with pages: false still provides the pages array for injection", func() {
		cfg := &config.Config{
			Title:   "Pages False Test",
			BaseURL: "https://example.com",
			Build:   config.BuildConfig{Output: "_site"},
		}
		contentMap := map[string]string{
			"data/elements.json":     `[{"name":"Button","slug":"button"}]`,
			"content/index.md":       "---\ntitle: Home\nlayout: default\n---\n# Home",
			"layouts/default.liquid": "<html><body>{{ content }}</body></html>",
			"plugins/no-pages.js": `export default function(alloy) {
  alloy.hook('onPagesReady', { pages: false, data: ["elements"] }, function(payload) {
    if (!payload.siteData || !payload.siteData.elements) {
      throw new Error('siteData.elements must be present when data scope includes elements');
    }
    if (payload.siteData.tokens) {
      throw new Error('siteData.tokens must not be present when data scope is ["elements"]');
    }
    if (!payload.pages || typeof payload.pages.push !== 'function') {
      throw new Error('pages array must be present for virtual page injection');
    }
    var elements = payload.siteData.elements;
    for (var i = 0; i < elements.length; i++) {
      payload.pages.push({
        path: 'generated/' + elements[i].slug + '.md',
        url: '/generated/' + elements[i].slug + '/',
        frontMatter: { title: elements[i].name, layout: 'default' },
        content: '# ' + elements[i].name
      });
    }
    return payload;
  });
}`,
		}
		result, err := pipeline.BuildWithContent(cfg, contentMap)
		Expect(err).NotTo(HaveOccurred(),
			"onPagesReady always sends the pages array regardless of pages scope — "+
				"runOnPagesReady does not check Pages.Mode because the hook needs "+
				"the array for virtual page injection (issue #539)")
		Expect(result).NotTo(BeNil())
		Expect(result.PageCount).To(Equal(2),
			"1 real page + 1 data-driven virtual page = 2 total (issue #539)")
	})

	It("onContentTransformed with pages scope none skips hook entirely", func() {
		cfg := &config.Config{
			Title:   "ContentTransformed Skip Test",
			BaseURL: "https://example.com",
			Build:   config.BuildConfig{Output: "_site"},
		}
		contentMap := map[string]string{
			"content/index.md":       "---\ntitle: Home\nlayout: default\n---\n# Home",
			"layouts/default.liquid": "<html><body>{{ content }}</body></html>",
			"plugins/skip-transform.js": `export default function(alloy) {
  alloy.hook('onContentTransformed', { pages: false }, (page) => {
    throw new Error('hook must not be called when pages scope is false');
  });
}`,
		}
		result, err := pipeline.BuildWithContent(cfg, contentMap)
		Expect(err).NotTo(HaveOccurred(),
			"onContentTransformed with pages: false must skip the hook entirely — "+
				"if this fails with 'hook must not be called', "+
				"fireContentTransformedHooks ignores the PagesScopeNone early exit (issue #539)")
		Expect(result).NotTo(BeNil())
	})

	It("onContentLoaded with pageFields: [\"frontMatter\"] receives pages without html/content", func() {
		cfg := &config.Config{
			Title:   "Scoped Fields Test",
			BaseURL: "https://example.com",
			Build:   config.BuildConfig{Output: "_site"},
		}
		contentMap := map[string]string{
			"content/index.md":       "---\ntitle: Home\nlayout: default\n---\n# Home",
			"layouts/default.liquid": "<html><body>{{ content }}</body></html>",
			"plugins/field-filter.js": `export default function(alloy) {
  alloy.hook('onContentLoaded', { pages: true, pageFields: ["frontMatter"] }, function(pages) {
    for (var i = 0; i < pages.length; i++) {
      if (!pages[i].frontMatter) {
        throw new Error('frontMatter must be present when listed in pageFields');
      }
      if (pages[i].html) {
        throw new Error('html must not be present when not listed in pageFields');
      }
      if (pages[i].content) {
        throw new Error('content must not be present when not listed in pageFields');
      }
    }
    return pages;
  });
}`,
		}
		result, err := pipeline.BuildWithContent(cfg, contentMap)
		Expect(err).NotTo(HaveOccurred(),
			"onContentLoaded with pageFields: [\"frontMatter\"] must omit html and content — "+
				"if this fails, the pipeline is serializing fields the plugin did not request (issue #539)")
		Expect(result).NotTo(BeNil())
	})
})
