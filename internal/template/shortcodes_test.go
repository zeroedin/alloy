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
})
