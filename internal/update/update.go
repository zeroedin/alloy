package update

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Masterminds/semver/v3"
	"github.com/zeroedin/alloy/internal/jsonutil"
)

var jsonCodec = jsonutil.JSON

const cacheTTL = 24 * time.Hour
const cacheFileName = "update-check.json"

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
	cur, err := semver.NewVersion(strings.TrimPrefix(current, "v"))
	if err != nil {
		return false
	}
	lat, err := semver.NewVersion(strings.TrimPrefix(latest, "v"))
	if err != nil {
		return false
	}
	return lat.GreaterThan(cur)
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
	reqURL := strings.TrimRight(baseURL, "/") + "/repos/zeroedin/alloy/releases/latest"

	req, err := http.NewRequest("GET", reqURL, nil)
	if err != nil {
		return "", fmt.Errorf("creating update check request: %w", err)
	}
	req.Header.Set("User-Agent", "alloy/update-check")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("update check request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("update check returned HTTP %d", resp.StatusCode)
	}

	const maxResponseSize = 1 << 20 // 1 MB
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseSize))
	if err != nil {
		return "", fmt.Errorf("reading update check response: %w", err)
	}

	var release struct {
		TagName string `json:"tag_name"`
	}
	if err := jsonCodec.Unmarshal(body, &release); err != nil {
		return "", fmt.Errorf("parsing update check response: %w", err)
	}

	if release.TagName == "" {
		return "", fmt.Errorf("update check response has empty tag_name")
	}

	return release.TagName, nil
}

// LoadCache reads the cached check result from the XDG config directory.
// Returns zero CacheResult and nil error if the file doesn't exist or
// is corrupt. Returns error only for unexpected I/O failures.
func LoadCache() (CacheResult, error) {
	path := filepath.Join(CacheDir(), cacheFileName)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return CacheResult{}, nil
		}
		return CacheResult{}, fmt.Errorf("reading cache file: %w", err)
	}

	var result CacheResult
	if err := jsonCodec.Unmarshal(data, &result); err != nil {
		return CacheResult{}, nil
	}

	return result, nil
}

// SaveCache writes the check result to the XDG config directory.
// Creates the directory if needed.
func SaveCache(result CacheResult) error {
	dir := CacheDir()
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("creating cache directory: %w", err)
	}

	data, err := jsonCodec.Marshal(result)
	if err != nil {
		return fmt.Errorf("marshaling cache result: %w", err)
	}

	return os.WriteFile(filepath.Join(dir, cacheFileName), data, 0644)
}

// ShouldCheck returns true if the cache is missing, expired (>24h),
// or has an empty LatestVersion.
func ShouldCheck(cached CacheResult) bool {
	if cached.LatestVersion == "" {
		return true
	}
	if cached.CheckedAt.IsZero() {
		return true
	}
	return time.Since(cached.CheckedAt) >= cacheTTL
}

// CacheDir returns the alloy config directory path,
// respecting XDG_CONFIG_HOME. Falls back to ~/.config/alloy.
func CacheDir() string {
	configHome := os.Getenv("XDG_CONFIG_HOME")
	if configHome == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return filepath.Join(os.TempDir(), "alloy")
		}
		configHome = filepath.Join(home, ".config")
	}
	return filepath.Join(configHome, "alloy")
}
