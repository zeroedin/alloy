package template_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	tmpl "github.com/zeroedin/alloy/internal/template"
)

var _ = Describe("Shortcodes", func() {

	// ── Inline shortcodes ──────────────────────────────────────────────

	Describe("Inline shortcodes", func() {
		It("RegisterShortcode registers without error", func() {
			fn := func(args []string) string { return "ok" }
			err := tmpl.RegisterShortcode("greeting", fn)
			Expect(err).NotTo(HaveOccurred())
		})

		It("registered shortcode function is callable", func() {
			called := false
			fn := func(args []string) string {
				called = true
				return "hello " + args[0]
			}
			err := tmpl.RegisterShortcode("hello", fn)
			Expect(err).NotTo(HaveOccurred())

			result := fn([]string{"world"})
			Expect(called).To(BeTrue())
			Expect(result).To(Equal("hello world"))
		})
	})

	// ── Block shortcodes ───────────────────────────────────────────────

	Describe("Block shortcodes", func() {
		It("RegisterBlockShortcode registers without error", func() {
			fn := func(args []string, content string) string { return content }
			err := tmpl.RegisterBlockShortcode("note", fn)
			Expect(err).NotTo(HaveOccurred())
		})

		It("block shortcode receives inner content", func() {
			var received string
			fn := func(args []string, content string) string {
				received = content
				return "<div class=\"note\">" + content + "</div>"
			}
			err := tmpl.RegisterBlockShortcode("note", fn)
			Expect(err).NotTo(HaveOccurred())

			result := fn([]string{}, "This is a note.")
			Expect(received).To(Equal("This is a note."))
			Expect(result).To(Equal("<div class=\"note\">This is a note.</div>"))
		})
	})

	// ── Shortcode template integration ────────────────────────────────

	Describe("Shortcode template integration", func() {
		It("expands inline shortcode syntax in template source", func() {
			err := tmpl.RegisterShortcode("youtube", func(args []string) string {
				return `<iframe src="https://youtube.com/embed/` + args[0] + `"></iframe>`
			})
			Expect(err).NotTo(HaveOccurred())

			source := `<p>Watch this:</p>{% youtube "dQw4w9WgXcQ" %}`
			result, renderErr := tmpl.RenderShortcodes(source)
			Expect(renderErr).NotTo(HaveOccurred())
			Expect(result).To(ContainSubstring("dQw4w9WgXcQ"))
		})

		It("expands block shortcode syntax with inner content", func() {
			err := tmpl.RegisterBlockShortcode("callout", func(args []string, content string) string {
				return `<div class="callout callout-` + args[0] + `">` + content + `</div>`
			})
			Expect(err).NotTo(HaveOccurred())

			source := `{% callout "warning" %}Be careful!{% endcallout %}`
			result, renderErr := tmpl.RenderShortcodes(source)
			Expect(renderErr).NotTo(HaveOccurred())
			Expect(result).To(ContainSubstring(`callout-warning`))
			Expect(result).To(ContainSubstring("Be careful!"))
		})

		It("handles shortcodes with no arguments", func() {
			err := tmpl.RegisterShortcode("hr", func(args []string) string {
				return "<hr/>"
			})
			Expect(err).NotTo(HaveOccurred())

			source := `Before{% hr %}After`
			result, renderErr := tmpl.RenderShortcodes(source)
			Expect(renderErr).NotTo(HaveOccurred())
			Expect(result).To(ContainSubstring("<hr/>"))
		})
	})
})
