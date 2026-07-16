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

var _ = Describe("onConfig passthrough path validation (issues #1031, #1034)", func() {
	// Passthrough from/to fields are filesystem paths that can be set by
	// plugins via onConfig hooks. Config-file passthrough intentionally
	// supports absolute from paths and ../relative for cross-project
	// asset sharing (§1h). But plugin-sourced paths are untrusted — a
	// malicious plugin could set from: "/etc/shadow" to exfiltrate files,
	// or to: "../../evil" to write outside the output directory. The same
	// path-containment rules used for build.output and structure.* fields
	// apply to plugin-sourced passthrough paths.
	//
	// Key difference from build.output/structure.*: passthrough to: "."
	// and to: "" are valid (they mean "copy to the root of the output
	// directory"), whereas build.output = "." is dangerous (it targets
	// the project root for CleanOutputDir deletion).

	baseConfig := func() *config.Config {
		return &config.Config{
			Title:   "Passthrough Path Test",
			BaseURL: "https://example.com",
			Build:   config.BuildConfig{Output: "_site"},
		}
	}

	baseContent := func(pluginName, pluginCode string) map[string]string {
		return map[string]string{
			"content/index.md":       "---\ntitle: Home\nlayout: default\n---\n# Home",
			"layouts/default.liquid": "<html><body>{{ content }}</body></html>",
			"plugins/" + pluginName:  pluginCode,
		}
	}

	// ── passthrough[].from: absolute path rejection ─────────────────

	Describe("passthrough from absolute path rejection", func() {
		It("rejects absolute path for passthrough[].from", func() {
			contentMap := baseContent("abs-from.js", `export default function(alloy) {
  alloy.hook('onConfig', {}, (config) => {
    config.passthrough = [{ from: "/etc/shadow", to: "exfil" }];
    return config;
  });
}`)
			_, err := pipeline.BuildWithContent(baseConfig(), contentMap)
			Expect(err).To(HaveOccurred(),
				"passthrough from set to an absolute path must be rejected — "+
					"a plugin could exfiltrate /etc/shadow into the output directory "+
					"by setting from to an absolute path. Config-file passthrough "+
					"allows absolute from by design, but plugin-sourced paths are "+
					"untrusted (issues #1031, #1034)")
			Expect(err.Error()).To(ContainSubstring("passthrough"),
				"error must identify passthrough as the source so plugin authors "+
					"know which onConfig field was rejected (issues #1031, #1034)")
			Expect(err.Error()).To(ContainSubstring("from"),
				"error must identify the from field specifically — passthrough "+
					"has both from and to fields and the plugin author needs to "+
					"know which one failed (issues #1031, #1034)")
			Expect(err.Error()).To(ContainSubstring("absolute"),
				"error must indicate an absolute path was the reason for rejection — "+
					"distinguishes from traversal rejection (issues #1031, #1034)")
		})
	})

	// ── passthrough[].to: absolute path rejection ───────────────────

	Describe("passthrough to absolute path rejection", func() {
		It("rejects absolute path for passthrough[].to", func() {
			contentMap := baseContent("abs-to.js", `export default function(alloy) {
  alloy.hook('onConfig', {}, (config) => {
    config.passthrough = [{ from: "vendor/assets", to: "/var/www/public" }];
    return config;
  });
}`)
			_, err := pipeline.BuildWithContent(baseConfig(), contentMap)
			Expect(err).To(HaveOccurred(),
				"passthrough to set to an absolute path must be rejected — "+
					"an absolute to path would cause CopyPassthrough to write "+
					"files outside the output directory (issues #1031, #1034)")
			Expect(err.Error()).To(ContainSubstring("passthrough"),
				"error must identify passthrough as the source (issues #1031, #1034)")
			Expect(err.Error()).To(ContainSubstring("to"),
				"error must identify the to field specifically — plugin author "+
					"needs to know which field in the mapping failed "+
					"(issues #1031, #1034)")
			Expect(err.Error()).To(ContainSubstring("absolute"),
				"error must indicate an absolute path was the reason for "+
					"rejection (issues #1031, #1034)")
		})
	})

	// ── passthrough[].from: relative traversal rejection ────────────

	Describe("passthrough from traversal rejection", func() {
		It("rejects traversal path for passthrough[].from", func() {
			contentMap := baseContent("trav-from.js", `export default function(alloy) {
  alloy.hook('onConfig', {}, (config) => {
    config.passthrough = [{ from: "../../etc", to: "exfil" }];
    return config;
  });
}`)
			_, err := pipeline.BuildWithContent(baseConfig(), contentMap)
			Expect(err).To(HaveOccurred(),
				"passthrough from with .. traversal above project root must be "+
					"rejected — the pipeline would read files from ../../etc and "+
					"copy them to the output directory. Config-file passthrough "+
					"allows ../relative for cross-project asset sharing, but "+
					"plugin-sourced paths are untrusted (issues #1031, #1034)")
			Expect(err.Error()).To(ContainSubstring("passthrough"),
				"error must identify passthrough as the source (issues #1031, #1034)")
			Expect(err.Error()).To(ContainSubstring("from"),
				"error must identify the from field (issues #1031, #1034)")
			Expect(err.Error()).To(ContainSubstring("travers"),
				"error must indicate path traversal was the reason for "+
					"rejection — distinguishes from absolute-path rejection "+
					"(issues #1031, #1034)")
		})

		It("rejects single .. for passthrough[].from", func() {
			contentMap := baseContent("dotdot-from.js", `export default function(alloy) {
  alloy.hook('onConfig', {}, (config) => {
    config.passthrough = [{ from: "..", to: "parent" }];
    return config;
  });
}`)
			_, err := pipeline.BuildWithContent(baseConfig(), contentMap)
			Expect(err).To(HaveOccurred(),
				"passthrough from set to '..' must be rejected — it resolves "+
					"to the parent of the project root, allowing a plugin to "+
					"read files above the project (issues #1031, #1034)")
			Expect(err.Error()).To(ContainSubstring("passthrough"),
				"error must identify passthrough as the source (issues #1031, #1034)")
			Expect(err.Error()).To(ContainSubstring("from"),
				"error must identify the from field (issues #1031, #1034)")
			Expect(err.Error()).To(ContainSubstring("travers"),
				"a bare '..' is a traversal, not an absolute path "+
					"(issues #1031, #1034)")
		})

		It("rejects embedded .. in passthrough[].from that escapes project root", func() {
			contentMap := baseContent("embedded-escape-from.js", `export default function(alloy) {
  alloy.hook('onConfig', {}, (config) => {
    config.passthrough = [{ from: "a/../../etc", to: "exfil" }];
    return config;
  });
}`)
			_, err := pipeline.BuildWithContent(baseConfig(), contentMap)
			Expect(err).To(HaveOccurred(),
				"passthrough from 'a/../../etc' cleans to '../etc' which "+
					"escapes the project root — must be rejected even though "+
					"the raw path starts with a safe component (issues #1031, #1034)")
			Expect(err.Error()).To(ContainSubstring("passthrough"),
				"error must identify passthrough as the source (issues #1031, #1034)")
			Expect(err.Error()).To(ContainSubstring("from"),
				"error must identify the from field (issues #1031, #1034)")
			Expect(err.Error()).To(ContainSubstring("travers"),
				"error must indicate path traversal — after filepath.Clean, "+
					"'a/../../etc' becomes '../etc' which starts with '..' "+
					"(issues #1031, #1034)")
		})
	})

	// ── passthrough[].to: relative traversal rejection ──────────────

	Describe("passthrough to traversal rejection", func() {
		It("rejects traversal path for passthrough[].to", func() {
			contentMap := baseContent("trav-to.js", `export default function(alloy) {
  alloy.hook('onConfig', {}, (config) => {
    config.passthrough = [{ from: "vendor/assets", to: "../../evil" }];
    return config;
  });
}`)
			_, err := pipeline.BuildWithContent(baseConfig(), contentMap)
			Expect(err).To(HaveOccurred(),
				"passthrough to with .. traversal must be rejected — "+
					"filepath.Join(outputDir, '../../evil') resolves outside "+
					"the output directory, writing files to an arbitrary "+
					"filesystem location (issues #1031, #1034)")
			Expect(err.Error()).To(ContainSubstring("passthrough"),
				"error must identify passthrough as the source (issues #1031, #1034)")
			Expect(err.Error()).To(ContainSubstring("to"),
				"error must identify the to field (issues #1031, #1034)")
			Expect(err.Error()).To(ContainSubstring("travers"),
				"error must indicate path traversal was the reason for "+
					"rejection (issues #1031, #1034)")
		})

		It("rejects single .. for passthrough[].to", func() {
			contentMap := baseContent("dotdot-to.js", `export default function(alloy) {
  alloy.hook('onConfig', {}, (config) => {
    config.passthrough = [{ from: "vendor/assets", to: ".." }];
    return config;
  });
}`)
			_, err := pipeline.BuildWithContent(baseConfig(), contentMap)
			Expect(err).To(HaveOccurred(),
				"passthrough to set to '..' must be rejected — files would be "+
					"written to the parent of the output directory "+
					"(issues #1031, #1034)")
			Expect(err.Error()).To(ContainSubstring("passthrough"),
				"error must identify passthrough as the source (issues #1031, #1034)")
			Expect(err.Error()).To(ContainSubstring("to"),
				"error must identify the to field (issues #1031, #1034)")
			Expect(err.Error()).To(ContainSubstring("travers"),
				"a bare '..' is a traversal (issues #1031, #1034)")
		})
	})

	// ── passthrough[].from: project root rejection ──────────────────

	Describe("passthrough from project root rejection", func() {
		It("rejects . for passthrough[].from", func() {
			contentMap := baseContent("dot-from.js", `export default function(alloy) {
  alloy.hook('onConfig', {}, (config) => {
    config.passthrough = [{ from: ".", to: "root-copy" }];
    return config;
  });
}`)
			_, err := pipeline.BuildWithContent(baseConfig(), contentMap)
			Expect(err).To(HaveOccurred(),
				"passthrough from set to '.' must be rejected — it would copy "+
					"the entire project root (including _site itself) into the "+
					"output directory, risking recursive copy and unbounded output "+
					"(issues #1031, #1034)")
			Expect(err.Error()).To(ContainSubstring("passthrough"),
				"error must identify passthrough as the source (issues #1031, #1034)")
			Expect(err.Error()).To(ContainSubstring("from"),
				"error must identify the from field (issues #1031, #1034)")
		})
	})

	// ── passthrough[].to: allow . and empty ─────────────────────────
	// Unlike build.output and structure.* fields, passthrough to: "."
	// and to: "" are valid. They mean "copy files to the root of the
	// output directory" — equivalent to filepath.Join(outputDir, ".").

	Describe("passthrough to allows . and empty", func() {
		It("allows . for passthrough[].to", func() {
			contentMap := baseContent("dot-to.js", `export default function(alloy) {
  alloy.hook('onConfig', {}, (config) => {
    config.passthrough = [{ from: "vendor/assets", to: "." }];
    return config;
  });
}`)
			// Provide the vendor/assets directory so the build does not
			// fail with "passthrough source does not exist".
			contentMap["vendor/assets/style.css"] = "body { color: red; }"
			result, err := pipeline.BuildWithContent(baseConfig(), contentMap)
			Expect(err).NotTo(HaveOccurred(),
				"passthrough to set to '.' must be accepted — it means 'copy "+
					"files to the root of the output directory'. Unlike build.output "+
					"where '.' targets the project root for CleanOutputDir deletion, "+
					"passthrough to is relative to the output directory and '.' is "+
					"a valid destination (issues #1031, #1034)")
			Expect(result).NotTo(BeNil())
		})

		It("allows empty string for passthrough[].to", func() {
			contentMap := baseContent("empty-to.js", `export default function(alloy) {
  alloy.hook('onConfig', {}, (config) => {
    config.passthrough = [{ from: "vendor/assets", to: "" }];
    return config;
  });
}`)
			contentMap["vendor/assets/script.js"] = "console.log('hello');"
			result, err := pipeline.BuildWithContent(baseConfig(), contentMap)
			Expect(err).NotTo(HaveOccurred(),
				"passthrough to set to '' (empty string) must be accepted — "+
					"empty to is the default behavior meaning 'root of output "+
					"directory'. It must not be rejected as a project-root "+
					"path (issues #1031, #1034)")
			Expect(result).NotTo(BeNil())
		})
	})

	// ── Safe relative paths accepted ────────────────────────────────

	Describe("safe relative paths accepted", func() {
		It("allows safe relative from and to paths", func() {
			contentMap := baseContent("safe-paths.js", `export default function(alloy) {
  alloy.hook('onConfig', {}, (config) => {
    config.passthrough = [{ from: "vendor/assets", to: "assets/vendor" }];
    return config;
  });
}`)
			contentMap["vendor/assets/lib.js"] = "// lib"
			result, err := pipeline.BuildWithContent(baseConfig(), contentMap)
			Expect(err).NotTo(HaveOccurred(),
				"passthrough with safe relative from and to paths must be "+
					"accepted — 'vendor/assets' is within the project root "+
					"and 'assets/vendor' is a valid output subdirectory "+
					"(issues #1031, #1034)")
			Expect(result).NotTo(BeNil())
		})

		It("allows from with embedded .. that resolves within project root", func() {
			contentMap := baseContent("safe-dotdot-from.js", `export default function(alloy) {
  alloy.hook('onConfig', {}, (config) => {
    config.passthrough = [{ from: "subdir/../vendor", to: "vendor-out" }];
    return config;
  });
}`)
			contentMap["vendor/data.json"] = `{"key": "value"}`
			result, err := pipeline.BuildWithContent(baseConfig(), contentMap)
			Expect(err).NotTo(HaveOccurred(),
				"passthrough from 'subdir/../vendor' cleans to 'vendor' which "+
					"is a valid relative path within the project root — must be "+
					"accepted, same as the build.output edge case (issues #1031, #1034)")
			Expect(result).NotTo(BeNil())
		})

		It("allows to with embedded .. that resolves safely", func() {
			contentMap := baseContent("safe-dotdot-to.js", `export default function(alloy) {
  alloy.hook('onConfig', {}, (config) => {
    config.passthrough = [{ from: "vendor/assets", to: "subdir/../assets" }];
    return config;
  });
}`)
			contentMap["vendor/assets/main.css"] = "body{}"
			result, err := pipeline.BuildWithContent(baseConfig(), contentMap)
			Expect(err).NotTo(HaveOccurred(),
				"passthrough to 'subdir/../assets' cleans to 'assets' which "+
					"is a valid output subdirectory — must be accepted "+
					"(issues #1031, #1034)")
			Expect(result).NotTo(BeNil())
		})
	})

	// ── Empty from filtering ────────────────────────────────────────
	// The existing passthrough deserialization filters out entries with
	// empty from values before validation runs. This means from: "" is
	// silently skipped (not rejected as ".") — it never reaches
	// validateOnConfigPath. This is safe: no files are copied for an
	// entry with no source path.

	Describe("empty from filtering", func() {
		It("silently skips passthrough entries with empty from", func() {
			contentMap := baseContent("empty-from.js", `export default function(alloy) {
  alloy.hook('onConfig', {}, (config) => {
    config.passthrough = [
      { from: "", to: "should-skip" },
      { from: "vendor/assets", to: "assets/vendor" }
    ];
    return config;
  });
}`)
			contentMap["vendor/assets/lib.js"] = "// lib"
			result, err := pipeline.BuildWithContent(baseConfig(), contentMap)
			Expect(err).NotTo(HaveOccurred(),
				"passthrough entry with from: '' must be silently skipped — "+
					"the existing empty-from filter removes the entry before "+
					"validation runs. filepath.Clean('') returns '.' which "+
					"would be rejected, but the filter prevents that path. "+
					"This is safe: no files are copied when from is empty "+
					"(issues #1031, #1034)")
			Expect(result).NotTo(BeNil())
		})
	})

	// ── Atomicity ───────────────────────────────────────────────────
	// Bad passthrough validation must prevent ALL config mutations,
	// including build.output and structure.* fields set in the same
	// hook return. This maintains the existing atomicity guarantee:
	// no partial config mutation on failure.
	//
	// This test calls applyOnConfigResult directly (via export_test.go)
	// rather than going through BuildWithContent, because BuildWithContent
	// does a struct copy of *cfg before passing it to Build — the test's
	// original cfg pointer is never mutated regardless of atomicity
	// implementation. Direct invocation ensures we observe whether the
	// validate-then-apply pattern actually prevents partial mutation.

	Describe("atomicity", func() {
		It("prevents all config mutations when passthrough validation fails", func() {
			cfg := baseConfig()
			resultMap := map[string]interface{}{
				"build": map[string]interface{}{
					"output": "dist",
				},
				"passthrough": []interface{}{
					map[string]interface{}{
						"from": "/etc/shadow",
						"to":   "exfil",
					},
				},
			}
			err := pipeline.ApplyOnConfigResult(cfg, resultMap)
			Expect(err).To(HaveOccurred(),
				"passthrough from set to absolute path must be rejected — "+
					"applyOnConfigResult must validate passthrough paths "+
					"before applying any config mutations "+
					"(issues #1031, #1034)")
			Expect(cfg.Build.Output).To(Equal("_site"),
				"build.output must remain '_site' (the original value) — "+
					"a passthrough validation failure must prevent ALL config "+
					"mutations, not just the passthrough field. The atomicity "+
					"guarantee requires that no structure/build fields are "+
					"applied when any validation fails (issues #1031, #1034)")
		})
	})

	// ── Error indexing ──────────────────────────────────────────────
	// When multiple passthrough mappings are present and a later mapping
	// has a bad path, the error message must include the zero-based
	// array index so the plugin author knows which entry to fix.

	Describe("error indexing", func() {
		It("includes the array index in error message for the failing mapping", func() {
			contentMap := baseContent("index-error.js", `export default function(alloy) {
  alloy.hook('onConfig', {}, (config) => {
    config.passthrough = [
      { from: "valid/dir", to: "valid" },
      { from: "also-valid/dir", to: "also-valid" },
      { from: "../../etc", to: "exfil" }
    ];
    return config;
  });
}`)
			_, err := pipeline.BuildWithContent(baseConfig(), contentMap)
			Expect(err).To(HaveOccurred(),
				"third mapping (index 2) has traversal in from — must be "+
					"rejected (issues #1031, #1034)")
			Expect(err.Error()).To(ContainSubstring("passthrough[2]"),
				"error must include the zero-based array index so plugin "+
					"authors know which mapping entry caused the failure — "+
					"'passthrough[2]' for the third entry (issues #1031, #1034)")
			Expect(err.Error()).To(ContainSubstring("from"),
				"error must identify the from field within the failing "+
					"mapping (issues #1031, #1034)")
			Expect(err.Error()).To(ContainSubstring("travers"),
				"error must indicate traversal as the violation type "+
					"(issues #1031, #1034)")
		})

		It("includes the array index for to field failures", func() {
			contentMap := baseContent("index-to-error.js", `export default function(alloy) {
  alloy.hook('onConfig', {}, (config) => {
    config.passthrough = [
      { from: "valid/dir", to: "valid" },
      { from: "another/dir", to: "/absolute/evil" }
    ];
    return config;
  });
}`)
			_, err := pipeline.BuildWithContent(baseConfig(), contentMap)
			Expect(err).To(HaveOccurred(),
				"second mapping (index 1) has absolute to — must be rejected "+
					"(issues #1031, #1034)")
			Expect(err.Error()).To(ContainSubstring("passthrough[1]"),
				"error must include 'passthrough[1]' for the second entry "+
					"(zero-indexed) (issues #1031, #1034)")
			Expect(err.Error()).To(ContainSubstring("to"),
				"error must identify the to field (issues #1031, #1034)")
			Expect(err.Error()).To(ContainSubstring("absolute"),
				"error must indicate absolute path as the violation "+
					"(issues #1031, #1034)")
		})
	})
})
