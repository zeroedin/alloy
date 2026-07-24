package pipeline_test

import (
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/zeroedin/alloy/internal/config"
)

// setupWithPlugin creates a minimal project with a single content page and a
// plugin that uses the given JS source. Used by page_dependency_test.go for
// addDependencies tests. Defined at package level to resolve a scoping issue
// where the "addDependencies from onContentTransformed" Describe block
// references this helper before it is defined in the sibling "addDependencies
// validation" Describe block (issue #1135).
func setupWithPlugin(pluginJS string) *config.Config {
	tmpDir := GinkgoT().TempDir()
	contentDir := filepath.Join(tmpDir, "content")
	layoutsDir := filepath.Join(tmpDir, "layouts")
	pluginsDir := filepath.Join(tmpDir, "plugins")
	outputDir := filepath.Join(tmpDir, "_site")

	Expect(os.MkdirAll(contentDir, 0755)).To(Succeed())
	Expect(os.MkdirAll(layoutsDir, 0755)).To(Succeed())
	Expect(os.MkdirAll(pluginsDir, 0755)).To(Succeed())

	Expect(os.WriteFile(filepath.Join(contentDir, "index.md"),
		[]byte("---\ntitle: Home\nlayout: default\n---\n# Home"),
		0644)).To(Succeed())

	Expect(os.WriteFile(filepath.Join(layoutsDir, "default.liquid"),
		[]byte("<html><body>{{ content }}</body></html>"),
		0644)).To(Succeed())

	Expect(os.WriteFile(filepath.Join(pluginsDir, "test-plugin.js"),
		[]byte(pluginJS), 0644)).To(Succeed())

	cfg := &config.Config{
		Title:       "Validation Test",
		BaseURL:     "https://example.com",
		ProjectRoot: tmpDir,
		Build:       config.BuildConfig{Output: outputDir},
		Structure: config.StructureConfig{
			Content: "content",
			Layouts: "layouts",
		},
	}
	config.ApplyDefaults(cfg)
	return cfg
}

// setupDepTrackingProject creates a project with content pages and a plugin
// that returns addDependencies from onPageRendered. Defined at package level
// to resolve a scoping issue where the "onFileChanged invalidateByDependency
// integration" Describe block references this helper from a sibling scope
// (issue #1135).
func setupDepTrackingProject() (string, *config.Config) {
	tmpDir := GinkgoT().TempDir()
	contentDir := filepath.Join(tmpDir, "content")
	layoutsDir := filepath.Join(tmpDir, "layouts")
	pluginsDir := filepath.Join(tmpDir, "plugins")
	outputDir := filepath.Join(tmpDir, "_site")

	Expect(os.MkdirAll(contentDir, 0755)).To(Succeed())
	Expect(os.MkdirAll(layoutsDir, 0755)).To(Succeed())
	Expect(os.MkdirAll(pluginsDir, 0755)).To(Succeed())

	Expect(os.WriteFile(filepath.Join(contentDir, "index.md"),
		[]byte("---\ntitle: Home\nlayout: default\n---\n# Home\n<rh-card>content</rh-card>\n<rh-icon>icon</rh-icon>"),
		0644)).To(Succeed())
	Expect(os.WriteFile(filepath.Join(contentDir, "about.md"),
		[]byte("---\ntitle: About\nlayout: default\n---\n# About\n<rh-icon>icon</rh-icon>"),
		0644)).To(Succeed())
	Expect(os.WriteFile(filepath.Join(contentDir, "blog.md"),
		[]byte("---\ntitle: Blog\nlayout: default\n---\n# Blog\nNo components here."),
		0644)).To(Succeed())

	Expect(os.WriteFile(filepath.Join(layoutsDir, "default.liquid"),
		[]byte("<html><body>{{ content }}</body></html>"),
		0644)).To(Succeed())

	pluginJS := `export default function(alloy) {
  alloy.hook('onPageRendered', {}, function(page) {
    var deps = [];
    if (page.html.indexOf('rh-card') !== -1) {
      deps.push('elements/rh-card/rh-card.js');
    }
    if (page.html.indexOf('rh-icon') !== -1) {
      deps.push('elements/rh-icon/rh-icon.js');
    }
    if (deps.length > 0) {
      return { html: page.html, addDependencies: deps };
    }
    return page;
  });
}`
	Expect(os.WriteFile(filepath.Join(pluginsDir, "ssr-deps.js"),
		[]byte(pluginJS), 0644)).To(Succeed())

	cfg := &config.Config{
		Title:       "Dep Tracking Test",
		BaseURL:     "https://example.com",
		ProjectRoot: tmpDir,
		Build:       config.BuildConfig{Output: outputDir},
		Structure: config.StructureConfig{
			Content: "content",
			Layouts: "layouts",
		},
	}
	config.ApplyDefaults(cfg)

	return tmpDir, cfg
}
