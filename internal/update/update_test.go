package update_test

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/zeroedin/alloy/internal/update"
)

var _ = Describe("Update", func() {

	// ── UpgradeURL constant ─────────────────────────────────────────

	Describe("UpgradeURL", func() {
		It("is the stable docs upgrade URL", func() {
			Expect(update.UpgradeURL).To(Equal("https://alloyproject.org/docs/upgrade/"),
				"UpgradeURL must be the exact stable docs URL — the CLI "+
					"hardcodes this path and it must not change across releases "+
					"(issue #1072)")
		})
	})

	// ── IsNewer — semantic version comparison ───────────────────────

	Describe("IsNewer", func() {
		It("returns true when latest is a higher patch version", func() {
			Expect(update.IsNewer("0.4.2", "0.4.3")).To(BeTrue(),
				"patch bump 0.4.2 → 0.4.3 must be detected as newer")
		})

		It("returns true when latest is a higher minor version", func() {
			Expect(update.IsNewer("0.4.2", "0.5.0")).To(BeTrue(),
				"minor bump 0.4.2 → 0.5.0 must be detected as newer")
		})

		It("returns true when latest is a higher major version", func() {
			Expect(update.IsNewer("0.4.2", "1.0.0")).To(BeTrue(),
				"major bump 0.4.2 → 1.0.0 must be detected as newer")
		})

		It("returns false when versions are equal", func() {
			Expect(update.IsNewer("0.5.0", "0.5.0")).To(BeFalse(),
				"same version must not be considered newer")
		})

		It("returns false when current is ahead of latest", func() {
			Expect(update.IsNewer("0.5.0", "0.4.2")).To(BeFalse(),
				"current ahead of latest means no update available")
		})

		It("handles v prefix on both strings", func() {
			Expect(update.IsNewer("v0.4.2", "v0.5.0")).To(BeTrue(),
				"v prefix must be stripped before comparison — "+
					"GitHub release tags use v-prefixed versions")
		})

		It("handles v prefix on only one string", func() {
			Expect(update.IsNewer("v0.4.2", "0.5.0")).To(BeTrue(),
				"mixed v-prefix must still compare correctly")
			Expect(update.IsNewer("0.4.2", "v0.5.0")).To(BeTrue(),
				"mixed v-prefix must still compare correctly")
		})

		It("returns false for invalid current version", func() {
			Expect(update.IsNewer("invalid", "0.5.0")).To(BeFalse(),
				"unparseable current version must return false — "+
					"never claim an update is available when we can't "+
					"determine the current version")
		})

		It("returns false for invalid latest version", func() {
			Expect(update.IsNewer("0.4.2", "invalid")).To(BeFalse(),
				"unparseable latest version must return false — "+
					"bad API response must not produce a false positive")
		})

		It("returns false when both versions are invalid", func() {
			Expect(update.IsNewer("abc", "xyz")).To(BeFalse(),
				"two invalid versions must return false")
		})

		It("returns true for pre-release current vs stable latest", func() {
			Expect(update.IsNewer("0.5.0-rc1", "0.5.0")).To(BeTrue(),
				"pre-release 0.5.0-rc1 must be considered older than "+
					"stable 0.5.0 — users on pre-release should be told "+
					"about the stable release")
		})

		It("returns false for stable current vs pre-release latest", func() {
			Expect(update.IsNewer("0.5.0", "0.5.0-rc1")).To(BeFalse(),
				"stable 0.5.0 must NOT be considered older than "+
					"pre-release 0.5.0-rc1 — pre-release is always behind stable")
		})

		It("returns true for older pre-release vs newer pre-release", func() {
			Expect(update.IsNewer("0.5.0-rc1", "0.5.0-rc2")).To(BeTrue(),
				"rc1 must be considered older than rc2 — pre-release "+
					"versions must compare their suffix numerically")
		})
	})

	// ── Cache operations ────────────────────────────────────────────

	Describe("Cache", func() {
		var cacheDir string

		BeforeEach(func() {
			cacheDir = GinkgoT().TempDir()
			GinkgoT().Setenv("XDG_CONFIG_HOME", cacheDir)
		})

		Describe("SaveCache + LoadCache round-trip", func() {
			It("preserves the version and timestamp", func() {
				now := time.Now().Truncate(time.Second)
				original := update.CacheResult{
					LatestVersion: "v0.5.0",
					CheckedAt:     now,
				}

				err := update.SaveCache(original)
				Expect(err).NotTo(HaveOccurred(),
					"SaveCache must write to the XDG config directory without error")

				loaded, err := update.LoadCache()
				Expect(err).NotTo(HaveOccurred(),
					"LoadCache must read back the saved cache without error")
				Expect(loaded.LatestVersion).To(Equal("v0.5.0"),
					"round-trip must preserve the version string exactly")
				Expect(loaded.CheckedAt.Unix()).To(Equal(now.Unix()),
					"round-trip must preserve the timestamp (compared at second granularity)")
			})
		})

		Describe("SaveCache", func() {
			It("creates the cache directory if it does not exist", func() {
				nestedDir := filepath.Join(cacheDir, "deep", "nested")
				GinkgoT().Setenv("XDG_CONFIG_HOME", nestedDir)

				err := update.SaveCache(update.CacheResult{
					LatestVersion: "v1.0.0",
					CheckedAt:     time.Now(),
				})
				Expect(err).NotTo(HaveOccurred(),
					"SaveCache must create intermediate directories via os.MkdirAll — "+
						"first-time users won't have ~/.config/alloy/ yet")

				// Verify the file was actually written
				loaded, loadErr := update.LoadCache()
				Expect(loadErr).NotTo(HaveOccurred())
				Expect(loaded.LatestVersion).To(Equal("v1.0.0"),
					"cache file must exist and be readable after SaveCache creates the directory")
			})
		})

		Describe("LoadCache", func() {
			It("returns zero CacheResult when no cache file exists", func() {
				loaded, err := update.LoadCache()
				Expect(err).NotTo(HaveOccurred(),
					"missing cache file must not be an error — "+
						"this is the normal state for first-time users")
				Expect(loaded.LatestVersion).To(BeEmpty(),
					"zero CacheResult must have empty LatestVersion")
				Expect(loaded.CheckedAt.IsZero()).To(BeTrue(),
					"zero CacheResult must have zero-value CheckedAt")
			})

			It("returns zero CacheResult for corrupt cache file", func() {
				alloyDir := filepath.Join(cacheDir, "alloy")
				Expect(os.MkdirAll(alloyDir, 0755)).To(Succeed())
				Expect(os.WriteFile(
					filepath.Join(alloyDir, "update-check.json"),
					[]byte("not valid json{{{"),
					0644,
				)).To(Succeed())

				loaded, err := update.LoadCache()
				Expect(err).NotTo(HaveOccurred(),
					"corrupt cache file must be treated as cache miss — "+
						"silently ignored, not propagated as an error. "+
						"The caller will perform a fresh API check instead.")
				Expect(loaded.LatestVersion).To(BeEmpty(),
					"corrupt cache must return zero CacheResult")
			})
		})

		Describe("CacheDir", func() {
			It("respects XDG_CONFIG_HOME", func() {
				dir := update.CacheDir()
				Expect(dir).To(HavePrefix(cacheDir),
					"CacheDir must use XDG_CONFIG_HOME when set — "+
						"standard XDG Base Directory compliance")
				Expect(dir).To(HaveSuffix("alloy"),
					"CacheDir must end with 'alloy' subdirectory")
			})

			It("falls back to ~/.config when XDG_CONFIG_HOME is not set", func() {
				GinkgoT().Setenv("XDG_CONFIG_HOME", "")
				dir := update.CacheDir()
				homeDir, err := os.UserHomeDir()
				Expect(err).NotTo(HaveOccurred())
				Expect(dir).To(Equal(filepath.Join(homeDir, ".config", "alloy")),
					"CacheDir must fall back to ~/.config/alloy when "+
						"XDG_CONFIG_HOME is not set")
			})
		})
	})

	// ── ShouldCheck — cache TTL logic ───────────────────────────────

	Describe("ShouldCheck", func() {
		It("returns true for zero CacheResult (first run)", func() {
			Expect(update.ShouldCheck(update.CacheResult{})).To(BeTrue(),
				"zero CacheResult means no previous check — must check")
		})

		It("returns true when cache is expired (>24h)", func() {
			expired := update.CacheResult{
				LatestVersion: "v0.5.0",
				CheckedAt:     time.Now().Add(-25 * time.Hour),
			}
			Expect(update.ShouldCheck(expired)).To(BeTrue(),
				"cache older than 24 hours must trigger a fresh check")
		})

		It("returns false when cache is fresh (<24h)", func() {
			fresh := update.CacheResult{
				LatestVersion: "v0.5.0",
				CheckedAt:     time.Now().Add(-1 * time.Hour),
			}
			Expect(update.ShouldCheck(fresh)).To(BeFalse(),
				"cache less than 24 hours old must not trigger a check — "+
					"avoids hitting the GitHub API on every startup")
		})

		It("returns true when LatestVersion is empty", func() {
			empty := update.CacheResult{
				LatestVersion: "",
				CheckedAt:     time.Now(),
			}
			Expect(update.ShouldCheck(empty)).To(BeTrue(),
				"empty LatestVersion means the previous check failed or "+
					"was incomplete — must retry")
		})

		It("returns true at exactly 24h boundary", func() {
			boundary := update.CacheResult{
				LatestVersion: "v0.5.0",
				CheckedAt:     time.Now().Add(-24 * time.Hour),
			}
			Expect(update.ShouldCheck(boundary)).To(BeTrue(),
				"cache exactly 24 hours old must trigger a fresh check — "+
					"the TTL is 24 hours, not 24 hours and 1 second")
		})
	})

	// ── CheckLatestVersion — GitHub API integration ─────────────────

	Describe("CheckLatestVersion", func() {
		It("parses the tag_name from a GitHub Releases API response", func() {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				Expect(r.URL.Path).To(Equal("/repos/zeroedin/alloy/releases/latest"),
					"must hit the correct GitHub API endpoint")
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(`{"tag_name": "v0.6.0", "name": "v0.6.0"}`))
			}))
			DeferCleanup(server.Close)

			version, err := update.CheckLatestVersionFrom(server.URL)
			Expect(err).NotTo(HaveOccurred(),
				"valid API response must not produce an error")
			Expect(version).To(Equal("v0.6.0"),
				"must return the tag_name from the JSON response")
		})

		It("returns error for non-200 HTTP status", func() {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusNotFound)
			}))
			DeferCleanup(server.Close)

			_, err := update.CheckLatestVersionFrom(server.URL)
			Expect(err).To(HaveOccurred(),
				"non-200 response must return an error")
		})

		It("returns error for invalid JSON response", func() {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(`not json`))
			}))
			DeferCleanup(server.Close)

			_, err := update.CheckLatestVersionFrom(server.URL)
			Expect(err).To(HaveOccurred(),
				"invalid JSON must return an error — malformed API response")
		})

		It("returns error for JSON response with empty tag_name", func() {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(`{"tag_name": "", "name": "v0.6.0"}`))
			}))
			DeferCleanup(server.Close)

			_, err := update.CheckLatestVersionFrom(server.URL)
			Expect(err).To(HaveOccurred(),
				"empty tag_name must return an error — the response is "+
					"technically valid JSON but contains no usable version")
		})

		It("returns error when server is unreachable", func() {
			_, err := update.CheckLatestVersionFrom("http://127.0.0.1:1")
			Expect(err).To(HaveOccurred(),
				"unreachable server must return an error — the passive "+
					"notification silently swallows this; --check prints it")
		})
	})
})
