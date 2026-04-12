package ssr_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/zeroedin/alloy/internal/ssr"
)

var _ = Describe("DepGraph", func() {

	// ── Dependency tracking ────────────────────────────────────────────

	Describe("Dependency tracking", func() {
		It("records parent-child relationships", func() {
			dg := ssr.NewDepGraph()
			dg.AddDependency("ds-card", "ds-icon")
			affected := dg.GetAffected("ds-icon")
			Expect(affected).To(ContainElement("ds-card"))
		})

		It("traverses transitive dependencies", func() {
			dg := ssr.NewDepGraph()
			dg.AddDependency("ds-sidebar", "ds-card")
			dg.AddDependency("ds-card", "ds-icon")
			// Changing ds-icon should affect ds-card and ds-sidebar
			affected := dg.GetAffected("ds-icon")
			Expect(affected).To(ContainElement("ds-card"))
			Expect(affected).To(ContainElement("ds-sidebar"))
		})
	})

	// ── Invalidation ───────────────────────────────────────────────────

	Describe("Invalidation", func() {
		It("returns all affected components for a change", func() {
			dg := ssr.NewDepGraph()
			dg.AddDependency("ds-card", "ds-icon")
			dg.AddDependency("ds-header", "ds-icon")
			affected := dg.GetAffected("ds-icon")
			Expect(affected).To(HaveLen(2))
		})

		It("ds-icon change affects ds-card and ds-sidebar per spec example", func() {
			dg := ssr.NewDepGraph()
			dg.AddDependency("ds-card", "ds-icon")
			dg.AddDependency("ds-sidebar", "ds-card")
			affected := dg.GetAffected("ds-icon")
			Expect(affected).To(ContainElement("ds-card"))
			Expect(affected).To(ContainElement("ds-sidebar"))
		})
	})

	// ── Definition change invalidation ────────────────────────────────

	Describe("Definition change invalidation", func() {
		It("returns pages affected when component definition changes", func() {
			dg := ssr.NewDepGraph()
			dg.AddDependency("ds-card", "ds-icon")
			dg.AddDependency("ds-header", "ds-icon")

			affected := dg.InvalidateByDefinition("ds-icon")
			Expect(affected).To(ConsistOf("ds-card", "ds-header"),
				"all components using the changed component must be invalidated")
		})
	})
})
