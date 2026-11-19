//go:build !windows

package plugin_test

import (
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/zeroedin/alloy/internal/ordered"
	"github.com/zeroedin/alloy/internal/plugin"
)

// ── Node hooks keep the key order of objects (issue #1374) ──────────────
// An object a Node hook returns comes back to Alloy as an *ordered.Map with
// its keys in the order JavaScript gives them, at every level, as on
// QuickJS (issue #571). That holds for a hook called on its own and for
// hooks run in parallel on the worker pool (BatchCallHook), which no
// template can observe directly.
//
// The objects have 12 keys, in no sorted order. A Go map of 8 keys or
// fewer comes back as its keys in insertion order, rotated by a random
// amount, which matches 1 time in 8; with 12 keys a lost order fails.
//
// See PLAN.md → "Hook Serialization Boundary — Type Preservation".
var _ = Describe("Node hooks keep the key order of objects (issue #1374)", func() {
	want := []string{"zeta", "alpha", "mid", "beta", "omega", "gamma", "delta", "kappa", "iota", "eta", "theta", "lambda"}

	// The hook returns an object with the keys above, holding a nested
	// object with the same keys.
	const hookJS = `export const runtime = "node";
export default function(alloy) {
  alloy.hook('onPageRendered', { pages: true }, (page) => {
    const make = () => ({zeta: 1, alpha: 2, mid: 3, beta: 4, omega: 5, gamma: 6,
      delta: 7, kappa: 8, iota: 9, eta: 10, theta: 11, lambda: 12});
    const out = make();
    out.kappa = make();
    return out;
  });
}
`

	// keysOf returns the keys of v, which must be an *ordered.Map.
	keysOf := func(v interface{}) []string {
		GinkgoHelper()
		m, ok := v.(*ordered.Map)
		Expect(ok).To(BeTrue(), "a hook's object result must be an *ordered.Map, got %T", v)
		return m.Keys()
	}

	newRuntime := func() *plugin.NodeRuntime {
		GinkgoHelper()
		if plugin.CheckNodeAvailable() != nil {
			Skip("node is not installed")
		}
		dir := GinkgoT().TempDir()
		plugin.ResetStalePIDCleanup(dir)
		path := filepath.Join(dir, "order.js")
		Expect(os.WriteFile(path, []byte(hookJS), 0o644)).To(Succeed())
		rt := plugin.NewNodeRuntime()
		rt.SetProjectRoot(dir)
		DeferCleanup(rt.Close)
		Expect(rt.EvalFile(path)).To(Succeed())
		return rt
	}

	It("keeps the order of a hook's object result and of objects nested in it", func() {
		rt := newRuntime()
		result, err := rt.CallHook("onPageRendered", map[string]interface{}{"html": "<p>x</p>"})
		Expect(err).NotTo(HaveOccurred())
		Expect(keysOf(result)).To(Equal(want))
		Expect(keysOf(result.(*ordered.Map).Get("kappa"))).To(Equal(want), "nested object")
	})

	It("keeps the order of each result of hooks run in parallel on the worker pool", func() {
		rt := newRuntime()
		Expect(rt.PrepareWorkerPool(3)).To(Succeed())
		DeferCleanup(rt.CloseWorkers)

		payloads := make([]interface{}, 6)
		for i := range payloads {
			payloads[i] = map[string]interface{}{"html": "<p>x</p>"}
		}
		results, err := rt.BatchCallHook("onPageRendered", payloads, nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(results).To(HaveLen(len(payloads)))
		for i, result := range results {
			Expect(keysOf(result)).To(Equal(want), "result %d", i)
			Expect(keysOf(result.(*ordered.Map).Get("kappa"))).To(Equal(want), "result %d, nested object", i)
		}
	})
})
