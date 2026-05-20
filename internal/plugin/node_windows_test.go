//go:build windows

package plugin_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/zeroedin/alloy/internal/plugin"
)

var _ = Describe("NodeBridge (Windows)", func() {

	// ── Process lifecycle (Windows) ──────────────────────────────────

	Describe("Process lifecycle", func() {
		It("NewNodeBridge creates bridge in not-started state", func() {
			bridge := plugin.NewNodeBridge("/project")
			Expect(bridge).NotTo(BeNil())
			Expect(bridge.State()).To(Equal(plugin.BridgeNotStarted))
		})

		It("Start transitions to running state", func() {
			bridge := plugin.NewNodeBridge("/project")
			err := bridge.Start()
			Expect(err).NotTo(HaveOccurred())
			defer bridge.Stop()
			Expect(bridge.State()).To(Equal(plugin.BridgeRunning))
		})

		It("Stop after Start transitions to stopped state", func() {
			bridge := plugin.NewNodeBridge("/project")
			err := bridge.Start()
			Expect(err).NotTo(HaveOccurred())

			err = bridge.Stop()
			Expect(err).NotTo(HaveOccurred())
			Expect(bridge.State()).To(Equal(plugin.BridgeStopped))
		})

		It("Stop is idempotent — calling twice does not error", func() {
			bridge := plugin.NewNodeBridge("/project")
			err := bridge.Start()
			Expect(err).NotTo(HaveOccurred())

			err = bridge.Stop()
			Expect(err).NotTo(HaveOccurred())

			err = bridge.Stop()
			Expect(err).NotTo(HaveOccurred())
			Expect(bridge.State()).To(Equal(plugin.BridgeStopped))
		})

		It("PID returns 0 after Stop", func() {
			bridge := plugin.NewNodeBridge("/project")
			err := bridge.Start()
			Expect(err).NotTo(HaveOccurred())
			Expect(bridge.PID()).To(BeNumerically(">", 0))

			err = bridge.Stop()
			Expect(err).NotTo(HaveOccurred())
			Expect(bridge.PID()).To(Equal(0),
				"PID should be 0 after process is stopped")
		})

		It("process is no longer running after Stop", func() {
			bridge := plugin.NewNodeBridge("/project")
			err := bridge.Start()
			Expect(err).NotTo(HaveOccurred())

			pid := bridge.PID()
			Expect(pid).To(BeNumerically(">", 0))

			err = bridge.Stop()
			Expect(err).NotTo(HaveOccurred())

			p, err := os.FindProcess(pid)
			if err == nil {
				killErr := p.Signal(os.Kill)
				Expect(killErr).To(HaveOccurred(),
					"process should not be signalable after Stop")
			}
		})
	})

	// ── PID file management (Windows mutex-based locking) ────────────

	Describe("PID file management", func() {
		var tmpDir string

		BeforeEach(func() {
			var err error
			tmpDir, err = os.MkdirTemp("", "alloy-win-pidfile-test-*")
			Expect(err).NotTo(HaveOccurred())
		})

		AfterEach(func() {
			os.RemoveAll(tmpDir)
		})

		It("writes worker PIDs to .alloy/workers.pid on Start", func() {
			bridge := plugin.NewNodeBridge(tmpDir)
			err := bridge.Start()
			Expect(err).NotTo(HaveOccurred())
			defer bridge.Stop()

			pidFile := filepath.Join(tmpDir, ".alloy", "workers.pid")
			Expect(pidFile).To(BeAnExistingFile())

			data, err := os.ReadFile(pidFile)
			Expect(err).NotTo(HaveOccurred())
			Expect(string(data)).To(ContainSubstring(fmt.Sprintf("%d", bridge.PID())))
		})

		It("removes PID from .alloy/workers.pid on Stop", func() {
			bridge := plugin.NewNodeBridge(tmpDir)
			err := bridge.Start()
			Expect(err).NotTo(HaveOccurred())

			pidFile := filepath.Join(tmpDir, ".alloy", "workers.pid")
			Expect(pidFile).To(BeAnExistingFile())

			err = bridge.Stop()
			Expect(err).NotTo(HaveOccurred())

			if _, statErr := os.Stat(pidFile); statErr == nil {
				data, err := os.ReadFile(pidFile)
				Expect(err).NotTo(HaveOccurred())
				Expect(strings.TrimSpace(string(data))).To(BeEmpty(),
					"PID file should be empty after clean shutdown")
			}
		})

		It("cleans up stale PIDs from a previous session on next Start", func() {
			alloyDir := filepath.Join(tmpDir, ".alloy")
			Expect(os.MkdirAll(alloyDir, 0755)).To(Succeed())
			pidFile := filepath.Join(alloyDir, "workers.pid")

			stalePID := 2147483647
			Expect(os.WriteFile(pidFile, []byte(fmt.Sprintf("%d\n", stalePID)), 0644)).To(Succeed())

			bridge := plugin.NewNodeBridge(tmpDir)
			err := bridge.Start()
			Expect(err).NotTo(HaveOccurred())
			defer bridge.Stop()

			data, err := os.ReadFile(pidFile)
			Expect(err).NotTo(HaveOccurred())
			Expect(string(data)).NotTo(ContainSubstring(fmt.Sprintf("%d", stalePID)),
				"stale PID should be cleaned from the file on startup")
			Expect(string(data)).To(ContainSubstring(fmt.Sprintf("%d", bridge.PID())),
				"current PID should be in the file")
		})

		It("handles malformed entries in workers.pid without error", func() {
			alloyDir := filepath.Join(tmpDir, ".alloy")
			Expect(os.MkdirAll(alloyDir, 0755)).To(Succeed())
			pidFile := filepath.Join(alloyDir, "workers.pid")
			Expect(os.WriteFile(pidFile, []byte("not-a-number\n-1\n0\n\n"), 0644)).To(Succeed())

			bridge := plugin.NewNodeBridge(tmpDir)
			err := bridge.Start()
			Expect(err).NotTo(HaveOccurred())
			defer bridge.Stop()

			data, err := os.ReadFile(pidFile)
			Expect(err).NotTo(HaveOccurred())
			Expect(string(data)).To(ContainSubstring(fmt.Sprintf("%d", bridge.PID())),
				"current PID should be in the file after cleaning malformed entries")
		})
	})

	// ── Concurrent PID file access (Windows mutex-based) ─────────────

	Describe("Concurrent PID file access", func() {
		var tmpDir string

		BeforeEach(func() {
			var err error
			tmpDir, err = os.MkdirTemp("", "alloy-win-concurrent-pid-test-*")
			Expect(err).NotTo(HaveOccurred())
		})

		AfterEach(func() {
			os.RemoveAll(tmpDir)
		})

		It("multiple bridges writing to same PID file preserves all PIDs", func() {
			const numBridges = 4
			bridges := make([]*plugin.NodeBridge, numBridges)
			errs := make(chan error, numBridges)

			DeferCleanup(func() {
				for _, b := range bridges {
					if b != nil {
						b.Stop()
					}
				}
			})

			for i := 0; i < numBridges; i++ {
				go func(idx int) {
					b := plugin.NewNodeBridge(tmpDir)
					if err := b.Start(); err != nil {
						errs <- fmt.Errorf("bridge %d: %w", idx, err)
						return
					}
					bridges[idx] = b
					errs <- nil
				}(i)
			}

			for i := 0; i < numBridges; i++ {
				err := <-errs
				Expect(err).NotTo(HaveOccurred())
			}

			pidFile := filepath.Join(tmpDir, ".alloy", "workers.pid")
			data, err := os.ReadFile(pidFile)
			Expect(err).NotTo(HaveOccurred())

			content := string(data)
			for i, b := range bridges {
				Expect(content).To(ContainSubstring(fmt.Sprintf("%d", b.PID())),
					fmt.Sprintf("PID file should contain bridge %d PID", i))
			}
		})
	})
})
