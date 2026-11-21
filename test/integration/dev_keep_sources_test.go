package integration_test

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync/atomic"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"
	"github.com/onsi/gomega/gexec"
)

// ── alloy dev keeps sources after a data file edit (issue #1402) ────────
// When a data file changes in alloy dev, the rebuild reloads the data files
// from disk and keeps every source (plugin, REST and GraphQL) as it was last
// fetched, at startup or the last full rebuild. It doesn't fetch them again.
//   - A source whose as is a data file's key still replaces that file's data.
//   - onDataFetched runs again on each source's value as fetched, so a
//     hook's change to a source isn't applied twice.
//   - alloy.data in QuickJS plugins gets the same data as templates.
//   - If the data directory is deleted, the sources still stay.
//
// Before, the reload replaced all of site.data with the data files, so
// every source was missing until a restart.
//
// This drives the real binary: cmd/dev.go hands the startup build's data to
// the rebuilds, and nothing else can reach that step.
//
// A data edit alone doesn't re-render a page that isn't paginated (issue
// #1288), so each step edits the page too, and waits for its new text.
//
// See PLAN.md → "Sources stay after a data file edit (issue #1402)".

var _ = Describe("alloy dev keeps sources after a data file edit (issue #1402)", Ordered, ContinueOnFailure, func() {
	// The layout prints a data file, two REST sources, a Node plugin source
	// and what the hook and a filter see. products has its own size key,
	// which must still win after a rebuild (issue #1396).
	const layout = "TEAM[{{ site.data.team.name }}] " +
		"PRODUCTS[{{ site.data.products.name }}] " +
		"SIZE[{{ site.data.products.size }}] " +
		"TAGS[{{ site.data.products.tags | join: \",\" }}] " +
		"CLASH[{{ site.data.clash.a }}] " +
		"OWN[{{ site.data.own.name }},{{ site.data.own.calls }}] " +
		"BUILT[{{ site.data.built }}] " +
		"FILTER[{{ \"x\" | from_data }}] " +
		"{{ content }}"

	// The hook adds a tag to the products source and a key of its own. It
	// runs on each source's value as fetched, so the tag appears once.
	const hooks = "export default function(alloy) {\n" +
		"  alloy.hook('onDataFetched', { data: ['*'] }, (d) => ({ ...d,\n" +
		"    products: { ...d.products, tags: [...(d.products && d.products.tags || []), 'hooked'] },\n" +
		"    built: 'yes' }));\n" +
		"  alloy.filter('from_data', () => (alloy.data && alloy.data.products ? alloy.data.products.name : 'none'));\n" +
		"}\n"

	// The Node plugin source counts its calls in module scope.
	const source = "export const runtime = \"node\";\nlet calls = 0;\nexport default function(alloy) {\n" +
		"  alloy.source('own-source', async () => ({ name: 'plugin', calls: ++calls }));\n" +
		"}\n"

	var (
		root, base string
		out        *gbytes.Buffer
		hits       = map[string]*atomic.Int32{"/products": {}, "/clash": {}}
		hasNode    bool
		startup    string
	)

	write := func(rel, body string) {
		GinkgoHelper()
		path := filepath.Join(root, rel)
		Expect(os.MkdirAll(filepath.Dir(path), 0755)).To(Succeed())
		Expect(os.WriteFile(path, []byte(body), 0644)).To(Succeed())
	}

	page := func(text string) string {
		return "---\ntitle: Home\nlayout: default\n---\n" + text + "\n"
	}

	get := func() string {
		resp, err := http.Get(base + "/")
		if err != nil {
			return ""
		}
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		return string(b)
	}

	// served waits until the page shows text, then returns it.
	served := func(text string) string {
		GinkgoHelper()
		var body string
		Eventually(func() string {
			body = get()
			return body
		}, 30*time.Second, 250*time.Millisecond).Should(ContainSubstring("<p>"+text+"</p>"),
			"sanity: alloy dev must rebuild the page after the edit")
		return body
	}

	needNode := func() {
		if !hasNode {
			Skip("node is not installed")
		}
	}

	BeforeAll(func() {
		_, err := exec.LookPath("node")
		hasNode = err == nil

		rest := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if c, ok := hits[r.URL.Path]; ok {
				c.Add(1)
			}
			w.Header().Set("Content-Type", "application/json")
			if r.URL.Path == "/clash" {
				fmt.Fprint(w, `{"a":"source"}`)
				return
			}
			fmt.Fprint(w, `{"name":"widgets","size":"big","tags":["new"]}`)
		}))
		DeferCleanup(rest.Close)

		root = GinkgoT().TempDir()
		config := "title: X\nbaseURL: http://localhost\nsources:\n" +
			"  products:\n    type: rest\n    url: " + rest.URL + "/products\n    as: products\n" +
			"  clash:\n    type: rest\n    url: " + rest.URL + "/clash\n    as: clash\n"
		if hasNode {
			config += "  own:\n    type: plugin\n    plugin: own-source\n    as: own\n"
			write("plugins/source.js", source)
		}
		write("alloy.config.yaml", config)
		write("layouts/default.liquid", layout)
		write("content/index.md", page("start"))
		write("data/team.json", `{"name":"T1"}`)
		write("data/clash.json", `{"a":"file"}`)
		write("plugins/hooks.js", hooks)

		binary, err := gexec.Build("github.com/zeroedin/alloy")
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(gexec.CleanupBuildArtifacts)

		ln, err := net.Listen("tcp", "127.0.0.1:0")
		Expect(err).NotTo(HaveOccurred())
		port := ln.Addr().(*net.TCPAddr).Port
		Expect(ln.Close()).To(Succeed())

		out = gbytes.NewBuffer()
		cmd := exec.Command(binary, "dev", "--port", fmt.Sprint(port))
		cmd.Dir = root
		session, err := gexec.Start(cmd, out, out)
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(func() {
			session.Interrupt()
			Eventually(session, 10*time.Second).Should(gexec.Exit())
		})
		Eventually(out, 30*time.Second).Should(gbytes.Say(`Serving at http://localhost:\d+`))
		base = "http://localhost:" + string(regexp.MustCompile(`Serving at http://localhost:(\d+)`).FindSubmatch(out.Contents())[1])
		startup = served("start")

		// "Serving at" prints before the watcher has registered its
		// folders, so a write right after it can be missed. Rewrite the
		// page until the watcher reports it.
		Eventually(func() bool {
			write("content/index.md", page("start"))
			return strings.Contains(string(out.Contents()), "rebuilding")
		}, 15*time.Second, 250*time.Millisecond).Should(BeTrue(),
			"the watcher must report a change before the test edits a data file")
	})

	// Guard: the startup build has every source, as alloy build does.
	It("has every source at startup (guard)", func() {
		Expect(startup).To(ContainSubstring("TEAM[T1]"))
		Expect(startup).To(ContainSubstring("PRODUCTS[widgets]"))
		Expect(startup).To(ContainSubstring("SIZE[big]"))
		Expect(startup).To(ContainSubstring("TAGS[new,hooked]"))
		Expect(startup).To(ContainSubstring("CLASH[source]"))
		Expect(startup).To(ContainSubstring("BUILT[yes]"))
		Expect(startup).To(ContainSubstring("FILTER[widgets]"))
	})
	It("has the plugin source at startup (guard)", func() {
		needNode()
		Expect(startup).To(ContainSubstring("OWN[plugin,1]"))
	})

	Describe("after a data file edit", func() {
		var body string
		BeforeAll(func() {
			write("data/team.json", `{"name":"T2"}`)
			write("content/index.md", page("edit one"))
			body = served("edit one")
		})

		// Guard: the edited data file is read again from disk.
		It("reads the edited data file again (guard)", func() {
			Expect(body).To(ContainSubstring("TEAM[T2]"))
		})
		It("keeps a REST source", func() {
			Expect(body).To(ContainSubstring("PRODUCTS[widgets]"))
		})
		It("keeps a REST source whose as is a data file's key, over that file", func() {
			Expect(body).To(ContainSubstring("CLASH[source]"))
		})
		It("keeps a plugin source, without calling it again", func() {
			needNode()
			Expect(body).To(ContainSubstring("OWN[plugin,1]"))
		})
		It("keeps a source's own size key winning (issue #1396)", func() {
			Expect(body).To(ContainSubstring("SIZE[big]"))
		})
		It("runs onDataFetched on each source's value as fetched, so its change appears once", func() {
			Expect(body).To(ContainSubstring("TAGS[new,hooked]"))
			Expect(body).To(ContainSubstring("BUILT[yes]"))
		})
		It("gives QuickJS plugins' alloy.data the sources too", func() {
			Expect(body).To(ContainSubstring("FILTER[widgets]"))
		})
		// Guard: the rebuild makes no network call.
		It("doesn't fetch a REST source again (guard)", func() {
			Expect(hits["/products"].Load()).To(BeEquivalentTo(1))
			Expect(hits["/clash"].Load()).To(BeEquivalentTo(1))
		})
	})

	Describe("after a second data file edit", func() {
		var body string
		BeforeAll(func() {
			write("data/team.json", `{"name":"T3"}`)
			write("content/index.md", page("edit two"))
			body = served("edit two")
		})

		It("keeps every source and applies the hook's change once", func() {
			Expect(body).To(ContainSubstring("TEAM[T3]"))
			Expect(body).To(ContainSubstring("PRODUCTS[widgets]"))
			Expect(body).To(ContainSubstring("TAGS[new,hooked]"))
		})
	})

	Describe("after the data directory is deleted", func() {
		var body string
		BeforeAll(func() {
			Expect(os.RemoveAll(filepath.Join(root, "data"))).To(Succeed())
			write("content/index.md", page("edit three"))
			body = served("edit three")
		})

		// Guard: the deleted data file is gone.
		It("drops the deleted data file (guard)", func() {
			Expect(body).To(ContainSubstring("TEAM[]"))
		})
		It("keeps the sources", func() {
			Expect(body).To(ContainSubstring("PRODUCTS[widgets]"))
			Expect(body).To(ContainSubstring("CLASH[source]"))
		})
	})
})

// ── ...when alloy dev starts on a site with no pages (issue #1411) ──────
// A site with no content pages takes an early return in Build. The sources
// must still reach the rebuilds from there, so a data edit doesn't drop
// them. The site starts with a REST source and a data file only. The test
// edits the data file, then adds the page, and checks the source is there.
var _ = Describe("alloy dev keeps sources when it starts with no pages (issue #1411)", Ordered, func() {
	var (
		root, base string
		out        *gbytes.Buffer
	)

	write := func(rel, body string) {
		GinkgoHelper()
		path := filepath.Join(root, rel)
		Expect(os.MkdirAll(filepath.Dir(path), 0755)).To(Succeed())
		Expect(os.WriteFile(path, []byte(body), 0644)).To(Succeed())
	}

	get := func() string {
		resp, err := http.Get(base + "/")
		if err != nil {
			return ""
		}
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		return string(b)
	}

	BeforeAll(func() {
		rest := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{"name":"widgets"}`)
		}))
		DeferCleanup(rest.Close)

		root = GinkgoT().TempDir()
		write("alloy.config.yaml", "title: X\nbaseURL: http://localhost\nsources:\n"+
			"  products:\n    type: rest\n    url: "+rest.URL+"/products\n    as: products\n")
		write("layouts/default.liquid", "TEAM[{{ site.data.team.name }}] PRODUCTS[{{ site.data.products.name }}] {{ content }}")
		write("data/team.json", `{"name":"T1"}`)

		binary, err := gexec.Build("github.com/zeroedin/alloy")
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(gexec.CleanupBuildArtifacts)

		ln, err := net.Listen("tcp", "127.0.0.1:0")
		Expect(err).NotTo(HaveOccurred())
		port := ln.Addr().(*net.TCPAddr).Port
		Expect(ln.Close()).To(Succeed())

		out = gbytes.NewBuffer()
		cmd := exec.Command(binary, "dev", "--port", fmt.Sprint(port))
		cmd.Dir = root
		session, err := gexec.Start(cmd, out, out)
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(func() {
			session.Interrupt()
			Eventually(session, 10*time.Second).Should(gexec.Exit())
		})
		Eventually(out, 30*time.Second).Should(gbytes.Say(`Serving at http://localhost:\d+`))
		base = "http://localhost:" + string(regexp.MustCompile(`Serving at http://localhost:(\d+)`).FindSubmatch(out.Contents())[1])

		// Edit the data file until the watcher reports it. That edit is
		// the data reload under test.
		Eventually(func() bool {
			write("data/team.json", `{"name":"T2"}`)
			return strings.Contains(string(out.Contents()), "rebuilding")
		}, 15*time.Second, 250*time.Millisecond).Should(BeTrue(),
			"the watcher must report the data file edit")
		write("content/index.md", "---\ntitle: Home\nlayout: default\n---\nnew page\n")
	})

	It("keeps a REST source on a page added after a data file edit", func() {
		var body string
		Eventually(func() string {
			body = get()
			return body
		}, 30*time.Second, 250*time.Millisecond).Should(ContainSubstring("<p>new page</p>"),
			"sanity: alloy dev must render the added page")
		Expect(body).To(ContainSubstring("TEAM[T2]"), "sanity: the page sees the edited data file")
		Expect(body).To(ContainSubstring("PRODUCTS[widgets]"))
	})
})
