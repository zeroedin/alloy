package content_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/zeroedin/alloy/internal/content"
)

// ── Whitespace control on raw block open tags (issue #1245) ───────────
//
// IMPLEMENTATION.md has Open() extract the tag name by "skipping whitespace
// and any whitespace-control `-`" after the marker, so `{%>- name -%}` is a
// valid raw open. Stripping the marker must leave the dash flush against the
// delimiter: `{%- name -%}` is Liquid's trim marker, `{% - name -%}` is a
// syntax error. The spec's tests only cover markers followed by whitespace.

var _ = Describe("Raw block marker stripping with whitespace control (issue #1245)", func() {
	rawMD := content.CreateGoldmark(content.MarkdownOptions{
		Unsafe: true, Typographer: true, TemplateTags: true,
	})

	It("keeps a Liquid trim dash flush against the delimiter", func() {
		source := []byte("{%>- helmet -%}\nbody\n{%- endhelmet -%}\n")
		out, _, err := content.RenderMarkdown(source, rawMD)
		Expect(err).NotTo(HaveOccurred())
		html := string(out)
		Expect(html).To(ContainSubstring("{%- helmet -%}"),
			"stripping must not separate the trim dash from the delimiter — "+
				"liquidgo reads `{%-` as one token")
		Expect(html).NotTo(ContainSubstring("{% - helmet"),
			"`{% - helmet -%}` is a Liquid syntax error")
		Expect(html).To(ContainSubstring("{%- endhelmet -%}"),
			"the close tag is emitted unchanged")
	})

	It("keeps a Go template trim dash flush against the delimiter", func() {
		source := []byte("{{%>- helmet -%}}\nbody\n{{% /helmet %}}\n")
		out, _, err := content.RenderMarkdown(source, rawMD)
		Expect(err).NotTo(HaveOccurred())
		Expect(string(out)).To(ContainSubstring("{{%- helmet -%}}"))
		Expect(string(out)).NotTo(ContainSubstring("{{% - helmet"))
	})

	It("still inserts one space when no trim dash is present", func() {
		source := []byte("{%>helmet %}\nbody\n{% endhelmet %}\n")
		out, _, err := content.RenderMarkdown(source, rawMD)
		Expect(err).NotTo(HaveOccurred())
		Expect(string(out)).To(ContainSubstring("{% helmet %}"),
			"the normalizing behavior the spec requires is unchanged")
	})
})
