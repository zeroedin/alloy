package validation_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/zeroedin/alloy/internal/content"
	"github.com/zeroedin/alloy/internal/validation"
)

var _ = Describe("Output path conflict detection", func() {

	Describe("DetectConflicts", func() {

		Context("no conflicts", func() {
			It("returns empty slice and nil error when all paths are unique", func() {
				entries := []validation.OutputPathEntry{
					{Path: "/about/index.html", Source: "content/about.md"},
					{Path: "/blog/index.html", Source: "content/blog/index.md"},
					{Path: "/contact/index.html", Source: "content/contact.md"},
				}
				conflicts, err := validation.DetectConflicts(entries)
				Expect(err).NotTo(HaveOccurred())
				Expect(conflicts).To(BeEmpty())
			})

			It("handles empty path list", func() {
				entries := []validation.OutputPathEntry{}
				conflicts, err := validation.DetectConflicts(entries)
				Expect(err).NotTo(HaveOccurred())
				Expect(conflicts).To(BeEmpty())
			})
		})

		Context("conflict detection", func() {
			It("detects two content files targeting the same output path", func() {
				entries := []validation.OutputPathEntry{
					{Path: "/about/index.html", Source: "content/about.md"},
					{Path: "/about/index.html", Source: "content/about/index.md"},
				}
				conflicts, err := validation.DetectConflicts(entries)
				Expect(err).NotTo(HaveOccurred())
				Expect(conflicts).To(HaveLen(1))
				Expect(conflicts[0].Path).To(Equal("/about/index.html"))
				Expect(conflicts[0].Sources).To(ConsistOf("content/about.md", "content/about/index.md"))
			})

			It("detects conflict between content file and static file", func() {
				entries := []validation.OutputPathEntry{
					{Path: "/favicon.ico", Source: "content/favicon.ico"},
					{Path: "/favicon.ico", Source: "static/favicon.ico"},
				}
				conflicts, err := validation.DetectConflicts(entries)
				Expect(err).NotTo(HaveOccurred())
				Expect(conflicts).To(HaveLen(1))
				Expect(conflicts[0].Path).To(Equal("/favicon.ico"))
				Expect(conflicts[0].Sources).To(ConsistOf("content/favicon.ico", "static/favicon.ico"))
			})

			It("detects conflict between static file and passthrough mapping", func() {
				entries := []validation.OutputPathEntry{
					{Path: "/assets/fonts/body.woff2", Source: "static/assets/fonts/body.woff2"},
					{Path: "/assets/fonts/body.woff2", Source: "passthrough:../design-system/dist/fonts"},
				}
				conflicts, err := validation.DetectConflicts(entries)
				Expect(err).NotTo(HaveOccurred())
				Expect(conflicts).To(HaveLen(1))
				Expect(conflicts[0].Path).To(Equal("/assets/fonts/body.woff2"))
				Expect(conflicts[0].Sources).To(ConsistOf(
					"static/assets/fonts/body.woff2",
					"passthrough:../design-system/dist/fonts",
				))
			})

			It("detects conflict between content file and alias", func() {
				entries := []validation.OutputPathEntry{
					{Path: "/old-post/index.html", Source: "content/old-post.md"},
					{Path: "/old-post/index.html", Source: "alias:content/new-post.md"},
				}
				conflicts, err := validation.DetectConflicts(entries)
				Expect(err).NotTo(HaveOccurred())
				Expect(conflicts).To(HaveLen(1))
				Expect(conflicts[0].Path).To(Equal("/old-post/index.html"))
				Expect(conflicts[0].Sources).To(ConsistOf("content/old-post.md", "alias:content/new-post.md"))
			})
		})

		Context("error reporting", func() {
			It("includes both conflicting source paths in conflict", func() {
				entries := []validation.OutputPathEntry{
					{Path: "/index.html", Source: "content/index.md"},
					{Path: "/index.html", Source: "content/home.md"},
				}
				conflicts, err := validation.DetectConflicts(entries)
				Expect(err).NotTo(HaveOccurred())
				Expect(conflicts).To(HaveLen(1))
				Expect(conflicts[0].Sources).To(HaveLen(2))
				Expect(conflicts[0].Sources).To(ContainElement("content/index.md"))
				Expect(conflicts[0].Sources).To(ContainElement("content/home.md"))
			})

			It("reports all conflicts, not just the first one", func() {
				entries := []validation.OutputPathEntry{
					{Path: "/about/index.html", Source: "content/about.md"},
					{Path: "/about/index.html", Source: "content/about/index.md"},
					{Path: "/contact/index.html", Source: "content/contact.md"},
					{Path: "/contact/index.html", Source: "static/contact/index.html"},
				}
				conflicts, err := validation.DetectConflicts(entries)
				Expect(err).NotTo(HaveOccurred())
				Expect(conflicts).To(HaveLen(2))

				paths := []string{conflicts[0].Path, conflicts[1].Path}
				Expect(paths).To(ConsistOf("/about/index.html", "/contact/index.html"))
			})
		})

		Context("error format contracts", func() {
			It("conflict error includes both conflicting source paths", func() {
				entries := []validation.OutputPathEntry{
					{Path: "about/index.html", Source: "about.md"},
					{Path: "about/index.html", Source: "about.html"},
				}
				conflicts, err := validation.DetectConflicts(entries)
				Expect(err).NotTo(HaveOccurred())
				Expect(conflicts).NotTo(BeEmpty())
				errMsg := conflicts[0].Path + ": " + conflicts[0].Sources[0] + ", " + conflicts[0].Sources[1]
				Expect(errMsg).To(ContainSubstring("about.md"),
					"conflict error must include first source path")
				Expect(errMsg).To(ContainSubstring("about.html"),
					"conflict error must include second source path")
			})
		})

		Context("auto-generated file conflicts", func() {
			It("detects conflict between content page and sitemap.xml", func() {
				entries := []validation.OutputPathEntry{
					{Path: "/sitemap.xml", Source: "auto:sitemap"},
					{Path: "/sitemap.xml", Source: "content/sitemap.xml"},
				}
				conflicts, err := validation.DetectConflicts(entries)
				Expect(err).NotTo(HaveOccurred())
				Expect(conflicts).To(HaveLen(1))
				Expect(conflicts[0].Sources).To(ConsistOf("auto:sitemap", "content/sitemap.xml"))
			})

			It("detects conflict between content page and feed.xml", func() {
				entries := []validation.OutputPathEntry{
					{Path: "/feed.xml", Source: "auto:feed"},
					{Path: "/feed.xml", Source: "content/feed.xml"},
				}
				conflicts, err := validation.DetectConflicts(entries)
				Expect(err).NotTo(HaveOccurred())
				Expect(conflicts).To(HaveLen(1))
				Expect(conflicts[0].Sources).To(ConsistOf("auto:feed", "content/feed.xml"))
			})
		})

		Context("pagination virtual page conflicts", func() {
			It("detects conflict between pagination page and existing content", func() {
				entries := []validation.OutputPathEntry{
					{Path: "/blog/page/2/index.html", Source: "content/blog/page/2.md"},
					{Path: "/blog/page/2/index.html", Source: "pagination:blog"},
				}
				conflicts, err := validation.DetectConflicts(entries)
				Expect(err).NotTo(HaveOccurred())
				Expect(conflicts).To(HaveLen(1))
				Expect(conflicts[0].Sources).To(ConsistOf(
					"content/blog/page/2.md",
					"pagination:blog",
				))
			})
		})

		Context("taxonomy page conflicts", func() {
			It("detects conflict between taxonomy page and existing content", func() {
				entries := []validation.OutputPathEntry{
					{Path: "/tags/go/index.html", Source: "content/tags/go.md"},
					{Path: "/tags/go/index.html", Source: "taxonomy:tags/go"},
				}
				conflicts, err := validation.DetectConflicts(entries)
				Expect(err).NotTo(HaveOccurred())
				Expect(conflicts).To(HaveLen(1))
				Expect(conflicts[0].Sources).To(ConsistOf(
					"content/tags/go.md",
					"taxonomy:tags/go",
				))
			})
		})

		Context("plugin-registered path conflicts", func() {
			It("detects conflict between plugin path and content page", func() {
				entries := []validation.OutputPathEntry{
					{Path: "/api/data.json", Source: "content/api/data.md"},
					{Path: "/api/data.json", Source: "plugin:my-api-plugin"},
				}
				conflicts, err := validation.DetectConflicts(entries)
				Expect(err).NotTo(HaveOccurred())
				Expect(conflicts).To(HaveLen(1))
				Expect(conflicts[0].Sources).To(ConsistOf(
					"content/api/data.md",
					"plugin:my-api-plugin",
				))
			})
		})
	})
})

var _ = Describe("Permalink-alias validation", func() {

	Describe("ValidatePermalinkAliases", func() {
		It("flags permalink:false pages that also have aliases", func() {
			pages := []*content.Page{
				{RelPath: "data-only.md", Permalink: "", Aliases: []string{"/old/"}},
			}
			errs := validation.ValidatePermalinkAliases(pages)
			Expect(errs).NotTo(BeEmpty(),
				"permalink:false + aliases must produce a validation error")
		})
	})
})
