package pipeline_test

import (
	"bytes"
	"log"
	"os"
	"path/filepath"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/zeroedin/alloy/internal/config"
	"github.com/zeroedin/alloy/internal/pipeline"
)

// ── Hook returns must carry their output field (issue #1179) ──────────
// On onPageRendered and onContentTransformed each hook's return becomes
// the next hook's input. A return without `html` left the next hook
// holding `undefined`, so the build failed inside the *next* plugin and
// blamed it for the earlier one's mistake. A lone hook making the same
// mistake built silently. The contract: on these events every hook's
// return must be an object carrying `html` as a string (whenever the
// pipeline would apply `html` back), checked at the hook that returned
// it, whatever its position. Anything else is a build error naming the
// event, the plugin, the page, and the field.
//
// See PLAN.md → "A hook's return must carry the field it exists to
// produce (issue #1179)".

// hookContractConfig returns a minimal site config for BuildWithContent.
func hookContractConfig(title string) *config.Config {
	return &config.Config{
		Title:   title,
		BaseURL: "https://example.com",
		Build:   config.BuildConfig{Output: "_site"},
	}
}

// victimPlugin is a well-behaved hook that runs after the offender. It
// dereferences page.html the way real plugins do, and marks its output
// so a test can tell whether it ran. If it is ever handed a malformed
// payload it throws a message the tests assert never appears.
func victimPlugin(event, scope string) string {
	return `export default function(alloy) {
  alloy.hook('` + event + `', ` + scope + `, function(page) {
    if (page === null || typeof page !== 'object' || typeof page.html !== 'string') {
      throw new Error('VICTIM-HANDED-MALFORMED-PAYLOAD');
    }
    page.html = page.html + '<!-- victim-ran -->';
    return page;
  });
}`
}

// expectContractError asserts the build error carries the four elements
// the contract requires: event, offending plugin, page, and field. The
// page is /about/ unless a test says otherwise.
func expectContractError(err error, event, plugin string) {
	GinkgoHelper()
	expectContractErrorOn(err, event, plugin, "about")
}

// expectContractErrorOn is expectContractError for the page whose URL is
// /<slug>/ and whose path is <slug>.md.
func expectContractErrorOn(err error, event, plugin, slug string) {
	GinkgoHelper()
	expectNonObjectError(err, event, plugin, slug)
	Expect(err.Error()).To(ContainSubstring("html"),
		"the error must name the missing or malformed field, `html` (issue #1179)")
}

// expectNonObjectError asserts the error for a return that is not an
// object at all. It must name the event, plugin and page; it names no
// field, because there is no object to be missing one.
func expectNonObjectError(err error, event, plugin, slug string) {
	GinkgoHelper()
	Expect(err).To(HaveOccurred(),
		"a malformed hook return on "+event+" must fail the build — it must "+
			"not be silently ignored or passed on to the next hook (issue #1179)")
	msg := err.Error()
	Expect(msg).To(ContainSubstring(event),
		"the error must name the event (issue #1179)")
	Expect(msg).To(ContainSubstring(plugin),
		"the error must name the plugin that returned the malformed value, "+
			"%q (issue #1179)", plugin)
	Expect(msg).To(Or(ContainSubstring("/"+slug+"/"), ContainSubstring(slug+".md")),
		"the error must identify the page by URL or path — /%s/ (issue #1179)", slug)
	Expect(msg).NotTo(ContainSubstring("VICTIM-HANDED-MALFORMED-PAYLOAD"),
		"the malformed return must be caught before it reaches the next hook "+
			"(issue #1179)")
}

var _ = Describe("Hook return contract (issue #1179)", func() {

	Describe("onPageRendered", func() {
		content := map[string]string{
			"content/about.md":       "---\ntitle: About\nlayout: default\n---\n# About Body",
			"layouts/default.liquid": "<html><body>{{ content }}</body></html>",
		}
		withPlugins := func(plugins map[string]string) map[string]string {
			m := make(map[string]string, len(content)+len(plugins))
			for k, v := range content {
				m[k] = v
			}
			for k, v := range plugins {
				m[k] = v
			}
			return m
		}

		It("names the earlier plugin, not the next one, when a chained hook drops html", func() {
			_, err := pipeline.BuildWithContent(hookContractConfig("Chain Blame"), withPlugins(map[string]string{
				"plugins/00-first.js": `export default function(alloy) {
  alloy.hook('onPageRendered', {}, function(page) {
    return { url: page.url };
  });
}`,
				"plugins/01-second.js": victimPlugin("onPageRendered", "{}"),
			}))
			expectContractError(err, "onPageRendered", "00-first")
			Expect(err.Error()).NotTo(ContainSubstring("01-second"),
				"the error must not name the next hook in the chain — that "+
					"misattribution is the defect issue #1179 reports")
		})

		It("rejects a hook that returns nothing", func() {
			_, err := pipeline.BuildWithContent(hookContractConfig("No Return"), withPlugins(map[string]string{
				"plugins/00-first.js": `export default function(alloy) {
  alloy.hook('onPageRendered', {}, function(page) {
    page.html = page.html + '<!-- mutated-but-not-returned -->';
  });
}`,
				"plugins/01-second.js": victimPlugin("onPageRendered", "{}"),
			}))
			expectContractError(err, "onPageRendered", "00-first")
		})

		It("rejects a lone hook that drops html — validity must not depend on position", func() {
			// Today this builds silently and keeps the original html. The
			// same plugin crashes a build the moment another hook follows it.
			_, err := pipeline.BuildWithContent(hookContractConfig("Lone Drop"), withPlugins(map[string]string{
				"plugins/00-first.js": `export default function(alloy) {
  alloy.hook('onPageRendered', {}, function(page) {
    return { url: page.url };
  });
}`,
			}))
			expectContractError(err, "onPageRendered", "00-first")
		})

		It("rejects the last hook in a chain that drops html", func() {
			_, err := pipeline.BuildWithContent(hookContractConfig("Last Drop"), withPlugins(map[string]string{
				"plugins/00-first.js": victimPlugin("onPageRendered", "{}"),
				"plugins/01-second.js": `export default function(alloy) {
  alloy.hook('onPageRendered', {}, function(page) {
    return { url: page.url };
  });
}`,
			}))
			expectContractError(err, "onPageRendered", "01-second")
		})

		It("checks every page in the batch, not only the first", func() {
			// Pages are dispatched in sorted order, so /zeta/ is item 1
			// behind /about/. Only /zeta/ is malformed: a check that looks
			// at item 0 alone would miss it.
			m := withPlugins(map[string]string{
				"plugins/00-first.js": `export default function(alloy) {
  alloy.hook('onPageRendered', {}, function(page) {
    if (page.url === '/zeta/') return { url: page.url };
    return page;
  });
}`,
				"plugins/01-second.js": victimPlugin("onPageRendered", "{}"),
			})
			m["content/zeta.md"] = "---\ntitle: Zeta\nlayout: default\n---\n# Zeta Body"
			_, err := pipeline.BuildWithContent(hookContractConfig("Batch Every Item"), m)
			expectContractErrorOn(err, "onPageRendered", "00-first", "zeta")
			Expect(err.Error()).NotTo(Or(ContainSubstring("/about/"), ContainSubstring("about.md")),
				"the error must name the page whose return was malformed, not "+
					"another page in the same batch (issue #1179)")
		})

		It("rejects a bare string return", func() {
			_, err := pipeline.BuildWithContent(hookContractConfig("String Return"), withPlugins(map[string]string{
				"plugins/00-first.js": `export default function(alloy) {
  alloy.hook('onPageRendered', {}, function(page) {
    return page.html + '<!-- old-string-api -->';
  });
}`,
			}))
			expectContractError(err, "onPageRendered", "00-first")
		})

		It("rejects an array return", func() {
			_, err := pipeline.BuildWithContent(hookContractConfig("Array Return"), withPlugins(map[string]string{
				"plugins/00-first.js": `export default function(alloy) {
  alloy.hook('onPageRendered', {}, function(page) {
    return [page.html];
  });
}`,
			}))
			expectContractError(err, "onPageRendered", "00-first")
		})

		It("rejects html present but not a string", func() {
			_, err := pipeline.BuildWithContent(hookContractConfig("Non-String HTML"), withPlugins(map[string]string{
				"plugins/00-first.js": `export default function(alloy) {
  alloy.hook('onPageRendered', {}, function(page) {
    page.html = 42;
    return page;
  });
}`,
				"plugins/01-second.js": victimPlugin("onPageRendered", "{}"),
			}))
			expectContractError(err, "onPageRendered", "00-first")
		})

		It("accepts the page object through a chain (guard)", func() {
			result, err := pipeline.BuildWithContent(hookContractConfig("Chain OK"), withPlugins(map[string]string{
				"plugins/00-first.js": `export default function(alloy) {
  alloy.hook('onPageRendered', {}, function(page) {
    page.html = page.html + '<!-- first-ran -->';
    return page;
  });
}`,
				"plugins/01-second.js": victimPlugin("onPageRendered", "{}"),
			}))
			Expect(err).NotTo(HaveOccurred(),
				"returning the page object must stay valid (issue #1179)")
			html := result.RenderedContent["about.md"]
			Expect(html).To(ContainSubstring("About Body"))
			Expect(html).To(ContainSubstring("<!-- first-ran --><!-- victim-ran -->"),
				"both hooks' changes must reach the page, in chain order")
		})

		It("accepts an empty html string (guard)", func() {
			// Empty is a legitimate result — a plugin may blank a page.
			// Only a missing or non-string html is malformed.
			result, err := pipeline.BuildWithContent(hookContractConfig("Empty HTML"), withPlugins(map[string]string{
				"plugins/00-first.js": `export default function(alloy) {
  alloy.hook('onPageRendered', {}, function(page) {
    page.html = '';
    return page;
  });
}`,
			}))
			Expect(err).NotTo(HaveOccurred(),
				"html: \"\" is a string and must be accepted (issue #1179)")
			Expect(result.RenderedContent["about.md"]).NotTo(ContainSubstring("About Body"),
				"the empty html must be applied back, not treated as absent")
		})
	})

	Describe("onContentTransformed", func() {
		content := map[string]string{
			"content/about.md":       "---\ntitle: About\nlayout: default\n---\n## Section\n\nAbout Body",
			"layouts/default.liquid": "<html><body><nav>{% for e in page.toc %}[{{ e.text }}]{% endfor %}</nav>{{ content }}</body></html>",
		}
		withPlugins := func(plugins map[string]string) map[string]string {
			m := make(map[string]string, len(content)+len(plugins))
			for k, v := range content {
				m[k] = v
			}
			for k, v := range plugins {
				m[k] = v
			}
			return m
		}

		It("names the earlier plugin, not the next one, when a chained hook drops html", func() {
			_, err := pipeline.BuildWithContent(hookContractConfig("CT Chain Blame"), withPlugins(map[string]string{
				"plugins/00-first.js": `export default function(alloy) {
  alloy.hook('onContentTransformed', {}, function(page) {
    return { url: page.url };
  });
}`,
				"plugins/01-second.js": victimPlugin("onContentTransformed", "{}"),
			}))
			expectContractError(err, "onContentTransformed", "00-first")
			Expect(err.Error()).NotTo(ContainSubstring("01-second"),
				"the error must not name the next hook in the chain (issue #1179)")
		})

		It("rejects a lone bare string return", func() {
			// Today a lone string return is applied back, but the same
			// plugin breaks any hook placed after it. Retired on both events.
			_, err := pipeline.BuildWithContent(hookContractConfig("CT String"), withPlugins(map[string]string{
				"plugins/00-first.js": `export default function(alloy) {
  alloy.hook('onContentTransformed', { pages: true, pageFields: ["html"] }, function(page) {
    return page.html + '<!-- old-string-api -->';
  });
}`,
			}))
			expectContractError(err, "onContentTransformed", "00-first")
		})

		It("requires html from a toc-scoped hook when another hook on the event wants html", func() {
			// The union scope wants html, so the chain's final return is
			// applied back as html. A toc-only return would drop it.
			_, err := pipeline.BuildWithContent(hookContractConfig("CT TOC In HTML Chain"), withPlugins(map[string]string{
				"plugins/00-toc.js": `export default function(alloy) {
  alloy.hook('onContentTransformed', { pages: true, pageFields: ["toc"] }, function(page) {
    return { toc: [{ id: 'x', text: 'From TOC Hook', level: 2 }] };
  });
}`,
				"plugins/01-html.js": victimPlugin("onContentTransformed", `{ pages: true, pageFields: ["html"] }`),
			}))
			expectContractError(err, "onContentTransformed", "00-toc")
		})

		It("rejects a bare string from a toc-only hook even when html is not required", func() {
			// html is optional here, but the return must still be an object.
			// Today the string is applied back as the page html.
			_, err := pipeline.BuildWithContent(hookContractConfig("CT TOC String"), withPlugins(map[string]string{
				"plugins/00-toc.js": `export default function(alloy) {
  alloy.hook('onContentTransformed', { pages: true, pageFields: ["toc"] }, function(page) {
    return '<p>replaced by a string</p>';
  });
}`,
			}))
			expectNonObjectError(err, "onContentTransformed", "00-toc", "about")
		})

		It("accepts a toc-only return when no hook on the event wants html (guard)", func() {
			// html is sent regardless of scope but is not applied back when
			// no hook asks for it, so it is not required. "Required if sent"
			// would wrongly reject this plugin.
			result, err := pipeline.BuildWithContent(hookContractConfig("CT TOC Only"), withPlugins(map[string]string{
				"plugins/00-toc.js": `export default function(alloy) {
  alloy.hook('onContentTransformed', { pages: true, pageFields: ["toc"] }, function(page) {
    return { toc: [{ id: 'x', text: 'From TOC Hook', level: 2 }] };
  });
}`,
			}))
			Expect(err).NotTo(HaveOccurred(),
				"html is not applied back when no hook on onContentTransformed "+
					"wants it, so a toc-only return must be accepted (issue #1179)")
			html := result.RenderedContent["about.md"]
			Expect(html).To(ContainSubstring("[From TOC Hook]"),
				"the returned toc must still be applied back")
			Expect(html).To(ContainSubstring("About Body"),
				"the page body must be left unchanged")
		})
	})

	Describe("Node runtime (*ordered.Map returns)", func() {
		// Node and WASM hooks return *ordered.Map, not
		// map[string]interface{} (#1219). The check must cover both shapes,
		// or Node and WASM plugins bypass it. Node plugins register after
		// QuickJS ones regardless of filename, so priority puts them first.
		content := map[string]string{
			"content/about.md":       "---\ntitle: About\nlayout: default\n---\n# About Body",
			"layouts/default.liquid": "<html><body>{{ content }}</body></html>",
		}
		withPlugins := func(plugins map[string]string) map[string]string {
			m := make(map[string]string, len(content)+len(plugins))
			for k, v := range content {
				m[k] = v
			}
			for k, v := range plugins {
				m[k] = v
			}
			return m
		}

		It("names a Node plugin that drops html, not the next one", func() {
			_, err := pipeline.BuildWithContent(hookContractConfig("Node Chain Blame"), withPlugins(map[string]string{
				"plugins/00-first.js": `export const runtime = "node";
export default function(alloy) {
  alloy.hook('onPageRendered', { priority: 10 }, function(page) {
    return { url: page.url };
  });
}`,
				"plugins/01-second.js": victimPlugin("onPageRendered", "{}"),
			}))
			expectContractError(err, "onPageRendered", "00-first")
			Expect(err.Error()).NotTo(ContainSubstring("01-second"),
				"the error must not name the next hook in the chain (issue #1179)")
		})

		It("checks every page in a Node batch, not only the first", func() {
			// Node splits a batch across workers and returns results in
			// input order. Only /zeta/ (item 1) is malformed.
			m := withPlugins(map[string]string{
				"plugins/00-first.js": `export const runtime = "node";
export default function(alloy) {
  alloy.hook('onPageRendered', { priority: 10 }, function(page) {
    if (page.url === '/zeta/') return { url: page.url };
    return page;
  });
}`,
				"plugins/01-second.js": victimPlugin("onPageRendered", "{}"),
			})
			m["content/zeta.md"] = "---\ntitle: Zeta\nlayout: default\n---\n# Zeta Body"
			_, err := pipeline.BuildWithContent(hookContractConfig("Node Batch Every Item"), m)
			expectContractErrorOn(err, "onPageRendered", "00-first", "zeta")
			Expect(err.Error()).NotTo(Or(ContainSubstring("/about/"), ContainSubstring("about.md")),
				"the error must name the page whose return was malformed (issue #1179)")
		})

		It("accepts the page object from a Node plugin (guard)", func() {
			result, err := pipeline.BuildWithContent(hookContractConfig("Node OK"), withPlugins(map[string]string{
				"plugins/00-first.js": `export const runtime = "node";
export default function(alloy) {
  alloy.hook('onPageRendered', { priority: 10 }, function(page) {
    page.html = page.html + '<!-- node-ran -->';
    return page;
  });
}`,
				"plugins/01-second.js": victimPlugin("onPageRendered", "{}"),
			}))
			Expect(err).NotTo(HaveOccurred(),
				"a Node plugin returning the page object is valid — an "+
					"*ordered.Map carrying a string html must be accepted (issue #1179)")
			Expect(result.RenderedContent["about.md"]).To(ContainSubstring("<!-- node-ran --><!-- victim-ran -->"))
		})
	})

	Describe("onFormatRendered (out of scope)", func() {
		It("still builds when a hook returns an object without content (guard)", func() {
			// onFormatRendered rebuilds the payload before every hook, so a
			// malformed return cannot reach the next hook. #1179 does not
			// change it.
			result, err := pipeline.BuildWithContent(hookContractConfig("Format Out Of Scope"), map[string]string{
				"content/about.md":            "---\ntitle: About\nlayout: default\noutputs:\n  - json\n---\nBody.",
				"layouts/default.liquid":      "{{ content }}",
				"layouts/default.json.liquid": `{"title":"{{ page.title }}"}`,
				"plugins/00-first.js": `export default function(alloy) {
  alloy.hook('onFormatRendered', {}, function(payload) {
    return { url: payload.url };
  });
}`,
			})
			Expect(err).NotTo(HaveOccurred(),
				"onFormatRendered is out of scope for #1179 and must be unchanged")
			Expect(result.RenderedContent["about.md"]).To(ContainSubstring(`"title":"About"`),
				"the rendered content must be kept when the return lacks it")
		})
	})

	Describe("BuildIncremental", func() {
		It("reports the offending plugin in the dev-rebuild warning", func() {
			// Incremental rebuilds log every hook error as a warning so the
			// dev server stays up. The contract error travels the same path
			// as a thrown error: a warning naming the offending plugin, not
			// the next one.
			tmpDir := GinkgoT().TempDir()
			for rel, body := range map[string]string{
				"content/about.md":       "---\ntitle: About\nlayout: default\n---\n# About Body",
				"layouts/default.liquid": "<html><body>{{ content }}</body></html>",
				"plugins/00-first.js": `export default function(alloy) {
  alloy.hook('onPageRendered', {}, function(page) {
    return { url: page.url };
  });
}`,
				"plugins/01-second.js": victimPlugin("onPageRendered", "{}"),
			} {
				p := filepath.Join(tmpDir, rel)
				Expect(os.MkdirAll(filepath.Dir(p), 0755)).To(Succeed())
				Expect(os.WriteFile(p, []byte(body), 0644)).To(Succeed())
			}

			cfg := &config.Config{
				Title:       "Incremental Contract",
				BaseURL:     "https://example.com",
				ProjectRoot: tmpDir,
				Build:       config.BuildConfig{Output: filepath.Join(tmpDir, "_site")},
				Structure: config.StructureConfig{
					Content: "content",
					Layouts: "layouts",
				},
			}
			config.ApplyDefaults(cfg)
			registry, hooks, _ := pipeline.DiscoverPlugins(cfg)
			defer registry.Close()
			pipelineState, psErr := pipeline.InitPipelineState(cfg, registry, hooks)
			Expect(psErr).NotTo(HaveOccurred())

			var logBuf bytes.Buffer
			log.SetOutput(&logBuf)
			DeferCleanup(func() { log.SetOutput(os.Stderr) })

			result, err := pipeline.BuildIncremental(cfg, nil, nil, nil,
				pipeline.BuildOptions{PipelineState: pipelineState, CaptureRenderedContent: true})
			Expect(err).NotTo(HaveOccurred(),
				"incremental rebuilds downgrade hook errors to warnings — "+
					"#1179 does not change that")

			logged := logBuf.String()
			Expect(strings.Split(logged, "\n")).To(ContainElement(SatisfyAll(
				ContainSubstring("00-first"),
				ContainSubstring("onPageRendered"),
				ContainSubstring("html"),
			)), "one dev-rebuild warning line must name the event, the plugin "+
				"that returned no html, and the field (issue #1179)")
			Expect(logged).NotTo(ContainSubstring("VICTIM-HANDED-MALFORMED-PAYLOAD"),
				"the malformed return must be caught before it reaches the next "+
					"hook on incremental rebuilds too (issue #1179)")
			Expect(result.RenderedContent["about.md"]).To(ContainSubstring("About Body"),
				"the page keeps its pre-hook html, as it does for a thrown error")
		})
	})
})
