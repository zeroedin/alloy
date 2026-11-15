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
		about := body(r.base + "/about/")
		Expect(about).To(ContainSubstring("<body>"),
			"sanity: /about/ must be served as HTML, or a missing overlay proves nothing")
		Expect(about).NotTo(ContainSubstring(`id="alloy-error-overlay"`),
			"no conflict is left at the end of the batch, so the overlay must not show one")
	})

	// A file created and deleted in the same batch leaves nothing to copy.
	// Its conflict is gone with it, even though static/site.css still owns
	// the path.
	It("keeps alloy serve running when the new file is deleted in the same batch", func() {
		r := start("serve", newSite())
		waitWatching(r)
		batches := strings.Count(string(r.out.Contents()), "rebuilding")

		write(r.root, "assets/site.css", "asset{}")
		Expect(os.Remove(filepath.Join(r.root, "assets/site.css"))).To(Succeed())

		Eventually(func() int { return strings.Count(string(r.out.Contents()), "rebuilding") }, 10*time.Second).
			Should(BeNumerically(">", batches), "sanity: the watcher handles the batch")
		Consistently(r.session, 2*time.Second).ShouldNot(gexec.Exit(),
			"assets/site.css is gone at the end of the batch, so no conflict is left")
		Expect(body(r.base+"/site.css")).To(Equal("static{}"), "the static file still owns /site.css")
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

	// ── Other owners and later batches (issue #1343) ─────────────────────
	// The old owner can be a passthrough file or a page, and a conflict can
	// outlive the batch that found it. These cases check the rule from
	// #1339 holds for each.

	// siteWith builds a site from the given files, on top of a config, a
	// layout and two pages. A passthrough mapping vendor → "" puts vendor/
	// files at the site root.
	siteWith := func(files map[string]string) string {
		GinkgoHelper()
		root := GinkgoT().TempDir()
		write(root, "alloy.config.yaml",
			"title: X\nbaseURL: http://localhost\npassthrough:\n  - from: vendor\n    to: \"\"\n")
		write(root, "layouts/default.liquid", "<html><body>{{ content }}</body></html>")
		write(root, "content/index.md", "---\ntitle: Home\nlayout: default\n---\nhome\n")
		write(root, "content/about.md", "---\ntitle: About\nlayout: default\n---\nabout\n")
		write(root, "assets/keep.txt", "x")
		write(root, "vendor/vendor-keep.txt", "x")
		write(root, "static/about/keep.txt", "x")
		for rel, b := range files {
			write(root, rel, b)
		}
		return root
	}

	// settle writes a marker asset in the same batch as the change under
	// test, then waits until it is served, so the batch has been handled.
	settle := func(r running, name string) {
		GinkgoHelper()
		write(r.root, "assets/"+name, name)
		Eventually(func() string { return body(r.base + "/" + name) }, 10*time.Second).
			Should(Equal(name), "sanity: the watcher handles the batch")
	}

	output := func(r running, rel string) string {
		GinkgoHelper()
		b, err := os.ReadFile(filepath.Join(r.root, "_site", rel))
		Expect(err).NotTo(HaveOccurred(), "%s must exist in the output", rel)
		return string(b)
	}

	// The overlay is added to every HTML page while an error is active.
	home := func(r running) string {
		GinkgoHelper()
		page := body(r.base + "/")
		Expect(page).To(ContainSubstring("<body>"), "sanity: / must be served as HTML")
		return page
	}

	for _, command := range []string{"serve", "dev"} {
		command := command
		It("frees a path when the passthrough file that owned it is deleted ("+command+")", func() {
			r := start(command, siteWith(map[string]string{"vendor/site.css": "vendor{}"}))
			Expect(body(r.base+"/site.css")).To(Equal("vendor{}"), "sanity: the passthrough file owns /site.css")
			waitWatching(r)

			write(r.root, "assets/site.css", "asset{}")
			Expect(os.Remove(filepath.Join(r.root, "vendor/site.css"))).To(Succeed())

			Eventually(func() string { return body(r.base + "/site.css") }, 10*time.Second).Should(Equal("asset{}"),
				"once the batch settles, the asset is the only owner of site.css")
			Consistently(r.session, 2*time.Second).ShouldNot(gexec.Exit(),
				"no conflict is left at the end of the batch")
			Expect(home(r)).NotTo(ContainSubstring(`id="alloy-error-overlay"`))
		})
	}

	It("frees a path when the page that owned it is deleted (dev)", func() {
		r := start("dev", siteWith(nil))
		Expect(body(r.base+"/about/")).To(ContainSubstring("<p>about</p>"), "sanity: the page owns /about/")
		waitWatching(r)

		write(r.root, "static/about/index.html", "<p>static about</p>")
		Expect(os.Remove(filepath.Join(r.root, "content/about.md"))).To(Succeed())

		Eventually(func() string { return body(r.base + "/about/") }, 10*time.Second).
			Should(ContainSubstring("static about"), "the page is gone, so the static file owns /about/")
		Expect(home(r)).NotTo(ContainSubstring(`id="alloy-error-overlay"`))
	})

	// Guard: with the page kept, the static file is a real conflict.
	It("keeps the page and reports the conflict when the page stays (dev)", func() {
		r := start("dev", siteWith(nil))
		waitWatching(r)

		write(r.root, "static/about/index.html", "<p>static about</p>")
		write(r.root, "content/index.md", "---\ntitle: Home\nlayout: default\n---\nhome edited\n")

		Eventually(func() string { return body(r.base + "/") }, 10*time.Second).
			Should(ContainSubstring("output path conflict"), "the conflict must show in the overlay")
		Expect(output(r, "about/index.html")).To(ContainSubstring("<p>about</p>"),
			"the page still owns /about/, so its output must stay")
	})

	It("frees a path for a deleted page even when the batch has another conflict (dev)", func() {
		r := start("dev", siteWith(nil))
		waitWatching(r)

		write(r.root, "static/about/index.html", "<p>static about</p>")
		Expect(os.Remove(filepath.Join(r.root, "content/about.md"))).To(Succeed())
		write(r.root, "content/home2.md", "---\ntitle: Home 2\nlayout: default\npermalink: /\n---\nhome2\n")

		Eventually(func() string { return body(r.base + "/about/") }, 10*time.Second).
			Should(ContainSubstring("static about"),
				"an unrelated conflict in the same batch must not stop the freed path from being copied")
		Expect(home(r)).To(ContainSubstring("output path conflict"),
			"the unrelated conflict must still show in the overlay")
	})

	// A batch finds a conflict between a static file and a page. A later
	// batch that only touches the static file must not change the page's
	// output.
	conflictWithPage := func() running {
		GinkgoHelper()
		r := start("dev", siteWith(nil))
		waitWatching(r)
		write(r.root, "static/about/index.html", "<p>static about</p>")
		write(r.root, "content/index.md", "---\ntitle: Home\nlayout: default\n---\nhome edited\n")
		Eventually(func() string { return body(r.base + "/") }, 10*time.Second).
			Should(ContainSubstring("output path conflict"), "sanity: the rebuild reports the conflict")
		return r
	}

	It("keeps the page's output when the conflicting static file is edited later (dev)", func() {
		r := conflictWithPage()

		write(r.root, "static/about/index.html", "<p>static about, edited</p>")
		settle(r, "after-edit.txt")

		Expect(output(r, "about/index.html")).To(ContainSubstring("<p>about</p>"),
			"the page still owns /about/, so an edit to the losing static file must not overwrite it")
	})

	It("keeps the page's output when the conflicting static file is deleted later (dev)", func() {
		r := conflictWithPage()

		Expect(os.Remove(filepath.Join(r.root, "static/about/index.html"))).To(Succeed())
		settle(r, "after-delete.txt")

		Expect(output(r, "about/index.html")).To(ContainSubstring("<p>about</p>"),
			"the static file never wrote /about/, so deleting it must not remove the page's output")
	})

	It("stops alloy serve when a new passthrough file collides with a static file", func() {
		r := start("serve", siteWith(map[string]string{"static/site.css": "static{}"}))
		waitWatching(r)

		write(r.root, "vendor/site.css", "vendor{}")

		Eventually(r.session, 15*time.Second).Should(gexec.Exit(1),
			"static/site.css owns the path, so the new passthrough file is a real conflict")
		Expect(string(r.out.Contents())).To(
			ContainSubstring("output path conflict: site.css is claimed by static/site.css and vendor/site.css"))
	})

	It("shows a conflict that remains in the alloy dev overlay", func() {
		r := start("dev", newSite())
		waitWatching(r)

		write(r.root, "assets/site.css", "asset{}")

		Eventually(func() string { return body(r.base + "/") }, 10*time.Second).
			Should(ContainSubstring("output path conflict: site.css"),
				"static/site.css still owns the path, so the overlay must show the conflict")
		Expect(body(r.base+"/site.css")).To(Equal("static{}"), "the static file keeps /site.css")
	})
})
