package cmd

import (
	"fmt"
	"io"
	"time"

	"github.com/zeroedin/alloy/internal/update"
)

// maybeNotifyUpdate checks for a newer version and prints a notification.
// Called after "Serving at http://localhost:..." in dev and serve commands.
// If the cache is fresh and has a newer version, prints immediately.
// If the cache is stale, launches a background goroutine to check.
// All errors are silently swallowed.
func maybeNotifyUpdate(out io.Writer, currentVersion string) {
	cached, err := update.LoadCache()
	if err != nil {
		return
	}

	if !update.ShouldCheck(cached) {
		if update.IsNewer(currentVersion, cached.LatestVersion) {
			fmt.Fprintf(out, "Update available: %s → %s — %s\n",
				currentVersion, cached.LatestVersion, update.UpgradeURL)
		}
		return
	}

	go func() {
		latest, err := update.CheckLatestVersion()
		if err != nil {
			return
		}
		_ = update.SaveCache(update.CacheResult{
			LatestVersion: latest,
			CheckedAt:     time.Now(),
		})
		if update.IsNewer(currentVersion, latest) {
			fmt.Fprintf(out, "Update available: %s → %s — %s\n",
				currentVersion, latest, update.UpgradeURL)
		}
	}()
}
