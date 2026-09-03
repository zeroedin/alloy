package pipeline_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/zeroedin/alloy/internal/config"
	"github.com/zeroedin/alloy/internal/pipeline"
)

// ── Unterminated raw block line numbers (issue #1245) ─────────────────
//
// The spec fixes the error's message format but its content-layer tests use
// front-matter-free sources, so they cannot see which line the number is
// relative to. Every real content file has front matter — Alloy requires it —
// and RenderMarkdown only ever sees the body, so a body-relative number points
// the author at the wrong line in every file that exists.
//
// Page.BodyLine carries the offset and RenderMarkdownAt applies it.

var _ = Describe("Unterminated raw block line numbers (issue #1245)", func() {
	buildWithBody := func(body string) error {
		cfg := &config.Config{
			Title:   "Raw Block Line Number Test",
			BaseURL: "https://example.com",
			Build:   config.BuildConfig{Output: "_site"},
		}
		contentMap := map[string]string{
			"content/blog/post.md":   body,
			"layouts/default.liquid": `<html><body>{{ content }}</body></html>`,
			"plugins/shortcodes.js": `export default function(alloy) {
  alloy.shortcode("helmet", function(args, content) { return content; });
}`,
		}
		_, err := pipeline.BuildWithContent(cfg, contentMap)
		return err
	}

	It("reports the line as it appears in the file, not in the body", func() {
		// The open tag is on line 8 of the file: four front matter lines, a
		// blank, "intro", a blank, then the tag.
		err := buildWithBody("---\ntitle: Post\nlayout: default\n---\n" +
			"\nintro\n\n{%> helmet %}\n<script>oops</script>\n")
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("opened at line 8"),
			"the author opens the file at this line — the number must count the "+
				"front matter that RenderMarkdown never sees")
	})

	It("counts a longer front matter block correctly", func() {
		// Front matter runs lines 1-7, so the tag lands on line 8 again.
		err := buildWithBody("---\ntitle: Post\nlayout: default\ndraft: false\n"+
			"tags:\n  - go\n---\n"+
			"{%> helmet %}\nbody\n")
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("opened at line 8"))
	})

	It("still names the page path alongside the line", func() {
		err := buildWithBody("---\ntitle: Post\nlayout: default\n---\n{%> helmet %}\nbody\n")
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("blog/post.md"))
		Expect(err.Error()).To(ContainSubstring("opened at line 5"),
			"the tag is the first body line, immediately after four front matter lines")
	})
})
