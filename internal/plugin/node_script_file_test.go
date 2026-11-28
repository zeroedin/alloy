//go:build linux

package plugin_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/zeroedin/alloy/internal/plugin"
)

// ── Each Node bridge runs its own copy of the bridge script (issue #1450) ──
// Every bridge in a project used to write and run the same file,
// .alloy/bridge.mjs. A worker pool starts its bridges at the same time, so:
//   - starting a bridge rewrote the file while another bridge's Node was
//     loading it. Node read an empty or cut-off file and exited, and the next
//     write to it failed with "broken pipe";
//   - stopping a bridge deleted the file while another bridge still needed it.
//
// Each bridge now writes its own script file in <projectRoot>/.alloy/, runs
// it, and deletes only that file when it stops. No other bridge writes or
// deletes it.
//
// The specs read the script a bridge's Node runs from /proc/<pid>/cmdline,
// so they hold whatever the files are named. Linux only.
//
// See PLAN.md → "Tier 3: Node Subprocess (opt-in)" → "Each bridge runs its
// own copy of the bridge script".
var _ = Describe("Each Node bridge runs its own bridge script (issue #1450)", func() {
	var dir string

	BeforeEach(func() {
		if plugin.CheckNodeAvailable() != nil {
			Skip("node is not installed")
		}
		dir = GinkgoT().TempDir()
		plugin.ResetStalePIDCleanup(dir)
	})

	// scriptOf returns the script the Node process with this PID runs: the
	// last argument of its command line.
	scriptOf := func(pid int) string {
		GinkgoHelper()
		Expect(pid).To(BeNumerically(">", 0), "the bridge must be running")
		raw, err := os.ReadFile(fmt.Sprintf("/proc/%d/cmdline", pid))
		Expect(err).NotTo(HaveOccurred())
		args := strings.Split(strings.TrimRight(string(raw), "\x00"), "\x00")
		Expect(len(args)).To(BeNumerically(">=", 2), "node's command line: %q", args)
		return args[len(args)-1]
	}

	start := func() *plugin.NodeBridge {
		GinkgoHelper()
		b := plugin.NewNodeBridge(dir)
		Expect(b.Start()).To(Succeed())
		DeferCleanup(b.Stop)
		return b
	}

	It("writes the script in the project's .alloy folder, as an .mjs file", func() {
		// Guard (issue #248): Node resolves import() from the project's
		// node_modules only when the script is inside the project.
		script := scriptOf(start().PID())
		Expect(filepath.Dir(script)).To(Equal(filepath.Join(dir, ".alloy")))
		Expect(script).To(HaveSuffix(".mjs"))
		Expect(script).To(BeARegularFile())
	})

	It("gives two bridges in the same project different script files", func() {
		a, b := start(), start()
		Expect(scriptOf(a.PID())).NotTo(Equal(scriptOf(b.PID())))
	})

	It("gives each bridge of a worker pool its own script file", func() {
		path := filepath.Join(dir, "p.js")
		Expect(os.WriteFile(path, []byte(`export const runtime = "node";
export default function(alloy) { alloy.hook('onPageRendered', { pages: true }, (p) => p); }
`), 0o644)).To(Succeed())
		rt := plugin.NewNodeRuntime()
		rt.SetProjectRoot(dir)
		DeferCleanup(rt.Close)
		Expect(rt.EvalFile(path)).To(Succeed())
		Expect(rt.PrepareWorkerPool(3)).To(Succeed())
		DeferCleanup(rt.CloseWorkers)

		// The main bridge and 2 workers: every node process running a
		// script in this project's .alloy folder.
		alloyDir := filepath.Join(dir, ".alloy") + string(filepath.Separator)
		scripts := map[string]int{}
		procs, err := filepath.Glob("/proc/[0-9]*/cmdline")
		Expect(err).NotTo(HaveOccurred())
		for _, p := range procs {
			raw, err := os.ReadFile(p)
			if err != nil {
				continue // the process has exited
			}
			args := strings.Split(strings.TrimRight(string(raw), "\x00"), "\x00")
			if last := args[len(args)-1]; strings.HasPrefix(last, alloyDir) {
				scripts[last]++
			}
		}
		Expect(scripts).To(HaveLen(3), "3 bridges, 3 script files: %v", scripts)
	})

	It("leaves a running bridge's script in place when another bridge stops", func() {
		a := start()
		script := scriptOf(a.PID())
		before, err := os.ReadFile(script)
		Expect(err).NotTo(HaveOccurred())

		b := plugin.NewNodeBridge(dir)
		Expect(b.Start()).To(Succeed())
		Expect(b.Stop()).To(Succeed())

		after, err := os.ReadFile(script)
		Expect(err).NotTo(HaveOccurred(), "a's script after b stopped")
		Expect(after).To(Equal(before))
	})

	It("doesn't write to a running bridge's script when another bridge starts", func() {
		a := start()
		script := scriptOf(a.PID())

		// Set the time back, so any write shows, however coarse the clock.
		past := time.Now().Add(-time.Hour).Truncate(time.Second)
		Expect(os.Chtimes(script, past, past)).To(Succeed())

		start()

		info, err := os.Stat(script)
		Expect(err).NotTo(HaveOccurred())
		Expect(info.ModTime()).To(Equal(past), "a's script was written when b started")
	})

	It("deletes its own script when it stops", func() {
		// Guard: a clean stop leaves no script behind.
		b := plugin.NewNodeBridge(dir)
		Expect(b.Start()).To(Succeed())
		script := scriptOf(b.PID())
		Expect(b.Stop()).To(Succeed())
		Expect(script).NotTo(BeAnExistingFile())
	})
})
