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

	// start serves an empty output directory, as after a failed full rebuild.
	start := func(mode server.ServerMode, withError bool) *server.Server {
		GinkgoHelper()
		root := GinkgoT().TempDir()
		Expect(os.MkdirAll(filepath.Join(root, "_site"), 0755)).To(Succeed())
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

	// Guards: nothing changes when there's no error, or outside alloy dev.
	It("keeps the plain 404 when there's no build error", func() {
		srv := start(server.ModeDev, false)
		resp, body := get(srv, "/about/")

		Expect(resp.StatusCode).To(Equal(http.StatusNotFound))
		Expect(body).NotTo(ContainSubstring("alloy-error-overlay"))
	})

	It("keeps the plain 404 in alloy serve", func() {
		srv := start(server.ModePreview, true)
		resp, body := get(srv, "/about/")

		Expect(resp.StatusCode).To(Equal(http.StatusNotFound))
		Expect(body).NotTo(ContainSubstring("alloy-error-overlay"),
			"alloy serve never shows the overlay")
	})
})
