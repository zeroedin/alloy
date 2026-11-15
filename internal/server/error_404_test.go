package server_test

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/zeroedin/alloy/internal/config"
	"github.com/zeroedin/alloy/internal/server"
)

// ── A build error shows on 404 pages in alloy dev (issue #1316) ────────
// A failed full rebuild empties _site, so while the error is active every
// page is a 404. The overlay was only added to HTML files that exist, so
// the browser showed a bare "404 page not found" and no error. It also had
// no live-reload script, so the page didn't reload once the error was fixed.
//
// See PLAN.md → "alloy dev never reports success while the site is
// incomplete (issue #1316)".

var _ = Describe("A build error shows on 404 pages in alloy dev (issue #1316)", func() {
	const message = `layout "nonexistent" not found for page "n5.md"`

	// start serves an output directory with only a home page, as when most
	// of the site is missing after a failed full rebuild.
	start := func(mode server.ServerMode, withError bool) *server.Server {
		GinkgoHelper()
		root := GinkgoT().TempDir()
		Expect(os.MkdirAll(filepath.Join(root, "_site"), 0755)).To(Succeed())
		Expect(os.WriteFile(filepath.Join(root, "_site", "index.html"),
			[]byte("<html><body>Home</body></html>"), 0644)).To(Succeed())
		srv := server.NewWithMode(&config.Config{
			Title:       "Test Site",
			ProjectRoot: root,
			Build:       config.BuildConfig{Output: "_site"},
		}, mode)
		if withError {
			srv.Overlay().SetErrors([]server.BuildError{{Message: message, Stage: "rebuild"}})
		}
		Expect(srv.Start(0)).To(Succeed())
		DeferCleanup(srv.Stop)
		return srv
	}

	get := func(srv *server.Server, path string) (*http.Response, string) {
		GinkgoHelper()
		resp, err := httpClient.Get(fmt.Sprintf("http://localhost:%d%s", srv.Port(), path))
		Expect(err).NotTo(HaveOccurred())
		defer resp.Body.Close()
		body, err := io.ReadAll(resp.Body)
		Expect(err).NotTo(HaveOccurred())
		return resp, string(body)
	}

	It("shows the active error on a page that isn't there", func() {
		srv := start(server.ModeDev, true)
		resp, body := get(srv, "/about/")

		Expect(resp.StatusCode).To(Equal(http.StatusNotFound),
			"the page still doesn't exist, so the status stays 404")
		Expect(resp.Header.Get("Content-Type")).To(HavePrefix("text/html"),
			"the 404 must be an HTML page so the browser can show the overlay")
		Expect(body).To(ContainSubstring(`id="alloy-error-overlay"`),
			"the browser must show the build error, not a bare 404")
		Expect(body).To(ContainSubstring("not found for page"),
			"the overlay must carry the error message")
	})

	It("reloads the 404 page once the error is fixed", func() {
		srv := start(server.ModeDev, true)
		_, body := get(srv, "/about/")

		Expect(body).To(ContainSubstring("/_alloy/ws"),
			"the 404 page needs the live-reload script, or it won't reload when the rebuild succeeds")
	})

	// The watcher sets and clears errors while requests read them. Run with
	// -race (as CI does): the overlay state must be safe for concurrent use.
	It("serves pages and 404s while the build errors change", func() {
		srv := start(server.ModeDev, false)
		done := make(chan struct{})
		go func() {
			defer GinkgoRecover()
			for {
				select {
				case <-done:
					return
				default:
					srv.Overlay().SetErrors([]server.BuildError{{Message: message, Stage: "rebuild"}})
					srv.Overlay().ClearErrors()
				}
			}
		}()
		for i := 0; i < 50; i++ {
			resp, _ := get(srv, "/")
			Expect(resp.StatusCode).To(Equal(http.StatusOK))
			resp, _ = get(srv, "/missing/")
			Expect(resp.StatusCode).To(Equal(http.StatusNotFound))
		}
		close(done)
	})

	// Guards: nothing changes when there's no error, or outside alloy dev.
	It("keeps the plain 404 when there's no build error", func() {
		srv := start(server.ModeDev, false)
		resp, body := get(srv, "/about/")

		Expect(resp.StatusCode).To(Equal(http.StatusNotFound))
		Expect(resp.Header.Get("Content-Type")).To(Equal("text/plain; charset=utf-8"))
		Expect(body).To(Equal("404 page not found\n"),
			"with no build error, the 404 is Go's plain one, unchanged")
	})

	It("keeps the plain 404 in alloy serve", func() {
		srv := start(server.ModePreview, true)
		resp, body := get(srv, "/about/")

		Expect(resp.StatusCode).To(Equal(http.StatusNotFound))
		Expect(resp.Header.Get("Content-Type")).To(Equal("text/plain; charset=utf-8"))
		Expect(body).To(Equal("404 page not found\n"),
			"alloy serve never shows the overlay")
	})
})
