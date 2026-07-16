package pipeline_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/zeroedin/alloy/internal/config"
	"github.com/zeroedin/alloy/internal/pipeline"
)

var _ = Describe("onConfig path traversal validation (issue #998)", func() {
	// Base config and content used by all tests. Each test overrides
	// the plugin to set a specific field to a malicious value.
	baseConfig := func() *config.Config {
		return &config.Config{
			Title:   "Path Traversal Test",
			BaseURL: "https://example.com",
			Build:   config.BuildConfig{Output: "_site"},
		}
	}

	baseContent := func(pluginName, pluginCode string) map[string]string {
		return map[string]string{
			"content/index.md":         "---\ntitle: Home\nlayout: default\n---\n# Home",
			"layouts/default.liquid":   "<html><body>{{ content }}</body></html>",
			"plugins/" + pluginName:    pluginCode,
		}
	}

	// ── Absolute path rejection ─────────────────────────────────────
	// A plugin must not be able to redirect any structure directory or
	// build.output to an absolute filesystem path. resolveDir returns
	// absolute paths as-is, so absolute values bypass project-root
	// sandboxing entirely.

	Describe("absolute path rejection", func() {
		It("rejects absolute path for build.output", func() {
			contentMap := baseContent("abs-output.js", `export default function(alloy) {
  alloy.hook('onConfig', {}, (config) => {
    config.build.output = "/tmp/evil";
    return config;
  });
}`)
			_, err := pipeline.BuildWithContent(baseConfig(), contentMap)
			Expect(err).To(HaveOccurred(),
				"build.output set to an absolute path must be rejected — "+
					"resolveDir returns absolute paths as-is, bypassing project-root "+
					"sandboxing. With clean: true (default), CleanOutputDir would run "+
					"os.RemoveAll on /tmp/evil (issue #998)")
			Expect(err.Error()).To(ContainSubstring("build.output"),
				"error must identify the offending field so plugin authors know "+
					"which onConfig mutation was rejected (issue #998)")
			Expect(err.Error()).To(ContainSubstring("absolute"),
				"error must indicate an absolute path was the reason for rejection — "+
					"distinguishes from other validation errors like directory overlap (issue #998)")
		})

		It("rejects absolute path for structure.content", func() {
			contentMap := baseContent("abs-content.js", `export default function(alloy) {
  alloy.hook('onConfig', {}, (config) => {
    config.structure.content = "/etc";
    return config;
  });
}`)
			_, err := pipeline.BuildWithContent(baseConfig(), contentMap)
			Expect(err).To(HaveOccurred(),
				"structure.content set to an absolute path must be rejected — "+
					"the pipeline would read content from /etc instead of the project (issue #998)")
			Expect(err.Error()).To(ContainSubstring("structure.content"),
				"error must identify structure.content as the offending field (issue #998)")
			Expect(err.Error()).To(ContainSubstring("absolute"),
				"error must indicate an absolute path was the reason for rejection (issue #998)")
		})

		It("rejects absolute path for structure.layouts", func() {
			contentMap := baseContent("abs-layouts.js", `export default function(alloy) {
  alloy.hook('onConfig', {}, (config) => {
    config.structure.layouts = "/usr/share/templates";
    return config;
  });
}`)
			_, err := pipeline.BuildWithContent(baseConfig(), contentMap)
			Expect(err).To(HaveOccurred(),
				"structure.layouts set to an absolute path must be rejected (issue #998)")
			Expect(err.Error()).To(ContainSubstring("structure.layouts"),
				"error must identify structure.layouts as the offending field (issue #998)")
			Expect(err.Error()).To(ContainSubstring("absolute"),
				"error must indicate an absolute path was the reason for rejection (issue #998)")
		})

		It("rejects absolute path for structure.assets", func() {
			contentMap := baseContent("abs-assets.js", `export default function(alloy) {
  alloy.hook('onConfig', {}, (config) => {
    config.structure.assets = "/var/www/assets";
    return config;
  });
}`)
			_, err := pipeline.BuildWithContent(baseConfig(), contentMap)
			Expect(err).To(HaveOccurred(),
				"structure.assets set to an absolute path must be rejected (issue #998)")
			Expect(err.Error()).To(ContainSubstring("structure.assets"),
				"error must identify structure.assets as the offending field (issue #998)")
			Expect(err.Error()).To(ContainSubstring("absolute"),
				"error must indicate an absolute path was the reason for rejection (issue #998)")
		})

		It("rejects absolute path for structure.static", func() {
			contentMap := baseContent("abs-static.js", `export default function(alloy) {
  alloy.hook('onConfig', {}, (config) => {
    config.structure.static = "/opt/static";
    return config;
  });
}`)
			_, err := pipeline.BuildWithContent(baseConfig(), contentMap)
			Expect(err).To(HaveOccurred(),
				"structure.static set to an absolute path must be rejected (issue #998)")
			Expect(err.Error()).To(ContainSubstring("structure.static"),
				"error must identify structure.static as the offending field (issue #998)")
			Expect(err.Error()).To(ContainSubstring("absolute"),
				"error must indicate an absolute path was the reason for rejection (issue #998)")
		})

		It("rejects absolute path for structure.data", func() {
			contentMap := baseContent("abs-data.js", `export default function(alloy) {
  alloy.hook('onConfig', {}, (config) => {
    config.structure.data = "/etc/passwd";
    return config;
  });
}`)
			_, err := pipeline.BuildWithContent(baseConfig(), contentMap)
			Expect(err).To(HaveOccurred(),
				"structure.data set to an absolute path must be rejected (issue #998)")
			Expect(err.Error()).To(ContainSubstring("structure.data"),
				"error must identify structure.data as the offending field (issue #998)")
			Expect(err.Error()).To(ContainSubstring("absolute"),
				"error must indicate an absolute path was the reason for rejection (issue #998)")
		})
	})

	// ── Relative traversal rejection ────────────────────────────────
	// A plugin must not be able to use ".." components to escape the
	// project root. resolveDir joins relative paths to projectRoot,
	// but "../../etc" resolves to a directory above the project.
	// With clean: true, this could delete files outside the project.

	Describe("relative traversal rejection", func() {
		It("rejects traversal path for build.output", func() {
			contentMap := baseContent("trav-output.js", `export default function(alloy) {
  alloy.hook('onConfig', {}, (config) => {
    config.build.output = "../../evil";
    return config;
  });
}`)
			_, err := pipeline.BuildWithContent(baseConfig(), contentMap)
			Expect(err).To(HaveOccurred(),
				"build.output with .. traversal above project root must be rejected — "+
					"resolveDir would resolve to a directory above the project. "+
					"With clean: true (default), CleanOutputDir would run os.RemoveAll "+
					"on entries outside the project tree (issue #998)")
			Expect(err.Error()).To(ContainSubstring("build.output"),
				"error must identify the offending field (issue #998)")
			Expect(err.Error()).To(ContainSubstring("travers"),
				"error must indicate path traversal was the reason for rejection — "+
					"distinguishes from absolute-path rejection (issue #998)")
		})

		It("rejects traversal path for structure.content", func() {
			contentMap := baseContent("trav-content.js", `export default function(alloy) {
  alloy.hook('onConfig', {}, (config) => {
    config.structure.content = "../../etc";
    return config;
  });
}`)
			_, err := pipeline.BuildWithContent(baseConfig(), contentMap)
			Expect(err).To(HaveOccurred(),
				"structure.content with .. traversal above project root must be rejected — "+
					"the pipeline would discover content from ../../etc (issue #998)")
			Expect(err.Error()).To(ContainSubstring("structure.content"),
				"error must identify structure.content as the offending field (issue #998)")
			Expect(err.Error()).To(ContainSubstring("travers"),
				"error must indicate path traversal was the reason for rejection (issue #998)")
		})

		It("rejects traversal path for structure.layouts", func() {
			contentMap := baseContent("trav-layouts.js", `export default function(alloy) {
  alloy.hook('onConfig', {}, (config) => {
    config.structure.layouts = "../../../layouts";
    return config;
  });
}`)
			_, err := pipeline.BuildWithContent(baseConfig(), contentMap)
			Expect(err).To(HaveOccurred(),
				"structure.layouts with .. traversal above project root must be rejected (issue #998)")
			Expect(err.Error()).To(ContainSubstring("structure.layouts"),
				"error must identify structure.layouts as the offending field (issue #998)")
			Expect(err.Error()).To(ContainSubstring("travers"),
				"error must indicate path traversal was the reason for rejection (issue #998)")
		})

		It("rejects traversal path for structure.assets", func() {
			contentMap := baseContent("trav-assets.js", `export default function(alloy) {
  alloy.hook('onConfig', {}, (config) => {
    config.structure.assets = "../../../assets";
    return config;
  });
}`)
			_, err := pipeline.BuildWithContent(baseConfig(), contentMap)
			Expect(err).To(HaveOccurred(),
				"structure.assets with .. traversal above project root must be rejected (issue #998)")
			Expect(err.Error()).To(ContainSubstring("structure.assets"),
				"error must identify structure.assets as the offending field (issue #998)")
			Expect(err.Error()).To(ContainSubstring("travers"),
				"error must indicate path traversal was the reason for rejection (issue #998)")
		})

		It("rejects traversal path for structure.static", func() {
			contentMap := baseContent("trav-static.js", `export default function(alloy) {
  alloy.hook('onConfig', {}, (config) => {
    config.structure.static = "../../static";
    return config;
  });
}`)
			_, err := pipeline.BuildWithContent(baseConfig(), contentMap)
			Expect(err).To(HaveOccurred(),
				"structure.static with .. traversal above project root must be rejected (issue #998)")
			Expect(err.Error()).To(ContainSubstring("structure.static"),
				"error must identify structure.static as the offending field (issue #998)")
			Expect(err.Error()).To(ContainSubstring("travers"),
				"error must indicate path traversal was the reason for rejection (issue #998)")
		})

		It("rejects traversal path for structure.data", func() {
			contentMap := baseContent("trav-data.js", `export default function(alloy) {
  alloy.hook('onConfig', {}, (config) => {
    config.structure.data = "../../data";
    return config;
  });
}`)
			_, err := pipeline.BuildWithContent(baseConfig(), contentMap)
			Expect(err).To(HaveOccurred(),
				"structure.data with .. traversal above project root must be rejected (issue #998)")
			Expect(err.Error()).To(ContainSubstring("structure.data"),
				"error must identify structure.data as the offending field (issue #998)")
			Expect(err.Error()).To(ContainSubstring("travers"),
				"error must indicate path traversal was the reason for rejection (issue #998)")
		})
	})

	// ── Edge cases ──────────────────────────────────────────────────

	Describe("edge cases", func() {
		It("allows relative path with embedded .. that stays within project root", func() {
			// "subdir/../dist" cleans to "dist" — a valid relative path
			// that does not escape the project root.
			contentMap := baseContent("safe-dotdot.js", `export default function(alloy) {
  alloy.hook('onConfig', {}, (config) => {
    config.build.output = "subdir/../dist";
    return config;
  });
}`)
			result, err := pipeline.BuildWithContent(baseConfig(), contentMap)
			Expect(err).NotTo(HaveOccurred(),
				"a path with embedded .. that resolves safely within the project root "+
					"must be accepted — 'subdir/../dist' cleans to 'dist' which is a "+
					"valid relative output directory. Rejecting this would be overly "+
					"restrictive and break legitimate plugin use cases (issue #998)")
			Expect(result).NotTo(BeNil())
			Expect(result.OutputDir).To(Equal("dist"),
				"build.output must resolve 'subdir/../dist' to 'dist' after path "+
					"cleaning — if OutputDir is 'subdir/../dist' the path was not "+
					"cleaned before use (issue #998)")
		})

		It("rejects single .. that traverses to parent directory", func() {
			// ".." alone escapes the project root — the output directory
			// would be the parent of the project.
			contentMap := baseContent("parent-escape.js", `export default function(alloy) {
  alloy.hook('onConfig', {}, (config) => {
    config.build.output = "..";
    return config;
  });
}`)
			_, err := pipeline.BuildWithContent(baseConfig(), contentMap)
			Expect(err).To(HaveOccurred(),
				"build.output set to '..' must be rejected — it resolves to the "+
					"parent of the project root. With clean: true, CleanOutputDir "+
					"would delete sibling project directories (issue #998)")
			Expect(err.Error()).To(ContainSubstring("build.output"),
				"error must identify build.output as the offending field (issue #998)")
			Expect(err.Error()).To(ContainSubstring("travers"),
				"error must indicate path traversal was the reason for rejection — "+
					"a bare '..' is a traversal, not an absolute path (issue #998)")
		})

		It("rejects current directory . as build.output", func() {
			// filepath.Clean("") returns ".". "." is not absolute and does
			// not start with "..", but it resolves to the project root itself.
			// With clean: true (default), CleanOutputDir would run os.RemoveAll
			// on every entry in the project root — deleting content, layouts,
			// plugins, and all source files.
			contentMap := baseContent("dot-output.js", `export default function(alloy) {
  alloy.hook('onConfig', {}, (config) => {
    config.build.output = ".";
    return config;
  });
}`)
			_, err := pipeline.BuildWithContent(baseConfig(), contentMap)
			Expect(err).To(HaveOccurred(),
				"build.output set to '.' must be rejected — it resolves to the "+
					"project root itself. With clean: true (default), CleanOutputDir "+
					"would delete all project source files including content, layouts, "+
					"and plugins (issue #998)")
			Expect(err.Error()).To(ContainSubstring("build.output"),
				"error must identify build.output as the offending field (issue #998)")
		})

		It("rejects empty string as build.output", func() {
			// filepath.Clean("") returns "." — same vulnerability as
			// explicitly setting ".". An empty string from a buggy plugin
			// must not silently target the project root.
			contentMap := baseContent("empty-output.js", `export default function(alloy) {
  alloy.hook('onConfig', {}, (config) => {
    config.build.output = "";
    return config;
  });
}`)
			_, err := pipeline.BuildWithContent(baseConfig(), contentMap)
			Expect(err).To(HaveOccurred(),
				"build.output set to '' (empty string) must be rejected — "+
					"filepath.Clean('') returns '.' which resolves to the project "+
					"root itself. Same destructive behavior as '.' (issue #998)")
			Expect(err.Error()).To(ContainSubstring("build.output"),
				"error must identify build.output as the offending field (issue #998)")
		})
	})
})
