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

// ── alloy dev never reports success while the site is incomplete (#1316) ─
// A full rebuild empties _site before it renders. When it fails, the next
// edit used to run a small incremental rebuild that rendered only the
// pages that changed. It reported success and cleared the overlay while
// every other page, the static files and sitemap.xml still returned 404.
//
// This drives the real binary: the full-rebuild decision lives in the
// alloy dev command's watcher loop, which nothing else can reach.
//
// See PLAN.md → "alloy dev never reports success while the site is
// incomplete (issue #1316)".

var _ = Describe("alloy dev after a failed full rebuild (issue #1316)", Ordered, func() {
	var binary string

	BeforeAll(func() {
		var err error
		binary, err = gexec.Build("github.com/zeroedin/alloy")
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(gexec.CleanupBuildArtifacts)
	})

	page := func(n int, layout string) string {
		return fmt.Sprintf("---\ntitle: N%d\nlayout: %s\n---\nn%d\n", n, layout, n)
	}

	freePort := func() int {
		GinkgoHelper()
		ln, err := net.Listen("tcp", "127.0.0.1:0")
		Expect(err).NotTo(HaveOccurred())
		defer ln.Close()
		return ln.Addr().(*net.TCPAddr).Port
	}

	status := func(base, path string) int {
		resp, err := http.Get(base + path)
		if err != nil {
			return 0
		}
		resp.Body.Close()
		return resp.StatusCode
	}

	It("brings the whole site back when the error is fixed", func() {
		root := GinkgoT().TempDir()
		write := func(rel, body string) {
			GinkgoHelper()
			path := filepath.Join(root, rel)
			Expect(os.MkdirAll(filepath.Dir(path), 0755)).To(Succeed())
			Expect(os.WriteFile(path, []byte(body), 0644)).To(Succeed())
		}
		write("alloy.config.yaml", "title: X\nbaseURL: http://localhost\n")
		write("layouts/default.liquid", "<html><body>{{ content }}</body></html>")
		write("static/site.css", "body{}")
		for i := 1; i <= 12; i++ {
			write(fmt.Sprintf("content/n%d.md", i), page(i, "default"))
		}

		out := gbytes.NewBuffer()
		cmd := exec.Command(binary, "dev", "--port", fmt.Sprint(freePort()))
		cmd.Dir = root
		session, err := gexec.Start(cmd, out, out)
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(func() {
			session.Interrupt()
			Eventually(session, 10*time.Second).Should(gexec.Exit())
		})

		Eventually(out, 30*time.Second).Should(gbytes.Say(`Serving at http://localhost:\d+`))
		port := regexp.MustCompile(`Serving at http://localhost:(\d+)`).FindSubmatch(out.Contents())[1]
		base := "http://localhost:" + string(port)
		Eventually(func() int { return status(base, "/n1/") }, 10*time.Second).Should(Equal(200),
			"sanity: the initial build serves the site")

		// More than 10 changes at once make alloy dev run a full rebuild.
		// One page names a layout that doesn't exist, so it fails.
		for i := 1; i <= 10; i++ {
			write(fmt.Sprintf("static/extra%d.css", i), "a{}")
		}
		write("content/n5.md", page(5, "nonexistent"))
		Eventually(out, 10*time.Second).Should(gbytes.Say(`rebuild failed: layout "nonexistent" not found`),
			"sanity: the full rebuild fails")

		// Fix the one broken page with a single edit.
		write("content/n5.md", page(5, "default"))
		Eventually(out, 30*time.Second).Should(gbytes.Say(`rebuild complete`))

		for _, path := range []string{"/n1/", "/n5/", "/n12/", "/site.css", "/extra1.css", "/sitemap.xml"} {
			Expect(status(base, path)).To(Equal(200),
				"after the fix reports success, %s must be served: the rebuild after a failed "+
					"full rebuild has to be a full one, not an incremental one", path)
		}
	})
})
