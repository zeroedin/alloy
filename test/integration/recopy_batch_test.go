package integration_test

import (
	"fmt"
	"io"
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

// ── A recopy conflict is judged when the batch settles (issue #1339) ─────
// The watcher hands alloy dev and alloy serve a batch of file changes. A
// changed static or asset file is recopied, and a copy to a path another
// source owns is an output path conflict. Each copy used to be checked in
// event order, so a new file created just before the old owner was deleted
// was reported as a conflict, even though the files left at the end don't
// conflict. alloy serve exited with code 1, alloy dev kept a stale overlay,
// and neither copied the new file.
//
// The rule: a conflict counts only if it is still there once the whole batch
// is applied, as a build of the final files would see it.
//
// This drives the real binary, because the recopy loop lives in the
// alloy dev and alloy serve commands.
//
// See PLAN.md → "Recopy conflicts are judged when the batch settles
// (issue #1339)".

var _ = Describe("a recopy conflict is judged when the batch settles (issue #1339)", func() {
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

	// static/site.css owns /site.css. The assets folder exists from the
	// start, so the watcher is already watching it.
	newSite := func() string {
		GinkgoHelper()
		root := GinkgoT().TempDir()
		write(root, "alloy.config.yaml", "title: X\nbaseURL: http://localhost\n")
		write(root, "layouts/default.liquid", "<html><body>{{ content }}</body></html>")
		write(root, "content/index.md", "---\ntitle: Home\nlayout: default\n---\nhome\n")
		write(root, "content/about.md", "---\ntitle: About\nlayout: default\n---\nabout\n")
		write(root, "static/site.css", "static{}")
		write(root, "assets/keep.txt", "x")
		return root
	}

	freePort := func() int {
		GinkgoHelper()
		ln, err := net.Listen("tcp", "127.0.0.1:0")
		Expect(err).NotTo(HaveOccurred())
		defer ln.Close()
		return ln.Addr().(*net.TCPAddr).Port
	}

	start := func(command, root string) running {
		GinkgoHelper()
		out := gbytes.NewBuffer()
		cmd := exec.Command(binary, command, "--port", fmt.Sprint(freePort()))
		cmd.Dir = root
		session, err := gexec.Start(cmd, out, out)
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(func() {
			session.Interrupt()
			Eventually(session, 10*time.Second).Should(gexec.Exit())
		})
		Eventually(out, 30*time.Second).Should(gbytes.Say(`Serving at http://localhost:\d+`))
		port := regexp.MustCompile(`Serving at http://localhost:(\d+)`).FindSubmatch(out.Contents())[1]
		return running{root: root, base: "http://localhost:" + string(port), out: out, session: session}
	}

	body := func(url string) string {
		resp, err := http.Get(url)
		if err != nil {
			return ""
		}
		defer resp.Body.Close()
		if resp.StatusCode != 200 {
			return fmt.Sprintf("status %d", resp.StatusCode)
		}
		b, _ := io.ReadAll(resp.Body)
		return string(b)
	}

	// "Serving at" prints before the watcher has registered its folders, so
	// a write right after it can be missed. Touch a harmless asset until its
	// copy is served, then make the change under test.
	waitWatching := func(r running) {
		GinkgoHelper()
		Eventually(func() string {
			write(r.root, "assets/ping.txt", "ping")
			return body(r.base + "/ping.txt")
		}, 15*time.Second, 250*time.Millisecond).Should(Equal("ping"),
			"the watcher must copy a changed asset before the test writes its input")
	}

	// In one batch: the new owner of site.css is created first, then the old
	// owner is deleted. The files left at the end don't conflict.
	moveOwner := func(r running) {
		GinkgoHelper()
		write(r.root, "assets/site.css", "asset{}")
		Expect(os.Remove(filepath.Join(r.root, "static/site.css"))).To(Succeed())
	}

	It("keeps alloy serve running and copies the new file", func() {
		r := start("serve", newSite())
		Expect(body(r.base+"/site.css")).To(Equal("static{}"), "sanity: the static file owns /site.css")
		waitWatching(r)

		moveOwner(r)

		Eventually(func() string { return body(r.base + "/site.css") }, 10*time.Second).Should(Equal("asset{}"),
			"once the batch settles, the asset is the only owner of site.css, so it must be copied")
		Consistently(r.session, 2*time.Second).ShouldNot(gexec.Exit(),
			"no conflict is left at the end of the batch, so alloy serve must keep running")
		Expect(string(r.out.Contents())).NotTo(ContainSubstring("output path conflict"))
	})

	It("copies the new file in alloy dev and shows no overlay", func() {
		r := start("dev", newSite())
		Expect(body(r.base+"/site.css")).To(Equal("static{}"), "sanity: the static file owns /site.css")
		waitWatching(r)

		moveOwner(r)

		Eventually(func() string { return body(r.base + "/site.css") }, 10*time.Second).Should(Equal("asset{}"),
			"once the batch settles, the asset is the only owner of site.css, so it must be copied")
		Expect(body(r.base+"/about/")).NotTo(ContainSubstring(`id="alloy-error-overlay"`),
			"no conflict is left at the end of the batch, so the overlay must not show one")
	})

	// Guard: a conflict still there at the end of the batch still counts.
	It("still stops alloy serve when the conflict remains", func() {
		r := start("serve", newSite())
		waitWatching(r)

		write(r.root, "assets/site.css", "asset{}")

		Eventually(r.session, 15*time.Second).Should(gexec.Exit(1),
			"static/site.css still owns the path, so the new asset is a real conflict")
		Expect(strings.Contains(string(r.out.Contents()), "output path conflict: site.css")).To(BeTrue())
	})
})
