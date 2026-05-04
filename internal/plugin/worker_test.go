package plugin_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/zeroedin/alloy/internal/plugin"
)

var _ = Describe("Worker Pool (issue #491)", func() {

	// ── Auto-scaling ────────────────────────────────────────────────
	// Worker count: min(NumCPU/2, 8) with floor 2.

	Context("AutoWorkerCount", func() {
		It("returns at least 2 (floor)", func() {
			count := plugin.AutoWorkerCount()
			Expect(count).To(BeNumerically(">=", 2),
				"auto worker count must be at least 2")
		})

		It("returns at most 8 (cap)", func() {
			count := plugin.AutoWorkerCount()
			Expect(count).To(BeNumerically("<=", 8),
				"auto worker count must be at most 8")
		})
	})

	// ── Config resolution ───────────────────────────────────────────
	// plugins.workers accepts "auto" (string) or integer.

	Context("ResolveWorkerCount", func() {
		It("auto resolves to AutoWorkerCount", func() {
			count := plugin.ResolveWorkerCount("auto")
			Expect(count).To(Equal(plugin.AutoWorkerCount()),
				"workers: auto must resolve to AutoWorkerCount()")
		})

		It("explicit integer is respected", func() {
			count := plugin.ResolveWorkerCount(4)
			Expect(count).To(Equal(4),
				"workers: 4 must resolve to 4")
		})

		It("explicit 1 is respected (no floor for explicit values)", func() {
			count := plugin.ResolveWorkerCount(1)
			Expect(count).To(Equal(1),
				"workers: 1 must resolve to 1 — "+
					"the floor of 2 only applies to auto-scaling, "+
					"not explicit config overrides")
		})

		It("0 falls back to auto", func() {
			count := plugin.ResolveWorkerCount(0)
			Expect(count).To(Equal(plugin.AutoWorkerCount()),
				"workers: 0 must fall back to auto")
		})

		It("negative falls back to auto", func() {
			count := plugin.ResolveWorkerCount(-1)
			Expect(count).To(Equal(plugin.AutoWorkerCount()),
				"workers: -1 must fall back to auto")
		})
	})
})
