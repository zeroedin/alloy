package data_test

import (
	"path/filepath"
	"runtime"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/zeroedin/alloy/internal/data"
)

// testdataDir returns the absolute path to the testdata directory
// relative to this test file.
func testdataDir() string {
	_, file, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(file), "testdata")
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

		// ── JSON key order preservation (issue #446) ────────────────
		// JSON objects must preserve key insertion order via _keys metadata.

		It("preserves JSON key insertion order via _keys metadata (issue #446)", func() {
			path := filepath.Join(testdataDir(), "ordered-keys.json")
			result, err := data.LoadFile(path)
			Expect(err).NotTo(HaveOccurred())

			keys, hasKeys := result["_keys"]
			Expect(hasKeys).To(BeTrue(),
				"JSON objects must include a _keys metadata array recording "+
					"the original key insertion order — if missing, json.Unmarshal "+
					"was used instead of json.Decoder with Token() streaming (issue #446)")

			keysSlice, ok := keys.([]interface{})
			Expect(ok).To(BeTrue(), "_keys must be a []interface{}")
			Expect(keysSlice).To(Equal([]interface{}{
				"white", "black", "accent", "brand", "surface",
			}),
				"_keys must preserve the exact insertion order from the JSON file — "+
					"white, black, accent, brand, surface")
		})

		It("nested JSON objects also have _keys metadata (issue #446)", func() {
			path := filepath.Join(testdataDir(), "authors.json")
			result, err := data.LoadFile(path)
			Expect(err).NotTo(HaveOccurred())

			// Top-level _keys should list alice, bob in insertion order
			keys, hasKeys := result["_keys"]
			Expect(hasKeys).To(BeTrue(),
				"top-level JSON object must have _keys")

			keysSlice, ok := keys.([]interface{})
			Expect(ok).To(BeTrue())
			Expect(keysSlice).To(Equal([]interface{}{"alice", "bob"}),
				"top-level _keys must be [alice, bob] matching JSON source order")

			// Nested objects should also have _keys
			alice, ok := result["alice"].(map[string]interface{})
			Expect(ok).To(BeTrue())
			nestedKeys, hasNestedKeys := alice["_keys"]
			Expect(hasNestedKeys).To(BeTrue(),
				"nested JSON objects must also have _keys metadata")

			nestedKeysSlice, ok := nestedKeys.([]interface{})
			Expect(ok).To(BeTrue())
			Expect(nestedKeysSlice).To(Equal([]interface{}{"name", "email"}),
				"nested _keys must preserve insertion order from the JSON source")
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
			dir := filepath.Join(testdataDir(), "collision")
			_, err := data.LoadDirectory(dir)
			Expect(err).To(HaveOccurred(),
				"LoadDirectory must error when two files share a stem name")
			Expect(err.Error()).To(SatisfyAll(
				ContainSubstring("team"),
				ContainSubstring("conflict"),
			), "error must name the conflicting stem and mention conflict")
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
