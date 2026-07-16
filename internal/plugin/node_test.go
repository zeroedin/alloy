//go:build !windows

package plugin_test

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"unicode/utf8"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/zeroedin/alloy/internal/ordered"
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
			plugin.ResetStalePIDCleanup(tmpDir)
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

		It("kills a real stale process found in workers.pid on Start", func() {
			sleeper := exec.Command("sleep", "300")
			sleeper.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
			Expect(sleeper.Start()).To(Succeed())
			stalePID := sleeper.Process.Pid

			// Reap the child in a goroutine so it doesn't linger as a
			// zombie after cleanStalePIDs sends SIGTERM.  Kill(pid, 0)
			// returns nil for zombies, which caused the Eventually to
			// time out.
			reaped := make(chan struct{})
			go func() {
				_, _ = sleeper.Process.Wait()
				close(reaped)
			}()
			DeferCleanup(func() {
				_ = sleeper.Process.Kill()
				<-reaped
			})

			alloyDir := filepath.Join(tmpDir, ".alloy")
			Expect(os.MkdirAll(alloyDir, 0755)).To(Succeed())
			pidFile := filepath.Join(alloyDir, "workers.pid")
			Expect(os.WriteFile(pidFile, []byte(fmt.Sprintf("%d\n", stalePID)), 0644)).To(Succeed())

			bridge := plugin.NewNodeBridge(tmpDir)
			err := bridge.Start()
			Expect(err).NotTo(HaveOccurred())
			defer bridge.Stop()

			Eventually(func() error {
				return syscall.Kill(stalePID, 0)
			}, "10s", "100ms").Should(HaveOccurred(),
				"stale process should be killed during startup cleanup")
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

	// ── Concurrent PID file access (#726) ────────────────────────────

	Describe("Concurrent PID file access", func() {
		var tmpDir string

		BeforeEach(func() {
			var err error
			tmpDir, err = os.MkdirTemp("", "alloy-concurrent-pid-test-*")
			Expect(err).NotTo(HaveOccurred())
			plugin.ResetStalePIDCleanup(tmpDir)
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

	// ── Stdout isolation (#968) ──────────────────────────────────────

	Describe("Stdout isolation (#968)", func() {
		var tmpDir string
		var rt *plugin.NodeRuntime

		BeforeEach(func() {
			var err error
			tmpDir, err = os.MkdirTemp("", "alloy-stdout-isolation-*")
			Expect(err).NotTo(HaveOccurred())
			plugin.ResetStalePIDCleanup(tmpDir)
			rt = plugin.NewNodeRuntime()
			rt.SetProjectRoot(tmpDir)
		})

		AfterEach(func() {
			if rt != nil {
				rt.Close()
			}
			os.RemoveAll(tmpDir)
		})

		It("filter calling process.stdout.write returns correct result without protocol corruption", func() {
			fixturePath, err := filepath.Abs("testdata/single-files/stdout-write-filter.js")
			Expect(err).NotTo(HaveOccurred())

			err = rt.EvalFile(fixturePath)
			Expect(err).NotTo(HaveOccurred())
			Expect(rt.RegisteredFilters()).To(ContainElement("noisyFilter"),
				"filter should be registered despite no stdout write during eval")

			result, err := rt.CallFilter("noisyFilter", "hello world")
			Expect(err).NotTo(HaveOccurred(),
				"filter call must succeed — process.stdout.write must not corrupt the protocol")
			Expect(result).To(Equal("HELLO WORLD"),
				"filter should return correct transformed value despite stdout.write call")
		})

		It("top-level process.stdout.write during eval does not corrupt the registration handshake", func() {
			fixturePath, err := filepath.Abs("testdata/single-files/stdout-write-toplevel.js")
			Expect(err).NotTo(HaveOccurred())

			err = rt.EvalFile(fixturePath)
			Expect(err).NotTo(HaveOccurred(),
				"eval should succeed despite top-level stdout.write during module load")
			Expect(rt.RegisteredFilters()).To(ContainElement("cleanFilter"),
				"filter from the plugin should be registered after successful handshake")

			result, err := rt.CallFilter("cleanFilter", "test")
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal("test-processed"),
				"filter should return correct value proving the handshake completed intact")
		})

		It("console.log in a hook does not corrupt the protocol (regression)", func() {
			fixturePath, err := filepath.Abs("testdata/single-files/console-log-hook.js")
			Expect(err).NotTo(HaveOccurred())

			err = rt.EvalFile(fixturePath)
			Expect(err).NotTo(HaveOccurred())
			Expect(rt.RegisteredHooks()).To(ContainElement("onBuildComplete"),
				"hook should be registered after eval")

			payload := map[string]interface{}{"key": "value"}
			result, err := rt.CallHook("onBuildComplete", payload)
			Expect(err).NotTo(HaveOccurred(),
				"hook call must succeed — console.log must be redirected to stderr, not stdout")

			resultMap, ok := result.(*ordered.Map)
			Expect(ok).To(BeTrue(),
				"hook result should be an *ordered.Map after JSON round-trip rewrap")
			Expect(resultMap.Get("hookProcessed")).To(Equal(true),
				"hook should return modified payload proving it executed and round-tripped correctly")
			Expect(resultMap.Get("key")).To(Equal("value"),
				"original payload fields should be preserved through the hook")
		})
	})

	// ── Malformed frame diagnostic (#968) ────────────────────────────

	Describe("Malformed frame diagnostic (#968)", func() {
		It("DecodeMessage names stdout pollution as the likely cause when non-frame bytes are received", func() {
			garbage := []byte("some debug output from a plugin\nmore output")
			_, err := plugin.DecodeMessage(garbage)
			Expect(err).To(HaveOccurred())
			errMsg := err.Error()
			Expect(errMsg).To(ContainSubstring("stdout"),
				"error should name stdout pollution as the likely cause")
			Expect(errMsg).To(ContainSubstring("some debug output"),
				"error should include a snippet of the offending bytes for diagnosis")
		})

		It("DecodeMessage truncates long non-frame content in the diagnostic snippet", func() {
			longGarbage := make([]byte, 500)
			for i := range longGarbage {
				longGarbage[i] = 'x'
			}
			_, err := plugin.DecodeMessage(longGarbage)
			Expect(err).To(HaveOccurred())
			errMsg := err.Error()
			Expect(errMsg).To(ContainSubstring("stdout"),
				"error should name stdout pollution as the likely cause")
			Expect(len(errMsg)).To(BeNumerically("<", 300),
				"error message should be bounded, not echo 500 bytes of garbage verbatim")
		})

		It("Send reports stdout pollution diagnostic when non-frame bytes arrive on stdout", func() {
			garbage := "plugin debug output\n"
			reader := bufio.NewReader(strings.NewReader(garbage))
			bridge := plugin.NewBridgeWithReader(reader)

			_, err := bridge.Send(&plugin.Message{Type: "filter", Name: "test"})
			Expect(err).To(HaveOccurred())
			errMsg := err.Error()
			Expect(errMsg).To(ContainSubstring("stdout"),
				"Send error should name stdout pollution as the likely cause")
			Expect(errMsg).To(ContainSubstring("plugin debug output"),
				"Send error should include a snippet of the non-frame bytes")
		})

		It("Send reports stdout pollution diagnostic when garbage has no trailing newline", func() {
			// No trailing newline — ReadString('\n') returns data + io.EOF;
			// the pollution check must fire before the EOF error is returned.
			garbage := "garbage without trailing newline"
			reader := bufio.NewReader(strings.NewReader(garbage))
			bridge := plugin.NewBridgeWithReader(reader)

			_, err := bridge.Send(&plugin.Message{Type: "filter", Name: "test"})
			Expect(err).To(HaveOccurred())
			errMsg := err.Error()
			Expect(errMsg).To(ContainSubstring("stdout"),
				"Send error should name stdout pollution even when garbage lacks a trailing newline")
			Expect(errMsg).To(ContainSubstring("garbage without trailing newline"),
				"Send error should include the full non-newline-terminated snippet")
		})

		It("Send truncates long non-frame content in the diagnostic snippet at 80 chars", func() {
			// 500 chars of garbage, no newline — exercises truncateSnippet in Send path.
			// DecodeMessage has this test; Send must have parity.
			longGarbage := strings.Repeat("x", 500)
			reader := bufio.NewReader(strings.NewReader(longGarbage))
			bridge := plugin.NewBridgeWithReader(reader)

			_, err := bridge.Send(&plugin.Message{Type: "filter", Name: "test"})
			Expect(err).To(HaveOccurred())
			errMsg := err.Error()
			Expect(errMsg).To(ContainSubstring("stdout"),
				"error should name stdout pollution as the likely cause")
			Expect(len(errMsg)).To(BeNumerically("<", 300),
				"error message should be bounded, not echo 500 bytes of garbage verbatim")
			Expect(errMsg).To(ContainSubstring(strings.Repeat("x", 80)+"..."),
				"snippet should contain exactly 80 chars of input followed by ellipsis")
		})

		It("Send diagnostic snippet handles binary/non-UTF8 data without panicking", func() {
			// ASCII prefix followed by 100 invalid UTF-8 continuation bytes (0x80).
			// Exercises truncateSnippet's rune-boundary walk-back on invalid sequences.
			data := "BINARY:" + strings.Repeat("\x80", 100)
			reader := bufio.NewReader(strings.NewReader(data))
			bridge := plugin.NewBridgeWithReader(reader)

			_, err := bridge.Send(&plugin.Message{Type: "filter", Name: "test"})
			Expect(err).To(HaveOccurred())
			errMsg := err.Error()
			Expect(errMsg).To(ContainSubstring("stdout"),
				"error should name stdout pollution as the likely cause")
			Expect(len(errMsg)).To(BeNumerically("<", 300),
				"error message should be bounded even with binary content")
			Expect(errMsg).To(ContainSubstring("BINARY:..."),
				"invalid bytes should be stripped at a valid UTF-8 rune boundary, preserving ASCII prefix")
			Expect(utf8.ValidString(errMsg)).To(BeTrue(),
				"error message must be valid UTF-8 after rune-boundary walk-back")
		})
	})

	// ── Plugin source registration (issue #979) ─────────────────────
	// alloy.source(name, fn) in bridge.js registers a data source handler.
	// The eval response includes a "sources" array. NodeRuntime exposes
	// RegisteredSources() and CallSource() for bridge-backed invocation.

	Describe("Plugin source registration (issue #979)", func() {
		var tmpDir string
		var rt *plugin.NodeRuntime

		BeforeEach(func() {
			var err error
			tmpDir, err = os.MkdirTemp("", "alloy-source-test-*")
			Expect(err).NotTo(HaveOccurred())
			plugin.ResetStalePIDCleanup(tmpDir)
			rt = plugin.NewNodeRuntime()
			rt.SetProjectRoot(tmpDir)
		})

		AfterEach(func() {
			if rt != nil {
				rt.Close()
			}
			os.RemoveAll(tmpDir)
		})

		It("EvalFile reports registered sources from bridge eval response", func() {
			fixturePath, err := filepath.Abs("testdata/single-files/source-plugin.js")
			Expect(err).NotTo(HaveOccurred())

			err = rt.EvalFile(fixturePath)
			Expect(err).NotTo(HaveOccurred())
			Expect(rt.RegisteredSources()).To(ContainElement("test-source"),
				"source registered via alloy.source() must appear in RegisteredSources()")
			Expect(rt.RegisteredSources()).To(HaveLen(1),
				"only one source was registered — list must have exactly one entry")
		})

		It("CallSource invokes the registered source handler and returns data", func() {
			fixturePath, err := filepath.Abs("testdata/single-files/source-plugin.js")
			Expect(err).NotTo(HaveOccurred())

			err = rt.EvalFile(fixturePath)
			Expect(err).NotTo(HaveOccurred())

			result, err := rt.CallSource("test-source", nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())

			arr, ok := result.([]interface{})
			Expect(ok).To(BeTrue(), "source handler must return an array")
			Expect(arr).To(HaveLen(2))

			first, ok := arr[0].(map[string]interface{})
			Expect(ok).To(BeTrue(), "array elements must be maps after JSON round-trip")
			Expect(first["title"]).To(Equal("Post 1"))
			Expect(first["slug"]).To(Equal("post-1"))

			second, ok := arr[1].(map[string]interface{})
			Expect(ok).To(BeTrue())
			Expect(second["title"]).To(Equal("Post 2"))
		})

		It("CallSource propagates handler errors", func() {
			fixturePath, err := filepath.Abs("testdata/single-files/source-error.js")
			Expect(err).NotTo(HaveOccurred())

			err = rt.EvalFile(fixturePath)
			Expect(err).NotTo(HaveOccurred())

			_, err = rt.CallSource("failing-source", nil)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("503"),
				"error from the JS handler must propagate through the bridge")
		})

		It("CallSource returns error for unregistered source name", func() {
			fixturePath, err := filepath.Abs("testdata/single-files/source-plugin.js")
			Expect(err).NotTo(HaveOccurred())

			err = rt.EvalFile(fixturePath)
			Expect(err).NotTo(HaveOccurred())

			_, err = rt.CallSource("nonexistent-source", nil)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(SatisfyAny(
				ContainSubstring("nonexistent-source"),
				ContainSubstring("not found"),
				ContainSubstring("not registered"),
			), "error must identify the missing source handler")
		})

		It("duplicate alloy.source() registration for same name produces a warning", func() {
			fixturePath, err := filepath.Abs("testdata/single-files/source-duplicate.js")
			Expect(err).NotTo(HaveOccurred())

			err = rt.EvalFile(fixturePath)
			Expect(err).NotTo(HaveOccurred())

			warnings := rt.EvalWarnings()
			found := false
			for _, w := range warnings {
				if strings.Contains(w, "dup-source") && strings.Contains(w, "duplicate") {
					found = true
					break
				}
			}
			Expect(found).To(BeTrue(),
				"registering the same source name twice must produce a warning — "+
					"same pattern as duplicate hook registration")
		})
	})
})
