package update

import (
	"errors"
	"time"
)

// ErrNotImplemented is returned by all stub functions.
var ErrNotImplemented = errors.New("not implemented")

// UpgradeURL is the stable documentation URL shown in update notifications.
// This URL must not change across releases — the CLI hardcodes it.
const UpgradeURL = "https://alloyproject.org/docs/upgrade/"

// CacheResult represents a cached version check result.
type CacheResult struct {
	LatestVersion string    `json:"latestVersion"`
	CheckedAt     time.Time `json:"checkedAt"`
}

// IsNewer returns true if latest is a higher semantic version than current.
// Both strings may have a leading "v" prefix (stripped before comparison).
// Returns false if either string is not valid semver.
func IsNewer(current, latest string) bool {
	return false
}

// CheckLatestVersion queries the GitHub Releases API for the latest
// release tag. Returns the tag name (e.g., "v0.5.0") or an error.
// Uses a 10-second HTTP timeout. Unauthenticated.
func CheckLatestVersion() (string, error) {
	return CheckLatestVersionFrom("https://api.github.com")
}

// CheckLatestVersionFrom queries the given base URL (which must serve
// the GitHub Releases API format) for the latest release tag.
// Tests use this with httptest servers; production calls CheckLatestVersion.
func CheckLatestVersionFrom(baseURL string) (string, error) {
	return "", ErrNotImplemented
}

// LoadCache reads the cached check result from the XDG config directory.
// Returns zero CacheResult and nil error if the file doesn't exist or
// is corrupt. Returns error only for unexpected I/O failures.
func LoadCache() (CacheResult, error) {
	return CacheResult{}, ErrNotImplemented
}

// SaveCache writes the check result to the XDG config directory.
// Creates the directory if needed.
func SaveCache(result CacheResult) error {
	return ErrNotImplemented
}

// ShouldCheck returns true if the cache is missing, expired (>24h),
// or has an empty LatestVersion.
func ShouldCheck(cached CacheResult) bool {
	return false
}

// CacheDir returns the alloy config directory path,
// respecting XDG_CONFIG_HOME. Falls back to ~/.config/alloy.
func CacheDir() string {
	return ""
}
