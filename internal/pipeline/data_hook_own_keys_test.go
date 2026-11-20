package pipeline_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/zeroedin/alloy/internal/config"
	"github.com/zeroedin/alloy/internal/fetch"
	"github.com/zeroedin/alloy/internal/pipeline"
	"github.com/zeroedin/alloy/internal/plugin"
)

// ── What .size and .first give after a data hook (issue #1396) ──────────
// onDataFetched and onAfterValidation write what they return back into
// site.data. After either hook, each top-level entry reads size and first
// by where it came from, at every depth:
//   - a data file: the key count and the first pair, as without a hook
//     (issue #1363), even when the hook changed or replaced it;
//   - every other entry (a plugin, REST or GraphQL source, or a key a hook
//     adds): its own size and first keys win, as a source's do without a
//     hook (issue #1392).
//
// Before, onDataFetched made every entry read like a data file, and
// onAfterValidation made every entry, data files included, let its own
// keys win. So each check below fails after one of the two hooks and
// guards the other.
//
// See PLAN.md → "What .size and .first give after a data hook".
var _ = Describe("What .size and .first give after a data hook (issue #1396)", func() {
	const node = "export const runtime = \"node\";\n"

	// Every object has its own size and first keys, and three keys, so the
	// two readings differ: own keys give the value, a data file gives 3 and
	// the first pair, printed as key then value (firsty). A data file's keys
	// are in sorted order, so its first pair doesn't depend on keeping file
	// order after onAfterValidation, which is issue #1401.
	const (
		fileJSON   = `{"first":"y","size":"x","zeta":1}`
		editedJSON = `{"first":"f","size":"e","zeta":1}`
		clashJSON  = `{"a":1}`
		restJSON   = `{"size":"rest","first":"r","zeta":1}`
		clashREST  = `{"size":"src","first":"s","zeta":1}`
	)

	// The layout prints every entry's size and first. built.inner is an
	// object inside a hook's object, so own keys must win at every depth.
	const layout = "FILE[{{ site.data.file.size }},{{ site.data.file.first }}] " +
		"EDITED[{{ site.data.edited.size }},{{ site.data.edited.first }}] " +
		"REST[{{ site.data.rest.size }},{{ site.data.rest.first }}] " +
		"CLASH[{{ site.data.clash.size }},{{ site.data.clash.first }}] " +
		"SOURCE[{{ site.data.own.size }},{{ site.data.own.first }}] " +
		"BUILT[{{ site.data.built.size }},{{ site.data.built.first }},{{ site.data.built.inner.size }}] " +
		"COPIED[{{ site.data.copied.size }},{{ site.data.copied.first }}] " +
		"{{ content }}"

	// built is a new object. copied is the file data file's object, put
	// under a new key: it's judged by its key, so its own keys win. edited
	// is a data file the hook changes: it still reads like a data file,
	// now with four keys. The added key sorts last, as above.
	const additions = `edited: { ...d.edited, zulu: 2 }, ` +
		`built: { size: 'big', first: 'one', zeta: 1, inner: { size: 'deep', zeta: 2 } }, ` +
		`copied: d.file`

	hooks := map[string]string{
		"onDataFetched": "export default function(alloy) {\n" +
			"  alloy.hook('onDataFetched', { data: ['*'] }, (d) => ({ ...d, " + additions + " }));\n" +
			"}\n",
		"onAfterValidation": "export default function(alloy) {\n" +
			"  alloy.hook('onAfterValidation', { data: ['*'] }, (p) => {\n" +
			"    const d = p.cascade;\n" +
			"    return { cascade: { ...d, " + additions + " } };\n" +
			"  });\n" +
			"}\n",
	}

	// rest serves the REST sources. /clash is a source whose as is the
	// same key as data/clash.json.
	var rest *httptest.Server
	BeforeEach(func() {
		rest = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			if r.URL.Path == "/clash" {
				fmt.Fprint(w, clashREST)
				return
			}
			fmt.Fprint(w, restJSON)
		}))
		DeferCleanup(rest.Close)
	})

	// build runs a full build with hookSrc as the only hook plugin, or none.
	// A Node plugin source (as: own) is added only when Node is installed.
	build := func(hookSrc string) string {
		GinkgoHelper()
		fetch.ResetPluginSources()
		DeferCleanup(fetch.ResetPluginSources)
		cfg := &config.Config{
			Title:   "Data Hook Own Keys",
			BaseURL: "https://example.com",
			Build:   config.BuildConfig{Output: "_site"},
		}
		cfg.Sources = map[string]*config.SourceConfig{
			"rest":  {Type: "rest", URL: rest.URL + "/rest", As: "rest"},
			"clash": {Type: "rest", URL: rest.URL + "/clash", As: "clash"},
		}
		files := map[string]string{
			"content/index.md":       "---\ntitle: Home\nlayout: default\n---\nhi",
			"layouts/default.liquid": layout,
			"data/file.json":         fileJSON,
			"data/edited.json":       editedJSON,
			"data/clash.json":        clashJSON,
		}
		if plugin.CheckNodeAvailable() == nil {
			cfg.Sources["own"] = &config.SourceConfig{Type: "plugin", Plugin: "own-source", As: "own"}
			files["plugins/source.js"] = node + "export default function(alloy) {\n" +
				"  alloy.source('own-source', async () => ({ size: 'large', first: 'primary', zeta: 1 }));\n" +
				"}\n"
		}
		if hookSrc != "" {
			files["plugins/hook.js"] = hookSrc
		}
		result, err := pipeline.BuildWithContent(cfg, files, pipeline.BuildOptions{CaptureRenderedContent: true})
		Expect(err).NotTo(HaveOccurred())
		return result.RenderedContent["index.md"]
	}

	needNode := func() {
		if plugin.CheckNodeAvailable() != nil {
			Skip("node is not installed")
		}
	}

	// Guard: what each entry gives with no hook. The rule keeps this.
	Describe("with no hook (guard)", Ordered, ContinueOnFailure, func() {
		var out string
		BeforeAll(func() { out = build("") })

		It("gives a data file's key count and first pair", func() {
			Expect(out).To(ContainSubstring("FILE[3,firsty]"))
			Expect(out).To(ContainSubstring("EDITED[3,firstf]"))
		})
		It("gives a REST source's own size and first keys", func() {
			Expect(out).To(ContainSubstring("REST[rest,r]"))
		})
		It("gives the own keys of a REST source whose as is a data file's key", func() {
			Expect(out).To(ContainSubstring("CLASH[src,s]"))
		})
		It("gives a plugin source's own size and first keys", func() {
			needNode()
			Expect(out).To(ContainSubstring("SOURCE[large,primary]"))
		})
	})

	for _, h := range []string{"onDataFetched", "onAfterValidation"} {
		for _, runtime := range []string{"QuickJS", "Node"} {
			hookName, rt := h, runtime
			Describe("after an "+hookName+" hook on "+rt, Ordered, ContinueOnFailure, func() {
				var out string
				BeforeAll(func() {
					src := hooks[hookName]
					if rt == "Node" {
						needNode()
						src = node + src
					}
					out = build(src)
				})

				It("gives a data file's key count and first pair", func() {
					Expect(out).To(ContainSubstring("FILE[3,firsty]"))
				})
				It("gives the key count and first pair of a data file the hook changed", func() {
					Expect(out).To(ContainSubstring("EDITED[4,firstf]"))
				})
				It("gives a REST source's own size and first keys", func() {
					Expect(out).To(ContainSubstring("REST[rest,r]"))
				})
				It("gives the own keys of a REST source whose as is a data file's key", func() {
					Expect(out).To(ContainSubstring("CLASH[src,s]"))
				})
				It("gives a plugin source's own size and first keys", func() {
					needNode()
					Expect(out).To(ContainSubstring("SOURCE[large,primary]"))
				})
				It("gives the own keys of an object the hook adds, at every depth", func() {
					Expect(out).To(ContainSubstring("BUILT[big,one,deep]"))
				})
				It("gives the own keys of a data file's object the hook copies to a new key", func() {
					Expect(out).To(ContainSubstring("COPIED[x,y]"))
				})
			})
		}
	}

	// A key onDataFetched adds is a hook's key in onAfterValidation too:
	// passing through the second hook doesn't make it a data file.
	Describe("after onDataFetched then onAfterValidation", Ordered, ContinueOnFailure, func() {
		var out string
		BeforeAll(func() {
			out = build("export default function(alloy) {\n" +
				"  alloy.hook('onDataFetched', { data: ['*'] }, (d) => ({ ...d, built: { size: 'big', first: 'one', zeta: 1, inner: { size: 'deep', zeta: 2 } } }));\n" +
				"  alloy.hook('onAfterValidation', { data: ['*'] }, (p) => ({ cascade: p.cascade }));\n" +
				"}\n")
		})

		It("gives the own keys of an object onDataFetched added", func() {
			Expect(out).To(ContainSubstring("BUILT[big,one,deep]"))
		})
		It("gives a data file's key count and first pair", func() {
			Expect(out).To(ContainSubstring("FILE[3,firsty]"))
		})
	})

	// alloy dev runs onDataFetched again after a data file edit. The rule
	// holds there too. Only data files and the hook's object are checked:
	// sources are missing from site.data after a data edit (issue #1402).
	Describe("in an alloy dev rebuild after a data file edit", Ordered, ContinueOnFailure, func() {
		var full, rebuilt string
		BeforeAll(func() {
			root := GinkgoT().TempDir()
			write := func(rel, content string) {
				GinkgoHelper()
				p := filepath.Join(root, rel)
				Expect(os.MkdirAll(filepath.Dir(p), 0o755)).To(Succeed())
				Expect(os.WriteFile(p, []byte(content), 0o644)).To(Succeed())
			}
			read := func() string {
				GinkgoHelper()
				b, err := os.ReadFile(filepath.Join(root, "_site", "index.html"))
				Expect(err).NotTo(HaveOccurred())
				return string(b)
			}
			write("content/index.md", "---\ntitle: Home\nlayout: default\n---\nhi")
			write("layouts/default.liquid", layout)
			write("data/file.json", fileJSON)
			write("data/edited.json", editedJSON)
			write("plugins/hook.js", hooks["onDataFetched"])
			cfg := &config.Config{
				Title:       "Data Hook Own Keys",
				BaseURL:     "https://example.com",
				ProjectRoot: root,
				Build:       config.BuildConfig{Output: filepath.Join(root, "_site")},
				Structure:   config.StructureConfig{Content: "content", Layouts: "layouts", Data: "data"},
			}
			config.ApplyDefaults(cfg)
			registry, hookRegistry, _ := pipeline.DiscoverPlugins(cfg)
			DeferCleanup(registry.Close)
			s, err := pipeline.InitPipelineState(cfg, registry, hookRegistry)
			Expect(err).NotTo(HaveOccurred())
			first, err := pipeline.Build(cfg, pipeline.BuildOptions{PipelineState: s})
			Expect(err).NotTo(HaveOccurred())
			full = read()

			// A data edit alone doesn't re-render a page that isn't
			// paginated (issue #1288), so the page is edited too.
			write("data/edited.json", `{"first":"f","size":"e","zeta":5}`)
			write("content/index.md", "---\ntitle: Home\nlayout: default\n---\nhi again")
			_, err = pipeline.BuildIncremental(cfg, nil, first.Cache,
				[]string{"data/edited.json", "content/index.md"}, pipeline.BuildOptions{PipelineState: s})
			Expect(err).NotTo(HaveOccurred())
			rebuilt = read()
		})

		It("gives the own keys of the hook's object in the full build", func() {
			Expect(full).To(ContainSubstring("BUILT[big,one,deep]"))
		})
		It("gives the own keys of the hook's object after the rebuild", func() {
			Expect(rebuilt).To(ContainSubstring("<p>hi again</p>"))
			Expect(rebuilt).To(ContainSubstring("BUILT[big,one,deep]"))
			Expect(rebuilt).To(ContainSubstring("COPIED[x,y]"))
		})
		It("gives data files' key counts after the rebuild", func() {
			Expect(rebuilt).To(ContainSubstring("FILE[3,firsty]"))
			Expect(rebuilt).To(ContainSubstring("EDITED[4,firstf]"))
		})
	})
})
