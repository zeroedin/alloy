//go:build !windows

package plugin_test

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unicode/utf8"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/zeroedin/alloy/internal/jsonutil"
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
			Expect(jsonutil.JSON.Unmarshal([]byte(parts[1]), &decoded)).To(Succeed())

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
			Expect(jsonutil.JSON.Unmarshal([]byte(parts[1]), &parsed)).To(Succeed())

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
			Expect(jsonutil.JSON.Unmarshal([]byte(parts[1]), &parsed)).To(Succeed())

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
			Expect(warnings).To(ContainElement(And(
				ContainSubstring("dup-source"),
				ContainSubstring("duplicate"),
			)), "registering the same source name twice must produce a warning — "+
				"same pattern as duplicate hook registration")
		})
	})

	// ── Plugin source error paths (issues #1043, #1045) ──────────────
	// CallSource must handle error conditions cleanly:
	// - bridge==nil (runtime not started) → descriptive error (#1045)
	// - slow handler → timeout error, not infinite hang (#1043)

	Describe("Plugin source error paths (issues #1043, #1045)", func() {

		It("CallSource returns error when bridge has not been started (issue #1045)", func() {
			// A NodeRuntime that has never had EvalFile called has bridge==nil.
			// CallSource must produce a descriptive error, not panic.
			rt := plugin.NewNodeRuntime()
			DeferCleanup(func() { rt.Close() })

			_, err := rt.CallSource("any-source", map[string]interface{}{"key": "val"})
			Expect(err).To(HaveOccurred(),
				"CallSource on a runtime with no bridge must return an error")
			Expect(err.Error()).To(SatisfyAll(
				ContainSubstring("any-source"),
				SatisfyAny(
					ContainSubstring("bridge"),
					ContainSubstring("not started"),
					ContainSubstring("not initialized"),
				),
			), "error must identify the source name and indicate the bridge is not started — "+
				"this distinguishes 'runtime not ready' from 'source not registered'")
		})

		It("CallSource returns timeout error for a slow source handler (issue #1043)", NodeTimeout(15*time.Second), func(_ SpecContext) {
			tmpDir, err := os.MkdirTemp("", "alloy-source-timeout-*")
			Expect(err).NotTo(HaveOccurred())
			DeferCleanup(func() { os.RemoveAll(tmpDir) })
			plugin.ResetStalePIDCleanup(tmpDir)

			rt := plugin.NewNodeRuntime()
			rt.SetProjectRoot(tmpDir)
			DeferCleanup(func() { rt.Close() })

			fixturePath, err := filepath.Abs("testdata/single-files/source-slow.js")
			Expect(err).NotTo(HaveOccurred())

			err = rt.EvalFile(fixturePath)
			Expect(err).NotTo(HaveOccurred())
			Expect(rt.RegisteredSources()).To(ContainElement("slow-source"))

			// CallSource must apply a timeout (cfg.Plugins.Timeout, default 5s)
			// and return a timeout error — NOT hang indefinitely.
			done := make(chan struct{})
			var callErr error
			go func() {
				_, callErr = rt.CallSource("slow-source", nil)
				close(done)
			}()

			// The plugin timeout default is 5s. Allow 8s for overhead.
			// If CallSource has no timeout, this Eventually fails because
			// done never closes within 8 seconds.
			Eventually(done, "8s").Should(BeClosed(),
				"CallSource must not hang indefinitely — it should apply "+
					"cfg.Plugins.Timeout and return within the timeout window")

			Expect(callErr).To(HaveOccurred(),
				"slow source handler must produce an error, not succeed after timeout")
			Expect(callErr.Error()).To(SatisfyAny(
				ContainSubstring("timeout"),
				ContainSubstring("deadline exceeded"),
				ContainSubstring("context canceled"),
			), "error must indicate the source call timed out — currently CallSource "+
				"uses bridge.Send() with no timeout, unlike hook calls which go through "+
				"RunWithTimeout with context.WithTimeout")
		})
	})

	// ── Split-body binary framing (issue #1181) ──────────────────
	// Hook payloads with large HTML/content fields use split-body framing
	// to avoid JSON-encoding the large string. The large field is stripped
	// from the JSON body and sent as raw bytes after it, with additional
	// headers (X-Body-Length, X-Body-Field) describing the split.

	Describe("Split-body binary framing (issue #1181)", func() {

		// parseSplitFrame extracts headers, JSON body, and raw body from
		// a split-body frame. Fails the test if the frame is malformed.
		parseSplitFrame := func(data []byte) (headers map[string]string, jsonBody []byte, rawBody []byte) {
			raw := string(data)
			sepIdx := strings.Index(raw, "\r\n\r\n")
			Expect(sepIdx).To(BeNumerically(">=", 0), "frame must contain \\r\\n\\r\\n separator")

			headerBlock := raw[:sepIdx]
			afterSep := data[sepIdx+4:]

			headers = make(map[string]string)
			for _, line := range strings.Split(headerBlock, "\r\n") {
				if kv := strings.SplitN(line, ": ", 2); len(kv) == 2 {
					headers[kv[0]] = kv[1]
				}
			}

			clStr, hasContentLen := headers["Content-Length"]
			Expect(hasContentLen).To(BeTrue(), "frame must have Content-Length header")
			contentLen, err := strconv.Atoi(clStr)
			Expect(err).NotTo(HaveOccurred())

			jsonBody = afterSep[:contentLen]
			rawBody = afterSep[contentLen:]
			return
		}

		// ── EncodeMessage outbound framing ───────────────────────

		Describe("EncodeMessage outbound framing", func() {

			DescribeTable("emits split-body headers for hook payloads with large string fields",
				func(payload interface{}, expectedField string, expectedBody string) {
					msg := &plugin.Message{
						Type:    "hook",
						Name:    "testHook",
						Payload: payload,
					}

					encoded, err := plugin.EncodeMessage(msg)
					Expect(err).NotTo(HaveOccurred())

					raw := string(encoded)

					// Must have split-body headers (suffix \r\n prevents 50 matching 500)
					Expect(raw).To(ContainSubstring(fmt.Sprintf("X-Body-Length: %d\r\n", len(expectedBody))),
						"X-Body-Length must equal the byte length of the split field value")
					Expect(raw).To(ContainSubstring(fmt.Sprintf("X-Body-Field: %s\r\n", expectedField)),
						"X-Body-Field must name the field sent as raw bytes")

					headers, jsonBody, rawBody := parseSplitFrame(encoded)

					// Header value must match actual raw body length
					Expect(headers["X-Body-Length"]).To(Equal(strconv.Itoa(len(rawBody))),
						"X-Body-Length header value must match the actual raw body byte count")

					// JSON body must be valid and must NOT contain the split field
					var parsed map[string]interface{}
					Expect(jsonutil.JSON.Unmarshal(jsonBody, &parsed)).To(Succeed(),
						"JSON portion of split-body frame must be valid JSON")

					payloadMap, ok := parsed["payload"].(map[string]interface{})
					Expect(ok).To(BeTrue(), "payload must be a JSON object in the frame body")
					Expect(payloadMap).NotTo(HaveKey(expectedField),
						expectedField+" must be stripped from JSON body — "+
							"the whole point of split-body is to avoid JSON-encoding this field")

					// Raw body must be the exact field value (no JSON escaping)
					Expect(string(rawBody)).To(Equal(expectedBody),
						"raw body bytes after JSON must equal the split field value exactly — "+
							"no JSON escaping, no length-prefix, just raw bytes")
				},
				Entry("HookRenderedPayload — html field",
					plugin.HookRenderedPayload{
						HTML:        "<h1>Hello</h1><p>Page with \"quotes\" and <tags></p>",
						FrontMatter: map[string]interface{}{"title": "Test"},
						URL:         "/test/",
						Path:        "test.md",
					},
					"html",
					"<h1>Hello</h1><p>Page with \"quotes\" and <tags></p>",
				),
				Entry("HookFormatRenderedPayload — content field",
					plugin.HookFormatRenderedPayload{
						Content:     `{"key":"value","items":[1,2,3]}`,
						Format:      "json",
						URL:         "/api/",
						Path:        "api.md",
						FrontMatter: map[string]interface{}{"title": "API"},
					},
					"content",
					`{"key":"value","items":[1,2,3]}`,
				),
				Entry("HookTransformPayload — html field",
					plugin.HookTransformPayload{
						HTML:        "<h2>Intro</h2><p>Transformed content</p>",
						FrontMatter: map[string]interface{}{"title": "Guide"},
						URL:         "/docs/guide/",
						Path:        "docs/guide.md",
					},
					"html",
					"<h2>Intro</h2><p>Transformed content</p>",
				),
				Entry("map[string]interface{} with html key — chained hook result",
					map[string]interface{}{
						"html":        "<p>chained result</p>",
						"frontMatter": map[string]interface{}{"title": "Chained"},
						"url":         "/chained/",
						"path":        "chained.md",
					},
					"html",
					"<p>chained result</p>",
				),
				Entry("map[string]interface{} with content key — chained onFormatRendered result",
					map[string]interface{}{
						"content":     "processed format body",
						"format":      "json",
						"url":         "/api/data/",
						"path":        "api/data.md",
						"frontMatter": map[string]interface{}{},
					},
					"content",
					"processed format body",
				),
				Entry("multi-byte characters — X-Body-Length is byte count not rune count",
					plugin.HookRenderedPayload{
						HTML:        "<p>日本語テスト 🎉 données françaises</p>",
						FrontMatter: map[string]interface{}{"title": "i18n"},
						URL:         "/i18n/",
						Path:        "i18n.md",
					},
					"html",
					"<p>日本語テスト 🎉 données françaises</p>",
				),
			)

			It("preserves single-header format for non-hook messages", func() {
				msg := &plugin.Message{
					Type: "filter",
					Name: "slugify",
					Payload: map[string]interface{}{
						"input": "Hello World",
						"html":  "<p>this html key must not trigger split-body for filter messages</p>",
					},
				}

				encoded, err := plugin.EncodeMessage(msg)
				Expect(err).NotTo(HaveOccurred())

				raw := string(encoded)
				Expect(raw).To(HavePrefix("Content-Length: "),
					"non-hook message must use standard Content-Length framing")
				Expect(raw).NotTo(ContainSubstring("X-Body-Length"),
					"non-hook message must not emit X-Body-Length — "+
						"split-body is reserved for hook payloads with large HTML/content fields")
				Expect(raw).NotTo(ContainSubstring("X-Body-Field"),
					"non-hook message must not emit X-Body-Field")

				// The full payload including html must be in the JSON body
				sepIdx := strings.Index(raw, "\r\n\r\n")
				Expect(sepIdx).To(BeNumerically(">=", 0))
				jsonBody := encoded[sepIdx+4:]

				var parsed map[string]interface{}
				Expect(jsonutil.JSON.Unmarshal(jsonBody, &parsed)).To(Succeed())

				payloadMap, ok := parsed["payload"].(map[string]interface{})
				Expect(ok).To(BeTrue())
				Expect(payloadMap).To(HaveKey("html"),
					"html must remain in JSON body for non-hook messages — "+
						"split-body optimization only applies to hook type messages")
			})

			It("falls back to single-header format when split field is empty string", func() {
				msg := &plugin.Message{
					Type: "hook",
					Name: "onPageRendered",
					Payload: plugin.HookRenderedPayload{
						HTML:        "", // empty — no benefit from split-body
						FrontMatter: map[string]interface{}{"title": "Empty"},
						URL:         "/empty/",
						Path:        "empty.md",
					},
				}

				encoded, err := plugin.EncodeMessage(msg)
				Expect(err).NotTo(HaveOccurred())

				raw := string(encoded)
				Expect(raw).NotTo(ContainSubstring("X-Body-Length"),
					"empty split field must not trigger split-body — "+
						"X-Body-Length: 0 has no performance benefit and adds unnecessary protocol complexity")
				Expect(raw).NotTo(ContainSubstring("X-Body-Field"),
					"empty split field must not trigger split-body headers")
			})

			It("splits html field when map payload contains both html and content keys", func() {
				msg := &plugin.Message{
					Type: "hook",
					Name: "testHook",
					Payload: map[string]interface{}{
						"html":        "<p>the html field</p>",
						"content":     "the content field",
						"frontMatter": map[string]interface{}{},
					},
				}

				encoded, err := plugin.EncodeMessage(msg)
				Expect(err).NotTo(HaveOccurred())

				raw := string(encoded)
				Expect(raw).To(ContainSubstring("X-Body-Field: html\r\n"),
					"when both html and content keys exist in a map payload, "+
						"html must take priority — it is the larger field in the common case "+
						"(onPageRendered ~800KB vs onFormatRendered ~100KB)")

				_, jsonBody, rawBody := parseSplitFrame(encoded)

				var parsed map[string]interface{}
				Expect(jsonutil.JSON.Unmarshal(jsonBody, &parsed)).To(Succeed())
				payloadMap, ok := parsed["payload"].(map[string]interface{})
				Expect(ok).To(BeTrue())
				Expect(payloadMap).NotTo(HaveKey("html"),
					"html must be stripped from JSON body")
				Expect(payloadMap).To(HaveKey("content"),
					"content must remain in JSON body when html is the split field")

				Expect(string(rawBody)).To(Equal("<p>the html field</p>"),
					"raw body must be the html field value")
			})

			It("does not mutate the caller's map payload", func() {
				original := map[string]interface{}{
					"html":        "<p>original</p>",
					"frontMatter": map[string]interface{}{"title": "Test"},
					"url":         "/test/",
				}

				msg := &plugin.Message{
					Type:    "hook",
					Name:    "testHook",
					Payload: original,
				}

				_, err := plugin.EncodeMessage(msg)
				Expect(err).NotTo(HaveOccurred())

				Expect(original).To(HaveKey("html"),
					"EncodeMessage must not delete keys from the caller's map — "+
						"the same map may be reused for chained hook dispatch or logging")
				Expect(original["html"]).To(Equal("<p>original</p>"),
					"html value in caller's map must be unchanged after EncodeMessage")
			})

			It("uses standard single-header framing for hook message with map payload lacking html/content keys", func() {
				// Issue #1188: when hooks chain, the result of a non-page hook
				// (e.g., onDataFetched, onBuildComplete) is a map[string]interface{}
				// without html or content keys. EncodeMessage must NOT use
				// split-body framing for these payloads — the key guard must
				// reject maps that lack the page-like html/content signature.
				msg := &plugin.Message{
					Type: "hook",
					Name: "onBuildComplete",
					Payload: map[string]interface{}{
						"pageCount": 42,
						"duration":  "1.5s",
						"outputDir": "_site",
						"errors":    []interface{}{},
					},
				}

				encoded, err := plugin.EncodeMessage(msg)
				Expect(err).NotTo(HaveOccurred())

				raw := string(encoded)
				Expect(raw).To(HavePrefix("Content-Length: "),
					"non-page-like map must use standard Content-Length framing")
				Expect(raw).NotTo(ContainSubstring("X-Body-Length"),
					"map[string]interface{} without html/content keys must NOT "+
						"emit X-Body-Length — split-body is reserved for page-like "+
						"payloads with large html/content fields (issue #1188)")
				Expect(raw).NotTo(ContainSubstring("X-Body-Field"),
					"map[string]interface{} without html/content keys must NOT "+
						"emit X-Body-Field — the key guard must reject non-page maps")

				// Verify the full payload is present in the JSON body
				parts := strings.SplitN(raw, "\r\n\r\n", 2)
				Expect(parts).To(HaveLen(2))
				var parsed map[string]interface{}
				Expect(jsonutil.JSON.Unmarshal([]byte(parts[1]), &parsed)).To(Succeed())
				payloadMap, ok := parsed["payload"].(map[string]interface{})
				Expect(ok).To(BeTrue())
				Expect(payloadMap).To(HaveKeyWithValue("pageCount", BeNumerically("==", 42)),
					"all fields must be present in the JSON body when standard framing is used")
				Expect(payloadMap).To(HaveKeyWithValue("duration", "1.5s"),
					"non-page payload fields must be preserved in JSON body")
				Expect(payloadMap).To(HaveKeyWithValue("outputDir", "_site"),
					"non-page payload fields must be preserved in JSON body")
			})
		})

		// ── DecodeMessage split-body parsing ─────────────────────

		Describe("DecodeMessage split-body parsing", func() {

			It("parses multi-header frame and injects raw body into result map under X-Body-Field name", func() {
				jsonPart := `{"id":1,"result":{"processed":true}}`
				rawPart := "<p>injected html content</p>"

				frame := fmt.Sprintf(
					"Content-Length: %d\r\nX-Body-Length: %d\r\nX-Body-Field: html\r\n\r\n%s%s",
					len(jsonPart), len(rawPart), jsonPart, rawPart,
				)

				msg, err := plugin.DecodeMessage([]byte(frame))
				Expect(err).NotTo(HaveOccurred())
				Expect(msg).NotTo(BeNil())
				Expect(msg.ID).To(Equal(1))

				result, ok := msg.Result.(map[string]interface{})
				Expect(ok).To(BeTrue(),
					"Result must be a map after JSON unmarshal + split-body injection")
				Expect(result).To(HaveKeyWithValue("processed", true),
					"existing JSON fields in Result must be preserved after split-body injection")
				Expect(result).To(HaveKeyWithValue("html", rawPart),
					"raw body bytes must be injected into Result map under the X-Body-Field name — "+
						"this is how the large string field is reassembled without JSON deserialization")
			})
		})

		// ── Send multi-header response reading ───────────────────

		Describe("Send multi-header response reading", func() {

			It("reads multi-header response and injects raw body into result under correct field name", func() {
				jsonPart := `{"id":1,"result":{"status":"ok"}}`
				rawPart := "<div>rendered page content</div>"

				response := fmt.Sprintf(
					"Content-Length: %d\r\nX-Body-Length: %d\r\nX-Body-Field: html\r\n\r\n%s%s",
					len(jsonPart), len(rawPart), jsonPart, rawPart,
				)

				reader := bufio.NewReader(strings.NewReader(response))
				bridge := plugin.NewBridgeWithReader(reader)

				resp, err := bridge.Send(&plugin.Message{Type: "hook", Name: "test"})
				Expect(err).NotTo(HaveOccurred())
				Expect(resp).NotTo(BeNil())

				result, ok := resp.Result.(map[string]interface{})
				Expect(ok).To(BeTrue(),
					"Result must be a map after multi-header response reading")
				Expect(result).To(HaveKeyWithValue("status", "ok"),
					"existing JSON fields must be preserved through multi-header response reading")
				Expect(result).To(HaveKeyWithValue("html", rawPart),
					"raw body bytes must be injected into Result under X-Body-Field name — "+
						"this proves Send's multi-header parser correctly reads X-Body-Length "+
						"bytes after the JSON body and injects them as a string value")
			})
		})

		// ── Split-body edge cases (issue #1194) ─────────────────

		Describe("EncodeMessage → DecodeMessage round-trip fidelity", func() {

			It("round-trips a split-body hook message preserving the split field in payload", func() {
				original := &plugin.Message{
					Type: "hook",
					Name: "onPageRendered",
					Payload: plugin.HookRenderedPayload{
						HTML:        "<h1>Round-trip test</h1><p>Content with \"quotes\" and <tags></p>",
						FrontMatter: map[string]interface{}{"title": "Round-trip"},
						URL:         "/round-trip/",
						Path:        "round-trip.md",
					},
				}

				encoded, err := plugin.EncodeMessage(original)
				Expect(err).NotTo(HaveOccurred())

				// Verify the encoded frame IS split-body (not single-header)
				Expect(string(encoded)).To(ContainSubstring("X-Body-Length:"),
					"encoded frame must use split-body framing — if this fails, "+
						"EncodeMessage is not detecting the hook payload type")

				decoded, err := plugin.DecodeMessage(encoded)
				Expect(err).NotTo(HaveOccurred())
				Expect(decoded).NotTo(BeNil())

				Expect(decoded.Type).To(Equal("hook"),
					"message type must survive the round-trip")
				Expect(decoded.Name).To(Equal("onPageRendered"),
					"message name must survive the round-trip")

				// The split field must be injected back into the decoded message.
				// When Result is nil (outbound message, not a response), DecodeMessage
				// injects into Payload if it's a map.
				payloadMap, ok := decoded.Payload.(map[string]interface{})
				Expect(ok).To(BeTrue(),
					"Payload must be a map[string]interface{} after JSON round-trip "+
						"of a struct payload (EncodeMessage converts structs to maps via "+
						"JSON marshal/unmarshal in stripField)")
				Expect(payloadMap).To(HaveKeyWithValue("html",
					"<h1>Round-trip test</h1><p>Content with \"quotes\" and <tags></p>"),
					"the split html field must be injected back into the Payload map "+
						"during DecodeMessage — this is the round-trip fidelity guarantee: "+
						"encode strips the field from JSON and sends it as raw bytes, "+
						"decode reads those raw bytes and injects them back")
				Expect(payloadMap).To(HaveKeyWithValue("url", "/round-trip/"),
					"non-split fields must be preserved through the round-trip")
				Expect(payloadMap).To(HaveKeyWithValue("path", "round-trip.md"),
					"non-split fields must be preserved through the round-trip")
				Expect(payloadMap).To(HaveKey("frontMatter"),
					"frontMatter must be preserved through the round-trip")
			})
		})

		Describe("DecodeMessage malformed split-body frames", func() {

			It("returns error when Content-Length exceeds available bytes (truncated frame)", func() {
				jsonPart := `{"id":1,"result":{"status":"ok"}}`
				// Claim Content-Length is much larger than the actual body
				frame := fmt.Sprintf(
					"Content-Length: %d\r\n\r\n%s",
					len(jsonPart)+500, // 500 bytes more than actually present
					jsonPart,
				)

				_, err := plugin.DecodeMessage([]byte(frame))
				Expect(err).To(HaveOccurred(),
					"DecodeMessage must reject frames where Content-Length exceeds available bytes")
				Expect(err.Error()).To(SatisfyAny(
					ContainSubstring("Content-Length"),
					ContainSubstring("available bytes"),
					ContainSubstring("truncated"),
				), "error must indicate the frame is truncated or Content-Length is invalid")
			})

			It("returns error when X-Body-Length is negative", func() {
				jsonPart := `{"id":1,"result":{"status":"ok"}}`
				frame := fmt.Sprintf(
					"Content-Length: %d\r\nX-Body-Length: -1\r\nX-Body-Field: html\r\n\r\n%s",
					len(jsonPart),
					jsonPart,
				)

				_, err := plugin.DecodeMessage([]byte(frame))
				Expect(err).To(HaveOccurred(),
					"DecodeMessage must reject frames with negative X-Body-Length — "+
						"strconv.Atoi(\"-1\") succeeds, so this tests explicit bounds checking")
				Expect(err.Error()).To(ContainSubstring("X-Body-Length"),
					"error must identify X-Body-Length as the invalid header")
			})

			It("returns error when X-Body-Length is present but X-Body-Field is missing", func() {
				jsonPart := `{"id":1,"result":{"status":"ok"}}`
				frame := fmt.Sprintf(
					"Content-Length: %d\r\nX-Body-Length: 10\r\n\r\n%s0123456789",
					len(jsonPart),
					jsonPart,
				)

				_, err := plugin.DecodeMessage([]byte(frame))
				Expect(err).To(HaveOccurred(),
					"DecodeMessage must reject frames with X-Body-Length but no X-Body-Field — "+
						"without X-Body-Field, the raw body bytes have no target key for injection")
				Expect(err.Error()).To(SatisfyAll(
					ContainSubstring("X-Body-Field"),
					SatisfyAny(
						ContainSubstring("missing"),
						ContainSubstring("X-Body-Length"),
					),
				), "error must indicate that X-Body-Field is missing when X-Body-Length is present")
			})
		})

		Describe("Send malformed split-body responses", func() {

			It("rejects stdout pollution in key: value format that mimics a header", func() {
				// "debug: loading plugin" looks like a header to naive SplitN parsing,
				// but must be rejected as stdout pollution because the first line
				// must start with "Content-Length:".
				pollution := "debug: loading plugin\r\n"
				reader := bufio.NewReader(strings.NewReader(pollution))
				bridge := plugin.NewBridgeWithReader(reader)

				_, err := bridge.Send(&plugin.Message{Type: "hook", Name: "test"})
				Expect(err).To(HaveOccurred(),
					"Send must reject lines that look like headers but aren't Content-Length — "+
						"without the first-line check, SplitN(line, \": \", 2) would parse "+
						"\"debug: loading plugin\" as headers[\"debug\"] = \"loading plugin\"")
				Expect(err.Error()).To(ContainSubstring("stdout"),
					"error must name stdout pollution as the likely cause, not report a "+
						"generic framing error — this helps plugin authors diagnose the issue")
				Expect(err.Error()).To(ContainSubstring("debug"),
					"error must include a snippet of the offending bytes for diagnosis")
			})

			It("returns error when X-Body-Length is negative in Send response", func() {
				jsonPart := `{"id":1,"result":{"status":"ok"}}`
				response := fmt.Sprintf(
					"Content-Length: %d\r\nX-Body-Length: -1\r\nX-Body-Field: html\r\n\r\n%s",
					len(jsonPart),
					jsonPart,
				)

				reader := bufio.NewReader(strings.NewReader(response))
				bridge := plugin.NewBridgeWithReader(reader)

				_, err := bridge.Send(&plugin.Message{Type: "hook", Name: "test"})
				Expect(err).To(HaveOccurred(),
					"Send must reject responses with negative X-Body-Length")
				Expect(err.Error()).To(ContainSubstring("X-Body-Length"),
					"error must identify X-Body-Length as the invalid header")
			})

			It("returns error when X-Body-Field is missing from Send response", func() {
				jsonPart := `{"id":1,"result":{"status":"ok"}}`
				rawPart := "some raw body"
				response := fmt.Sprintf(
					"Content-Length: %d\r\nX-Body-Length: %d\r\n\r\n%s%s",
					len(jsonPart), len(rawPart),
					jsonPart, rawPart,
				)

				reader := bufio.NewReader(strings.NewReader(response))
				bridge := plugin.NewBridgeWithReader(reader)

				_, err := bridge.Send(&plugin.Message{Type: "hook", Name: "test"})
				Expect(err).To(HaveOccurred(),
					"Send must reject responses with X-Body-Length but no X-Body-Field")
				Expect(err.Error()).To(SatisfyAll(
					ContainSubstring("X-Body-Field"),
					SatisfyAny(
						ContainSubstring("missing"),
						ContainSubstring("X-Body-Length"),
					),
				), "error must indicate that X-Body-Field is missing")
			})
		})
	})
})
