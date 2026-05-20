//go:build !windows

package plugin_test

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/zeroedin/alloy/internal/plugin"
)

var _ = Describe("NodeBridge", func() {

	// ── Message framing (LSP-style Content-Length) ─────────────────────

	Describe("Message framing", func() {
		It("encodes a hook message with Content-Length header", func() {
			msg := &plugin.Message{
				ID:      1,
				Type:    "hook",
				Name:    "onContentTransformed",
				Payload: []string{"page1", "page2"},
			}

			encoded, err := plugin.EncodeMessage(msg)
			Expect(err).NotTo(HaveOccurred())
			Expect(encoded).NotTo(BeEmpty())

			raw := string(encoded)

			// Must start with Content-Length header
			Expect(raw).To(HavePrefix("Content-Length: "))

			// Must contain \r\n\r\n separator between header and body
			Expect(raw).To(ContainSubstring("\r\n\r\n"))

			// Body after separator must be valid JSON
			parts := strings.SplitN(raw, "\r\n\r\n", 2)
			Expect(parts).To(HaveLen(2))

			var decoded map[string]interface{}
			Expect(json.Unmarshal([]byte(parts[1]), &decoded)).To(Succeed())

			// Content-Length must match actual body length
			bodyLen := len(parts[1])
			Expect(raw).To(HavePrefix(fmt.Sprintf("Content-Length: %d\r\n", bodyLen)))
		})

		It("decodes a framed response back to a Message struct", func() {
			body := `{"id":1,"result":{"status":"ok"}}`
			frame := fmt.Sprintf("Content-Length: %d\r\n\r\n%s", len(body), body)

			msg, err := plugin.DecodeMessage([]byte(frame))
			Expect(err).NotTo(HaveOccurred())
			Expect(msg).NotTo(BeNil())
			Expect(msg.ID).To(Equal(1))
			Expect(msg.Result).NotTo(BeNil())
		})

		It("roundtrip encode then decode produces equivalent message", func() {
			original := &plugin.Message{
				ID:   42,
				Type: "filter",
				Name: "slugify",
				Payload: map[string]interface{}{
					"input": "Hello World!",
				},
			}

			encoded, err := plugin.EncodeMessage(original)
			Expect(err).NotTo(HaveOccurred())

			decoded, err := plugin.DecodeMessage(encoded)
			Expect(err).NotTo(HaveOccurred())
			Expect(decoded.ID).To(Equal(original.ID))
			Expect(decoded.Type).To(Equal(original.Type))
			Expect(decoded.Name).To(Equal(original.Name))
		})

		It("rejects malformed frame missing Content-Length header", func() {
			// No Content-Length header, just raw JSON
			malformed := []byte(`{"id":1,"type":"hook","name":"onConfig"}`)

			_, err := plugin.DecodeMessage(malformed)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(
				SatisfyAny(
					ContainSubstring("Content-Length"),
					ContainSubstring("malformed"),
					ContainSubstring("frame"),
					ContainSubstring("header"),
				),
				"error should describe the framing problem",
			)
		})
	})

	// ── Message types ─────────────────────────────────────────────────

	Describe("Message types", func() {
		It("hook message serializes with type, name, and payload fields", func() {
			msg := &plugin.Message{
				ID:      1,
				Type:    "hook",
				Name:    "onContentTransformed",
				Payload: map[string]interface{}{"pages": 42},
			}

			encoded, err := plugin.EncodeMessage(msg)
			Expect(err).NotTo(HaveOccurred())

			// Extract JSON body after the header
			raw := string(encoded)
			parts := strings.SplitN(raw, "\r\n\r\n", 2)
			Expect(parts).To(HaveLen(2))

			var parsed map[string]interface{}
			Expect(json.Unmarshal([]byte(parts[1]), &parsed)).To(Succeed())

			Expect(parsed).To(HaveKeyWithValue("type", "hook"))
			Expect(parsed).To(HaveKeyWithValue("name", "onContentTransformed"))
			Expect(parsed).To(HaveKey("payload"))
		})

		It("SSR message serializes with instances array", func() {
			msg := &plugin.Message{
				ID:   2,
				Type: "ssr",
				Instances: []plugin.SSRInstance{
					{Hash: "abc123", HTML: "<ds-button>Click</ds-button>"},
					{Hash: "def456", HTML: "<ds-card>Content</ds-card>"},
				},
			}

			encoded, err := plugin.EncodeMessage(msg)
			Expect(err).NotTo(HaveOccurred())

			raw := string(encoded)
			parts := strings.SplitN(raw, "\r\n\r\n", 2)
			Expect(parts).To(HaveLen(2))

			var parsed map[string]interface{}
			Expect(json.Unmarshal([]byte(parts[1]), &parsed)).To(Succeed())

			Expect(parsed).To(HaveKeyWithValue("type", "ssr"))
			instances, ok := parsed["instances"].([]interface{})
			Expect(ok).To(BeTrue(), "instances should be an array")
			Expect(instances).To(HaveLen(2))
		})

		It("response message deserializes result field", func() {
			body := `{"id":2,"result":[{"hash":"abc123","html":"<ds-button><template shadowrootmode=\"open\">...</template></ds-button>"}]}`
			frame := fmt.Sprintf("Content-Length: %d\r\n\r\n%s", len(body), body)

			msg, err := plugin.DecodeMessage([]byte(frame))
			Expect(err).NotTo(HaveOccurred())
			Expect(msg).NotTo(BeNil())
			Expect(msg.ID).To(Equal(2))
			Expect(msg.Result).NotTo(BeNil())
		})
	})

	// ── Process lifecycle (state machine) ─────────────────────────────

	Describe("Process lifecycle", func() {
		It("NewNodeBridge creates bridge in not-started state", func() {
			bridge := plugin.NewNodeBridge("/project")
			Expect(bridge).NotTo(BeNil())
			Expect(bridge.State()).To(Equal(plugin.BridgeNotStarted))

			// The bridge must be startable for the state machine to be real
			err := bridge.Start()
			Expect(err).NotTo(HaveOccurred())
			Expect(bridge.State()).To(Equal(plugin.BridgeRunning))
		})

		It("Start transitions to running state", func() {
			bridge := plugin.NewNodeBridge("/project")
			err := bridge.Start()
			Expect(err).NotTo(HaveOccurred())
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

		It("stderr log path defaults to .alloy/plugin.log", func() {
			bridge := plugin.NewNodeBridge("/my-project")
			logPath := bridge.LogPath()
			Expect(logPath).To(Equal(filepath.Join("/my-project", ".alloy", "plugin.log")))
		})
	})

	// ── Process group isolation (#723) ────────────────────────────────

	Describe("Process group isolation", func() {
		It("spawns Node subprocess in its own process group", func() {
			bridge := plugin.NewNodeBridge("/project")
			err := bridge.Start()
			Expect(err).NotTo(HaveOccurred())
			defer bridge.Stop()

			pid := bridge.PID()
			Expect(pid).To(BeNumerically(">", 0))

			pgid, err := syscall.Getpgid(pid)
			Expect(err).NotTo(HaveOccurred())
			Expect(pgid).To(Equal(pid),
				"Node process should be the leader of its own process group (pgid == pid)")
		})

		It("Stop kills the entire process group, not just the leader", func() {
			bridge := plugin.NewNodeBridge("/project")
			err := bridge.Start()
			Expect(err).NotTo(HaveOccurred())
			defer bridge.Stop()

			pid := bridge.PID()
			Expect(pid).To(BeNumerically(">", 0))

			pgid, err := syscall.Getpgid(pid)
			Expect(err).NotTo(HaveOccurred())

			err = bridge.Stop()
			Expect(err).NotTo(HaveOccurred())

			err = syscall.Kill(-pgid, 0)
			Expect(err).To(HaveOccurred(),
				"process group should not exist after Stop")
		})
	})

	// ── Crash recovery via PID file (#723) ────────────────────────────

	Describe("PID file management", func() {
		var tmpDir string

		BeforeEach(func() {
			var err error
			tmpDir, err = os.MkdirTemp("", "alloy-pidfile-test-*")
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
	})

	// ── Process cleanup verification (#723) ───────────────────────────

	Describe("Process cleanup", func() {
		It("process is no longer running after Stop", func() {
			bridge := plugin.NewNodeBridge("/project")
			err := bridge.Start()
			Expect(err).NotTo(HaveOccurred())
			defer bridge.Stop()

			pid := bridge.PID()
			Expect(pid).To(BeNumerically(">", 0))

			err = bridge.Stop()
			Expect(err).NotTo(HaveOccurred())

			err = syscall.Kill(pid, 0)
			Expect(err).To(HaveOccurred(),
				"process should not exist after Stop")
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
	})
})
