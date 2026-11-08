package integration_test

import (
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"
	"github.com/onsi/gomega/gexec"
)

// ── alloy dev never reports success while the site is incomplete (#1316) ─
// A full build empties _site before it renders. When it fails, whether it
// is the startup build or a full rebuild, the next edit used to run a small
// incremental rebuild that rendered only the pages that changed. It
// reported success and cleared the overlay while static files, sitemap.xml
// and (after a full rebuild) every other page still returned 404.
//
// This drives the real binary: the full-rebuild decision lives in the
// alloy dev command's watcher loop, which nothing else can reach.
//
// See PLAN.md → "alloy dev never reports success while the site is
// incomplete (issue #1316)".

var _ = Describe("alloy dev after a failed full build (issue #1316)", func() {
	var binary string

	BeforeEach(func() {
		var err error
		binary, err = gexec.Build("github.com/zeroedin/alloy")
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(gexec.CleanupBuildArtifacts)
	})

	type running struct {
		root string
		base string
		out  *gbytes.Buffer
	}

	page := func(n int, layout string) string {
		return fmt.Sprintf("---\ntitle: N%d\nlayout: %s\n---\nn%d\n", n, layout, n)
	}

	write := func(root, rel, body string) {
		GinkgoHelper()
		path := filepath.Join(root, rel)
		Expect(os.MkdirAll(filepath.Dir(path), 0755)).To(Succeed())
		Expect(os.WriteFile(path, []byte(body), 0644)).To(Succeed())
	}

	// newSite has 12 pages and a stylesheet. brokenPage, when non-zero,
	// names a layout that doesn't exist.
	newSite := func(brokenPage int) string {
		GinkgoHelper()
		root := GinkgoT().TempDir()
		write(root, "alloy.config.yaml", "title: X\nbaseURL: http://localhost\n")
		write(root, "layouts/default.liquid", "<html><body>{{ content }}</body></html>")
		write(root, "static/site.css", "body{}")
		for i := 1; i <= 12; i++ {
			layout := "default"
			if i == brokenPage {
				layout = "nonexistent"
			}
			write(root, fmt.Sprintf("content/n%d.md", i), page(i, layout))
		}
		return root
	}

	freePort := func() int {
		GinkgoHelper()
		ln, err := net.Listen("tcp", "127.0.0.1:0")
		Expect(err).NotTo(HaveOccurred())
		defer ln.Close()
		return ln.Addr().(*net.TCPAddr).Port
	}

	status := func(url string) int {
		resp, err := http.Get(url)
		if err != nil {
			return 0
		}
		resp.Body.Close()
		return resp.StatusCode
	}

	dev := func(root string) running {
		GinkgoHelper()
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
		return running{root: root, base: "http://localhost:" + string(port), out: out}
	}

	// "Serving at" prints before the watcher has registered its folders, so
	// a write right after it can be missed. Rewrite a page with its own
	// content until the watcher reports it, then make the change under test.
	waitWatching := func(r running) {
		GinkgoHelper()
		Eventually(func() bool {
			write(r.root, "content/n1.md", page(1, "default"))
			return strings.Contains(string(r.out.Contents()), "rebuilding")
		}, 15*time.Second, 250*time.Millisecond).Should(BeTrue(),
			"the watcher must report a change before the test writes its input")
	}

	expectWholeSite := func(r running, why string) {
		GinkgoHelper()
		for _, path := range []string{"/n1/", "/n5/", "/n12/", "/site.css", "/sitemap.xml"} {
			Expect(status(r.base+path)).To(Equal(200),
				"after the fix reports success, %s must be served: %s", path, why)
		}
	}

	It("brings the whole site back after a failed full rebuild", func() {
		r := dev(newSite(0))
		Eventually(func() int { return status(r.base + "/n1/") }, 10*time.Second).Should(Equal(200),
			"sanity: the initial build serves the site")
		waitWatching(r)

		// More than 10 changes at once make alloy dev run a full rebuild.
		// One page names a layout that doesn't exist, so it fails.
		for i := 1; i <= 10; i++ {
			write(r.root, fmt.Sprintf("static/extra%d.css", i), "a{}")
		}
		write(r.root, "content/n5.md", page(5, "nonexistent"))
		Eventually(r.out, 15*time.Second).Should(gbytes.Say(`rebuild failed: layout "nonexistent" not found`),
			"sanity: the full rebuild fails")

		// Fix the one broken page with a single edit.
		write(r.root, "content/n5.md", page(5, "default"))
		Eventually(r.out, 30*time.Second).Should(gbytes.Say(`rebuild complete`))

		expectWholeSite(r, "the rebuild after a failed full rebuild has to be a full one")
		Expect(status(r.base+"/extra1.css")).To(Equal(200),
			"static files added during the failed rebuild must be copied too")
	})

	It("brings the whole site back after a failed startup build", func() {
		r := dev(newSite(5))
		Expect(string(r.out.Contents())).To(ContainSubstring("initial build failed"),
			"sanity: the startup build fails")
		waitWatching(r)

		// Fix the one broken page with a single edit.
		write(r.root, "content/n5.md", page(5, "default"))
		Eventually(r.out, 30*time.Second).Should(gbytes.Say(`rebuild complete`))

		expectWholeSite(r, "the startup build is a full build, so the rebuild after it failed has to be full too")
	})
})
