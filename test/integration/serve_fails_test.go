package integration_test

import (
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"
	"github.com/onsi/gomega/gexec"
)

// ── alloy serve fails like alloy build ──────────────────────────────────
// alloy serve is the production preview. A build error, at startup or on a
// rebuild, stops the server with exit code 1, as alloy build does. Before,
// a failed rebuild kept the server running over an emptied _site, so every
// page returned 404 and the error only showed in the terminal.
//
// This drives the real binary, because the rebuild loop lives in the
// alloy serve command.
//
// See PLAN.md → "alloy serve — fail like alloy build".

var _ = Describe("alloy serve fails like alloy build", func() {
	var binary string

	BeforeEach(func() {
		var err error
		binary, err = gexec.Build("github.com/zeroedin/alloy")
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(gexec.CleanupBuildArtifacts)
	})

	type running struct {
		root    string
		base    string
		out     *gbytes.Buffer
		session *gexec.Session
	}

	write := func(root, rel, body string) {
		GinkgoHelper()
		path := filepath.Join(root, rel)
		Expect(os.MkdirAll(filepath.Dir(path), 0755)).To(Succeed())
		Expect(os.WriteFile(path, []byte(body), 0644)).To(Succeed())
	}

	page := func(title, layout string) string {
		return fmt.Sprintf("---\ntitle: %s\nlayout: %s\n---\n%s\n", title, layout, title)
	}

	newSite := func() string {
		GinkgoHelper()
		root := GinkgoT().TempDir()
		write(root, "alloy.config.yaml", "title: X\nbaseURL: http://localhost\n")
		write(root, "layouts/default.liquid", "<html><body>{{ content }}</body></html>")
		write(root, "content/index.md", page("Home", "default"))
		write(root, "content/about.md", page("About", "default"))
		write(root, "static/site.css", "body{}")
		return root
	}

	freePort := func() int {
		GinkgoHelper()
		ln, err := net.Listen("tcp", "127.0.0.1:0")
		Expect(err).NotTo(HaveOccurred())
		defer ln.Close()
		return ln.Addr().(*net.TCPAddr).Port
	}

	serve := func(root string) running {
		GinkgoHelper()
		out := gbytes.NewBuffer()
		cmd := exec.Command(binary, "serve", "--port", fmt.Sprint(freePort()))
		cmd.Dir = root
		session, err := gexec.Start(cmd, out, out)
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(func() {
			session.Interrupt()
			Eventually(session, 10*time.Second).Should(gexec.Exit())
		})
		return running{root: root, out: out, session: session}
	}

	waitServing := func(r *running) {
		GinkgoHelper()
		Eventually(r.out, 30*time.Second).Should(gbytes.Say(`Serving at http://localhost:\d+`))
		port := regexp.MustCompile(`Serving at http://localhost:(\d+)`).FindSubmatch(r.out.Contents())[1]
		r.base = "http://localhost:" + string(port)
	}

	status := func(url string) int {
		resp, err := http.Get(url)
		if err != nil {
			return 0
		}
		resp.Body.Close()
		return resp.StatusCode
	}

	It("exits 1 when a rebuild fails", func() {
		r := serve(newSite())
		waitServing(&r)

		write(r.root, "content/about.md", page("About", "nonexistent"))

		Eventually(r.session, 15*time.Second).Should(gexec.Exit(1),
			"a failed rebuild must stop alloy serve with exit code 1, as alloy build does, "+
				"instead of serving an emptied _site")
		Expect(string(r.out.Contents())).To(ContainSubstring(`layout "nonexistent" not found for page "about.md"`),
			"the error must be printed before exiting")
	})

	It("removes its lockfile when a rebuild fails", func() {
		r := serve(newSite())
		waitServing(&r)
		Expect(filepath.Join(r.root, ".alloy", "server.lock")).To(BeAnExistingFile(),
			"sanity: a running alloy serve writes its lockfile")

		write(r.root, "content/about.md", page("About", "nonexistent"))

		Eventually(r.session, 15*time.Second).Should(gexec.Exit())
		Expect(filepath.Join(r.root, ".alloy", "server.lock")).NotTo(BeAnExistingFile(),
			"a server stopped by a build error must clean up like one stopped by Ctrl-C")
	})

	// alloy build fails on an output path conflict (issue #1238), so a
	// conflict found while recopying a changed static file stops serve too.
	It("exits 1 when a changed static file claims a page's output path", func() {
		root := newSite()
		// The folder exists from the start, so the watcher is already
		// watching it when the conflicting file appears.
		write(root, "static/about/photo.txt", "x")
		r := serve(root)
		waitServing(&r)

		write(r.root, "static/about/index.html", "<p>static about</p>")

		Eventually(r.session, 15*time.Second).Should(gexec.Exit(1),
			"an output path conflict fails alloy build, so it must stop alloy serve")
		Expect(string(r.out.Contents())).To(ContainSubstring("output path conflict: about/index.html"))
	})

	It("exits 1 when the startup build fails", func() {
		root := newSite()
		write(root, "content/about.md", page("About", "nonexistent"))
		r := serve(root)

		Eventually(r.session, 15*time.Second).Should(gexec.Exit(1))
		Expect(string(r.out.Contents())).NotTo(ContainSubstring("Serving at"),
			"a site that doesn't build is never served")
	})

	// Guard: a rebuild that succeeds keeps the server running.
	It("keeps serving after a rebuild that succeeds", func() {
		r := serve(newSite())
		waitServing(&r)

		write(r.root, "content/about.md", page("About edited", "default"))

		Eventually(r.out, 30*time.Second).Should(gbytes.Say(`rebuild complete`))
		Consistently(r.session, 2*time.Second).ShouldNot(gexec.Exit())
		Expect(status(r.base + "/about/")).To(Equal(200))
		Expect(status(r.base + "/site.css")).To(Equal(200))
	})
})
