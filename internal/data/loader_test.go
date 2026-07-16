package data_test

import (
	"path/filepath"
	"runtime"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/zeroedin/alloy/internal/data"
	"github.com/zeroedin/alloy/internal/ordered"
)

// testdataDir returns the absolute path to the testdata directory
// relative to this test file.
func testdataDir() string {
	_, file, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(file), "testdata")
}

// testdataErrorsDir returns the absolute path to the testdata-errors
// directory, which contains fixture directories with intentional error
// conditions (stem collisions, dir-file collisions). Separated from
// testdata/ so that root-level LoadDirectory tests can load testdata/
// without hitting error fixtures during recursive traversal.
func testdataErrorsDir() string {
	_, file, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(file), "testdata-errors")
}

var _ = Describe("Data Loader", func() {

	// ── YAML data files ────────────────────────────────────────────────

	Context("YAML data files", func() {
		It("loads .yaml file into map", func() {
			path := filepath.Join(testdataDir(), "navigation.yaml")
			result, err := data.LoadFile(path)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(HaveKey("items"))

			items, ok := result["items"].([]interface{})
			Expect(ok).To(BeTrue(), "items should be a slice")
			Expect(items).To(HaveLen(2))
		})
	})

	// ── TOML data files ────────────────────────────────────────────────

	Context("TOML data files", func() {
		It("loads .toml file into map", func() {
			path := filepath.Join(testdataDir(), "settings.toml")
			result, err := data.LoadFile(path)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(HaveKey("site"))

			site, ok := result["site"].(map[string]interface{})
			Expect(ok).To(BeTrue(), "site should be a map")
			Expect(site).To(HaveKeyWithValue("name", "Test"))
		})
	})

	// ── JSON data files ────────────────────────────────────────────────

	Context("JSON data files", func() {
		It("loads .json file into map", func() {
			path := filepath.Join(testdataDir(), "authors.json")
			result, err := data.LoadFile(path)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(HaveKey("alice"))
			Expect(result).To(HaveKey("bob"))
		})

		// ── JSON key order preservation (issue #453) ────────────────
		// JSON data files must return *ordered.Map to preserve key
		// insertion order. Only JSON — YAML/TOML use map[string]interface{}.

		It("JSON LoadFileAny returns *ordered.Map preserving key order (issue #453)", func() {
			path := filepath.Join(testdataDir(), "ordered-keys.json")
			result, err := data.LoadFileAny(path)
			Expect(err).NotTo(HaveOccurred())

			om, ok := result.(*ordered.Map)
			Expect(ok).To(BeTrue(),
				"LoadFileAny for .json must return *ordered.Map — "+
					"if this fails, json.Unmarshal into map[string]interface{} "+
					"is still being used instead of ordered.Map.UnmarshalJSON (issue #453)")

			Expect(om.Keys()).To(Equal([]string{"white", "black", "accent", "brand", "surface"}),
				"JSON key insertion order must be preserved")
		})
	})

	// ── CSV data files ─────────────────────────────────────────────────

	Context("CSV data files", func() {
		It("loads .csv as array of maps with header row as keys", func() {
			path := filepath.Join(testdataDir(), "team.csv")
			rows, err := data.LoadCSV(path)
			Expect(err).NotTo(HaveOccurred())
			Expect(rows).To(HaveLen(3))
		})

		It("each row has keys name, role, email", func() {
			path := filepath.Join(testdataDir(), "team.csv")
			rows, err := data.LoadCSV(path)
			Expect(err).NotTo(HaveOccurred())
			for _, row := range rows {
				Expect(row).To(HaveKey("name"))
				Expect(row).To(HaveKey("role"))
				Expect(row).To(HaveKey("email"))
			}
		})
	})

	// ── data/ directory loading ────────────────────────────────────────

	Context("data/ directory loading", func() {
		It("loads all files from directory, keyed by filename without extension", func() {
			dir := testdataDir()
			result, err := data.LoadDirectory(dir)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(HaveKey("navigation"))
			Expect(result).To(HaveKey("settings"))
			Expect(result).To(HaveKey("authors"))
		})

		It("handles mixed file formats", func() {
			dir := testdataDir()
			result, err := data.LoadDirectory(dir)
			Expect(err).NotTo(HaveOccurred())
			// The directory contains .yaml, .toml, .json, and .csv files
			// All should be loaded without error
			Expect(len(result)).To(BeNumerically(">=", 3))
		})
	})

	// ── Data file stem collisions ─────────────────────────────────────

	Context("Data file stem collisions", func() {
		It("returns error when two files share a stem name (team.csv and team.yaml)", func() {
			dir := filepath.Join(testdataErrorsDir(), "collision")
			_, err := data.LoadDirectory(dir)
			Expect(err).To(HaveOccurred(),
				"LoadDirectory must error when two files share a stem name")
			Expect(err.Error()).To(SatisfyAll(
				ContainSubstring("team"),
				ContainSubstring("conflict"),
			), "error must name the conflicting stem and mention conflict")
		})
	})

	// ── Subdirectory recursive loading (issue #983) ──────────────────

	Context("Subdirectory recursive loading (issue #983)", func() {
		// LoadDirectory must recurse into subdirectories. Each subdirectory
		// becomes a nested namespace key: data/nav/main.yaml → result["nav"]["main"].
		// This matches Eleventy-style nested data namespacing.

		It("loads files from subdirectories into nested namespace", func() {
			dir := filepath.Join(testdataDir(), "nested")
			result, err := data.LoadDirectory(dir)
			Expect(err).NotTo(HaveOccurred(),
				"LoadDirectory must recurse into subdirectories without error")

			// data/nested/nav/ should produce result["nav"] as a map
			Expect(result).To(HaveKey("nav"),
				"subdirectory 'nav' must become a key in the result — "+
					"if this fails, LoadDirectory is skipping directories instead of recursing (issue #983)")
			navMap, ok := result["nav"].(map[string]interface{})
			Expect(ok).To(BeTrue(),
				"subdirectory key must be a map[string]interface{} containing child entries — "+
					"not the raw directory entry or a file value")

			// nav/main.yaml → result["nav"]["main"]
			Expect(navMap).To(HaveKey("main"),
				"nav/main.yaml must appear as result[\"nav\"][\"main\"]")
			mainData, ok := navMap["main"].(map[string]interface{})
			Expect(ok).To(BeTrue(),
				"nav/main.yaml parsed content must be a map")
			Expect(mainData).To(HaveKey("items"),
				"nav/main.yaml content must be accessible through the nested namespace")

			// nav/footer.json → result["nav"]["footer"]
			Expect(navMap).To(HaveKey("footer"),
				"nav/footer.json must appear as result[\"nav\"][\"footer\"]")
		})

		It("root-level files coexist with subdirectory namespaces", func() {
			dir := filepath.Join(testdataDir(), "nested")
			result, err := data.LoadDirectory(dir)
			Expect(err).NotTo(HaveOccurred())

			// colors.yaml at root level → result["colors"]
			Expect(result).To(HaveKey("colors"),
				"root-level file must still be loaded alongside subdirectory namespaces")
			colors, ok := result["colors"].([]interface{})
			Expect(ok).To(BeTrue(),
				"colors.yaml contains a root-level array — must parse as []interface{}")
			Expect(colors).To(HaveLen(3),
				"colors.yaml has 3 entries (red, green, blue)")

			// Subdirectory namespace also present
			Expect(result).To(HaveKey("nav"),
				"subdirectory namespace must coexist with root-level file entries")
		})

		It("deeply nested files create deeply nested maps", func() {
			dir := filepath.Join(testdataDir(), "nested")
			result, err := data.LoadDirectory(dir)
			Expect(err).NotTo(HaveOccurred())

			// data/nested/api/v2/endpoints.toml → result["api"]["v2"]["endpoints"]
			Expect(result).To(HaveKey("api"),
				"first-level subdirectory 'api' must be a key")
			apiMap, ok := result["api"].(map[string]interface{})
			Expect(ok).To(BeTrue(),
				"api must be a nested map, not a flat value")

			Expect(apiMap).To(HaveKey("v2"),
				"second-level subdirectory 'v2' must be a key within 'api'")
			v2Map, ok := apiMap["v2"].(map[string]interface{})
			Expect(ok).To(BeTrue(),
				"v2 must be a nested map")

			Expect(v2Map).To(HaveKey("endpoints"),
				"endpoints.toml must appear within the deeply nested namespace")
			endpointsData, ok := v2Map["endpoints"].(map[string]interface{})
			Expect(ok).To(BeTrue(),
				"endpoints.toml parsed content must be a map (TOML)")
			Expect(endpointsData).To(HaveKey("users"),
				"TOML content within deeply nested subdirectory must be fully parsed and accessible")
		})

		It("empty subdirectories produce no key in result", func() {
			dir := filepath.Join(testdataDir(), "nested-empty-subdir")
			result, err := data.LoadDirectory(dir)
			Expect(err).NotTo(HaveOccurred())

			// Root-level file must still load
			Expect(result).To(HaveKey("root-file"),
				"root-level file in directory with empty subdirectory must still load")

			// Empty subdirectory must not produce a key
			Expect(result).NotTo(HaveKey("emptydir"),
				"empty subdirectory must not produce a key in the result — "+
					"only directories containing data files (at any depth) should appear")
		})

		It("errors on directory-file stem collision", func() {
			dir := filepath.Join(testdataErrorsDir(), "dir-file-collision")
			_, err := data.LoadDirectory(dir)
			Expect(err).To(HaveOccurred(),
				"LoadDirectory must error when a file and subdirectory share the same stem — "+
					"nav.yaml and nav/ both claim the key \"nav\" (issue #983)")
			Expect(err.Error()).To(SatisfyAll(
				ContainSubstring("nav"),
				ContainSubstring("conflict"),
			), "error must name the colliding stem and mention conflict — "+
				"same collision semantics as two files sharing a stem (#982)")
		})

		It("applies stem collision detection within subdirectories", func() {
			dir := filepath.Join(testdataErrorsDir(), "nested-collision")
			_, err := data.LoadDirectory(dir)
			Expect(err).To(HaveOccurred(),
				"stem collision rules must apply recursively within subdirectories — "+
					"sub/team.yaml and sub/team.json share the stem \"team\"")
			Expect(err.Error()).To(SatisfyAll(
				ContainSubstring("team"),
				ContainSubstring("conflict"),
			), "error must name the conflicting stem within the subdirectory")
		})
	})

	// ── Error handling ─────────────────────────────────────────────────

	Context("Error handling", func() {
		It("returns error for malformed data file", func() {
			_, err := data.LoadFile("/nonexistent/path/does-not-exist.yaml")
			Expect(err).To(HaveOccurred())
			// The error must indicate the file was not found, not be a generic stub error
			Expect(err.Error()).To(
				SatisfyAny(
					ContainSubstring("no such file"),
					ContainSubstring("not found"),
					ContainSubstring("does not exist"),
				),
				"error should indicate file-not-found, not a generic error",
			)
		})
	})
})
