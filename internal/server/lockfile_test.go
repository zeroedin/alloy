package server_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"github.com/bytedance/sonic"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/zeroedin/alloy/internal/server"
)

// deadPID returns a PID that is guaranteed to be not running.
// It spawns a short-lived subprocess and waits for it to exit.
func deadPID() int {
	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.Command("cmd", "/c", "echo", "done")
	} else {
		cmd = exec.Command("true")
	}
	err := cmd.Start()
	Expect(err).NotTo(HaveOccurred())
	err = cmd.Wait()
	Expect(err).NotTo(HaveOccurred())
	return cmd.Process.Pid
}

var _ = Describe("Server lockfile (issue #1094)", func() {

	// ── LockfilePath ──────────────────────────────────────────────────

	Describe("LockfilePath", func() {
		It("returns .alloy/server.lock under the project root", func() {
			path := server.LockfilePath("/my/project")
			Expect(path).To(Equal(filepath.Join("/my/project", ".alloy", "server.lock")),
				"lockfile must be at .alloy/server.lock inside the project directory")
		})

		It("handles empty project root by using current directory", func() {
			path := server.LockfilePath("")
			Expect(path).To(Equal(filepath.Join(".alloy", "server.lock")),
				"empty project root should produce a relative .alloy/server.lock path")
		})
	})

	// ── WriteLockfile ─────────────────────────────────────────────────

	Describe("WriteLockfile", func() {
		var tmpDir string

		BeforeEach(func() {
			var err error
			tmpDir, err = os.MkdirTemp("", "alloy-lockfile-test-*")
			Expect(err).NotTo(HaveOccurred())
			DeferCleanup(func() { os.RemoveAll(tmpDir) })
		})

		It("creates .alloy/ directory and server.lock with correct JSON", func() {
			info := server.LockfileInfo{
				PID:       12345,
				Port:      3003,
				Mode:      "serve",
				StartedAt: "2026-07-14T13:00:00-04:00",
			}
			err := server.WriteLockfile(tmpDir, info)
			Expect(err).NotTo(HaveOccurred())

			lockPath := filepath.Join(tmpDir, ".alloy", "server.lock")
			Expect(lockPath).To(BeAnExistingFile(),
				"WriteLockfile must create .alloy/server.lock")

			data, err := os.ReadFile(lockPath)
			Expect(err).NotTo(HaveOccurred())

			var parsed server.LockfileInfo
			err = sonic.Unmarshal(data, &parsed)
			Expect(err).NotTo(HaveOccurred(),
				"lockfile must contain valid JSON")
			Expect(parsed.PID).To(Equal(12345))
			Expect(parsed.Port).To(Equal(3003))
			Expect(parsed.Mode).To(Equal("serve"))
			Expect(parsed.StartedAt).To(Equal("2026-07-14T13:00:00-04:00"))
		})

		It("creates .alloy/ directory if it does not exist", func() {
			alloyDir := filepath.Join(tmpDir, ".alloy")
			Expect(alloyDir).NotTo(BeADirectory(),
				"precondition: .alloy/ must not exist before WriteLockfile")

			info := server.LockfileInfo{
				PID:  1,
				Port: 3000,
				Mode: "dev",
			}
			err := server.WriteLockfile(tmpDir, info)
			Expect(err).NotTo(HaveOccurred())
			Expect(alloyDir).To(BeADirectory(),
				"WriteLockfile must create .alloy/ directory")
		})

		It("overwrites an existing lockfile", func() {
			info1 := server.LockfileInfo{
				PID:       111,
				Port:      3000,
				Mode:      "dev",
				StartedAt: "2026-07-14T12:00:00Z",
			}
			err := server.WriteLockfile(tmpDir, info1)
			Expect(err).NotTo(HaveOccurred())

			info2 := server.LockfileInfo{
				PID:       222,
				Port:      3001,
				Mode:      "serve",
				StartedAt: "2026-07-14T13:00:00Z",
			}
			err = server.WriteLockfile(tmpDir, info2)
			Expect(err).NotTo(HaveOccurred())

			read, err := server.ReadLockfile(tmpDir)
			Expect(err).NotTo(HaveOccurred())
			Expect(read).NotTo(BeNil())
			Expect(read.PID).To(Equal(222),
				"WriteLockfile must overwrite the previous lockfile — "+
					"the new process replaces the old one")
			Expect(read.Port).To(Equal(3001))
			Expect(read.Mode).To(Equal("serve"))
		})

		It("returns error when .alloy path is blocked by a regular file (issue #1124)", func() {
			// If a regular file exists at .alloy (not a directory), MkdirAll fails
			alloyPath := filepath.Join(tmpDir, ".alloy")
			Expect(os.WriteFile(alloyPath, []byte("not a directory"), 0644)).To(Succeed())

			info := server.LockfileInfo{PID: 1, Port: 3000, Mode: "dev"}
			err := server.WriteLockfile(tmpDir, info)
			Expect(err).To(HaveOccurred(),
				"WriteLockfile must return an error when .alloy exists as a regular file")
			Expect(err.Error()).To(SatisfyAny(
				ContainSubstring("lockfile"),
				ContainSubstring("directory"),
			), "error message must reference the lockfile or directory operation")
		})

		It("writes the mode as dev for dev servers", func() {
			info := server.LockfileInfo{
				PID:  os.Getpid(),
				Port: 3000,
				Mode: "dev",
			}
			err := server.WriteLockfile(tmpDir, info)
			Expect(err).NotTo(HaveOccurred())

			read, err := server.ReadLockfile(tmpDir)
			Expect(err).NotTo(HaveOccurred())
			Expect(read.Mode).To(Equal("dev"),
				"mode must distinguish dev from serve")
		})
	})

	// ── ReadLockfile ──────────────────────────────────────────────────

	Describe("ReadLockfile", func() {
		var tmpDir string

		BeforeEach(func() {
			var err error
			tmpDir, err = os.MkdirTemp("", "alloy-lockfile-test-*")
			Expect(err).NotTo(HaveOccurred())
			DeferCleanup(func() { os.RemoveAll(tmpDir) })
		})

		It("returns nil,nil when .alloy/ exists but server.lock does not", func() {
			alloyDir := filepath.Join(tmpDir, ".alloy")
			Expect(os.MkdirAll(alloyDir, 0755)).To(Succeed())

			info, err := server.ReadLockfile(tmpDir)
			Expect(err).NotTo(HaveOccurred(),
				"missing lockfile must not be an error when .alloy/ directory exists")
			Expect(info).To(BeNil(),
				"missing lockfile must return nil info")
		})

		It("parses a valid lockfile", func() {
			alloyDir := filepath.Join(tmpDir, ".alloy")
			Expect(os.MkdirAll(alloyDir, 0755)).To(Succeed())

			lockJSON := `{"pid":4659,"port":3003,"mode":"serve","startedAt":"2026-07-14T13:00:00-04:00"}`
			lockPath := filepath.Join(alloyDir, "server.lock")
			Expect(os.WriteFile(lockPath, []byte(lockJSON), 0644)).To(Succeed())

			info, err := server.ReadLockfile(tmpDir)
			Expect(err).NotTo(HaveOccurred())
			Expect(info).NotTo(BeNil())
			Expect(info.PID).To(Equal(4659))
			Expect(info.Port).To(Equal(3003))
			Expect(info.Mode).To(Equal("serve"))
			Expect(info.StartedAt).To(Equal("2026-07-14T13:00:00-04:00"))
		})

		It("returns error for corrupt JSON", func() {
			alloyDir := filepath.Join(tmpDir, ".alloy")
			Expect(os.MkdirAll(alloyDir, 0755)).To(Succeed())

			lockPath := filepath.Join(alloyDir, "server.lock")
			Expect(os.WriteFile(lockPath, []byte("{corrupt"), 0644)).To(Succeed())

			_, err := server.ReadLockfile(tmpDir)
			Expect(err).To(HaveOccurred(),
				"corrupt JSON must return an error — callers decide whether to treat as stale")
		})

		It("returns error when .alloy path is blocked by a regular file (issue #1124)", func() {
			// If a regular file exists at .alloy (not a directory),
			// os.ReadFile(".alloy/server.lock") fails with ENOTDIR,
			// which is not an IsNotExist error — must surface as a read error.
			alloyPath := filepath.Join(tmpDir, ".alloy")
			Expect(os.WriteFile(alloyPath, []byte("not a directory"), 0644)).To(Succeed())

			info, err := server.ReadLockfile(tmpDir)
			Expect(err).To(HaveOccurred(),
				"ReadLockfile must return an error when .alloy is a regular file, not a directory")
			Expect(info).To(BeNil())
			Expect(err.Error()).To(ContainSubstring("lockfile"),
				"error message must reference the lockfile operation")
		})

		It("returns nil,nil when .alloy/ directory does not exist", func() {
			info, err := server.ReadLockfile(tmpDir)
			Expect(err).NotTo(HaveOccurred())
			Expect(info).To(BeNil())
		})
	})

	// ── RemoveLockfile ────────────────────────────────────────────────

	Describe("RemoveLockfile", func() {
		var tmpDir string

		BeforeEach(func() {
			var err error
			tmpDir, err = os.MkdirTemp("", "alloy-lockfile-test-*")
			Expect(err).NotTo(HaveOccurred())
			DeferCleanup(func() { os.RemoveAll(tmpDir) })
		})

		It("removes an existing lockfile", func() {
			info := server.LockfileInfo{PID: 1, Port: 3000, Mode: "dev"}
			Expect(server.WriteLockfile(tmpDir, info)).To(Succeed())

			lockPath := filepath.Join(tmpDir, ".alloy", "server.lock")
			Expect(lockPath).To(BeAnExistingFile(),
				"precondition: lockfile must exist")

			server.RemoveLockfile(tmpDir)

			Expect(lockPath).NotTo(BeAnExistingFile(),
				"RemoveLockfile must delete .alloy/server.lock")
		})

		It("does not error when no lockfile exists", func() {
			// RemoveLockfile should be safe to call even if there's nothing to clean up.
			// This is important for the signal handler path — crashes may have
			// already cleaned up or the lockfile may never have been written.
			Expect(func() {
				server.RemoveLockfile(tmpDir)
			}).NotTo(Panic(),
				"RemoveLockfile must not panic when no lockfile exists")
		})

		It("does not remove the .alloy/ directory itself", func() {
			alloyDir := filepath.Join(tmpDir, ".alloy")
			Expect(os.MkdirAll(alloyDir, 0755)).To(Succeed())

			info := server.LockfileInfo{PID: 1, Port: 3000, Mode: "dev"}
			Expect(server.WriteLockfile(tmpDir, info)).To(Succeed())

			server.RemoveLockfile(tmpDir)

			Expect(alloyDir).To(BeADirectory(),
				"RemoveLockfile must remove only server.lock, not the .alloy/ directory — "+
					"other files (fetch cache, WASM cache, profiles) live there")
		})
	})

	// ── CheckAndWarnLockfile ──────────────────────────────────────────

	Describe("CheckAndWarnLockfile", func() {
		var tmpDir string

		BeforeEach(func() {
			var err error
			tmpDir, err = os.MkdirTemp("", "alloy-lockfile-test-*")
			Expect(err).NotTo(HaveOccurred())
			DeferCleanup(func() { os.RemoveAll(tmpDir) })
		})

		It("returns nil when no lockfile exists", func() {
			warnings := server.CheckAndWarnLockfile(tmpDir)
			Expect(warnings).To(BeNil(),
				"no lockfile means no warnings — startup proceeds normally")
		})

		It("removes stale lockfile (dead PID) and returns nil", func() {
			pid := deadPID()

			info := server.LockfileInfo{
				PID:       pid,
				Port:      3003,
				Mode:      "serve",
				StartedAt: "2026-07-14T13:00:00-04:00",
			}
			Expect(server.WriteLockfile(tmpDir, info)).To(Succeed())

			warnings := server.CheckAndWarnLockfile(tmpDir)
			Expect(warnings).To(BeNil(),
				"dead PID means the lockfile is stale — no warnings needed")

			lockPath := filepath.Join(tmpDir, ".alloy", "server.lock")
			Expect(lockPath).NotTo(BeAnExistingFile(),
				"stale lockfile must be removed so the new process can proceed")
		})

		It("returns warnings for an active lockfile (live PID)", func() {
			// Use the current process PID — it's always alive during the test
			livePID := os.Getpid()

			info := server.LockfileInfo{
				PID:       livePID,
				Port:      3003,
				Mode:      "serve",
				StartedAt: "2026-07-14T13:00:00-04:00",
			}
			Expect(server.WriteLockfile(tmpDir, info)).To(Succeed())

			warnings := server.CheckAndWarnLockfile(tmpDir)
			Expect(warnings).NotTo(BeNil(),
				"live PID means another instance is running — must return warnings")
			Expect(len(warnings)).To(BeNumerically(">=", 3),
				"must include: process info, impact description, and kill command")

			// Verify warning content references the conflicting process
			joined := strings.Join(warnings, "\n")
			Expect(joined).To(ContainSubstring(strconv.Itoa(livePID)),
				"warnings must include the PID of the conflicting process")
			Expect(joined).To(ContainSubstring("3003"),
				"warnings must include the port of the conflicting process")
			Expect(joined).To(ContainSubstring("serve"),
				"warnings must include the mode (dev/serve) of the conflicting process")
			Expect(joined).To(ContainSubstring("kill"),
				"warnings must include the kill command so the user can stop the other process")
		})

		It("warning messages follow the specified format", func() {
			livePID := os.Getpid()

			info := server.LockfileInfo{
				PID:       livePID,
				Port:      3003,
				Mode:      "serve",
				StartedAt: "2026-07-14T13:00:00-04:00",
			}
			Expect(server.WriteLockfile(tmpDir, info)).To(Succeed())

			warnings := server.CheckAndWarnLockfile(tmpDir)
			Expect(warnings).NotTo(BeEmpty())

			// First warning identifies the conflicting process
			Expect(warnings[0]).To(ContainSubstring("another alloy process"),
				"first warning must identify the conflicting process")
			Expect(warnings[0]).To(ContainSubstring(strconv.Itoa(livePID)),
				"first warning must include the PID")

			// Second warning explains the impact
			found := false
			for _, w := range warnings {
				if strings.Contains(w, "concurrent") || strings.Contains(w, "missing pages") || strings.Contains(w, "404") {
					found = true
					break
				}
			}
			Expect(found).To(BeTrue(),
				"warnings must explain the impact: concurrent instances cause missing pages/404s")

			// Last warning gives the kill command
			lastWarning := warnings[len(warnings)-1]
			Expect(lastWarning).To(ContainSubstring("kill "+strconv.Itoa(livePID)),
				"last warning must include the exact kill command: kill <PID>")
		})

		It("treats corrupt lockfile as stale and removes it", func() {
			alloyDir := filepath.Join(tmpDir, ".alloy")
			Expect(os.MkdirAll(alloyDir, 0755)).To(Succeed())

			lockPath := filepath.Join(alloyDir, "server.lock")
			Expect(os.WriteFile(lockPath, []byte("{corrupt"), 0644)).To(Succeed())

			warnings := server.CheckAndWarnLockfile(tmpDir)
			Expect(warnings).To(BeNil(),
				"corrupt lockfile should be treated as stale — "+
					"a crashed process may have left a half-written file")

			Expect(lockPath).NotTo(BeAnExistingFile(),
				"corrupt lockfile must be removed")
		})

		It("does not block startup even when another instance is active", func() {
			// The issue specifies "print a warning and continue (don't block startup)"
			// CheckAndWarnLockfile must return warnings, not an error — the caller
			// prints warnings and proceeds with startup.
			livePID := os.Getpid()

			info := server.LockfileInfo{
				PID:       livePID,
				Port:      3003,
				Mode:      "serve",
				StartedAt: "2026-07-14T13:00:00-04:00",
			}
			Expect(server.WriteLockfile(tmpDir, info)).To(Succeed())

			warnings := server.CheckAndWarnLockfile(tmpDir)
			// The function returns warnings ([]string), not an error.
			// Callers print these and continue — startup is never blocked.
			Expect(warnings).NotTo(BeEmpty(),
				"must return warnings, not block startup")
		})

		It("treats PID 0 as stale and removes lockfile (issue #1124)", func() {
			info := server.LockfileInfo{
				PID:       0,
				Port:      3000,
				Mode:      "dev",
				StartedAt: "2026-07-14T12:00:00Z",
			}
			Expect(server.WriteLockfile(tmpDir, info)).To(Succeed())

			lockPath := filepath.Join(tmpDir, ".alloy", "server.lock")
			Expect(lockPath).To(BeAnExistingFile(),
				"precondition: lockfile with PID 0 must exist")

			warnings := server.CheckAndWarnLockfile(tmpDir)
			Expect(warnings).To(BeNil(),
				"PID 0 is invalid — no real process has PID 0, must treat as stale")

			Expect(lockPath).NotTo(BeAnExistingFile(),
				"lockfile with PID 0 must be removed as stale")
		})

		It("treats negative PID as stale and removes lockfile (issue #1124)", func() {
			info := server.LockfileInfo{
				PID:       -1,
				Port:      3000,
				Mode:      "dev",
				StartedAt: "2026-07-14T12:00:00Z",
			}
			Expect(server.WriteLockfile(tmpDir, info)).To(Succeed())

			lockPath := filepath.Join(tmpDir, ".alloy", "server.lock")
			Expect(lockPath).To(BeAnExistingFile(),
				"precondition: lockfile with negative PID must exist")

			warnings := server.CheckAndWarnLockfile(tmpDir)
			Expect(warnings).To(BeNil(),
				"negative PID is invalid — must treat as stale and return no warnings")

			Expect(lockPath).NotTo(BeAnExistingFile(),
				"lockfile with negative PID must be removed as stale")
		})

		It("preserves the lockfile when the PID is alive", func() {
			livePID := os.Getpid()

			info := server.LockfileInfo{
				PID:       livePID,
				Port:      3003,
				Mode:      "serve",
				StartedAt: "2026-07-14T13:00:00-04:00",
			}
			Expect(server.WriteLockfile(tmpDir, info)).To(Succeed())

			_ = server.CheckAndWarnLockfile(tmpDir)

			lockPath := filepath.Join(tmpDir, ".alloy", "server.lock")
			Expect(lockPath).To(BeAnExistingFile(),
				"CheckAndWarnLockfile must NOT remove a lockfile with a live PID — "+
					"the other process is still running and owns the lockfile")
		})
	})

	// ── PID ownership lifecycle ──────────────────────────────────────

	Describe("PID ownership lifecycle (issue #1124)", func() {
		var tmpDir string

		BeforeEach(func() {
			var err error
			tmpDir, err = os.MkdirTemp("", "alloy-lockfile-test-*")
			Expect(err).NotTo(HaveOccurred())
			DeferCleanup(func() { os.RemoveAll(tmpDir) })
		})

		It("does not delete lockfile owned by another process (cascade scenario)", func() {
			// Scenario: Process A starts and writes lockfile with its PID.
			// Process B starts later and overwrites the lockfile with its PID.
			// Process A shuts down and checks PID ownership before removing.
			// The lockfile must survive because it belongs to Process B.

			// Step 1: "Process A" (current process) writes lockfile
			infoA := server.LockfileInfo{
				PID:       os.Getpid(),
				Port:      3000,
				Mode:      "dev",
				StartedAt: "2026-07-14T12:00:00Z",
			}
			Expect(server.WriteLockfile(tmpDir, infoA)).To(Succeed())

			// Step 2: "Process B" overwrites lockfile with a different PID
			otherPID := os.Getpid() + 99999
			infoB := server.LockfileInfo{
				PID:       otherPID,
				Port:      3001,
				Mode:      "serve",
				StartedAt: "2026-07-14T13:00:00Z",
			}
			Expect(server.WriteLockfile(tmpDir, infoB)).To(Succeed())

			// Step 3: "Process A" shutdown — read lockfile and check PID ownership.
			// This mirrors the guard in cmd/dev.go and cmd/serve.go:
			//   if info, _ := server.ReadLockfile(...); info != nil && info.PID == os.Getpid() {
			//       server.RemoveLockfile(...)
			//   }
			info, err := server.ReadLockfile(tmpDir)
			Expect(err).NotTo(HaveOccurred())
			Expect(info).NotTo(BeNil())
			Expect(info.PID).To(Equal(otherPID),
				"lockfile must contain Process B's PID after overwrite")
			Expect(info.PID).NotTo(Equal(os.Getpid()),
				"PID guard (info.PID == os.Getpid()) must evaluate to false — "+
					"Process A must NOT remove a lockfile it no longer owns")

			// Simulate the call-site guard: only remove if PID matches current process
			if info != nil && info.PID == os.Getpid() {
				server.RemoveLockfile(tmpDir)
			}

			// Lockfile must still exist — Process A's guard prevented deletion
			lockPath := filepath.Join(tmpDir, ".alloy", "server.lock")
			Expect(lockPath).To(BeAnExistingFile(),
				"lockfile must survive — Process B owns it, the PID guard prevented deletion")

			// Verify the surviving lockfile still has Process B's data
			surviving, err := server.ReadLockfile(tmpDir)
			Expect(err).NotTo(HaveOccurred())
			Expect(surviving.PID).To(Equal(otherPID))
			Expect(surviving.Port).To(Equal(3001))
			Expect(surviving.Mode).To(Equal("serve"))
		})
	})

	// ── LockfileInfo struct ───────────────────────────────────────────

	Describe("LockfileInfo", func() {
		It("round-trips through JSON serialization", func() {
			info := server.LockfileInfo{
				PID:       4659,
				Port:      3003,
				Mode:      "serve",
				StartedAt: "2026-07-14T13:00:00-04:00",
			}

			data, err := sonic.Marshal(info)
			Expect(err).NotTo(HaveOccurred())

			var parsed server.LockfileInfo
			err = sonic.Unmarshal(data, &parsed)
			Expect(err).NotTo(HaveOccurred())
			Expect(parsed).To(Equal(info))
		})

		It("serializes to the documented JSON field names", func() {
			info := server.LockfileInfo{
				PID:       4659,
				Port:      3003,
				Mode:      "serve",
				StartedAt: "2026-07-14T13:00:00-04:00",
			}

			data, err := sonic.Marshal(info)
			Expect(err).NotTo(HaveOccurred())

			jsonStr := string(data)
			Expect(jsonStr).To(ContainSubstring(`"pid"`),
				"JSON field must be named 'pid' (lowercase)")
			Expect(jsonStr).To(ContainSubstring(`"port"`),
				"JSON field must be named 'port' (lowercase)")
			Expect(jsonStr).To(ContainSubstring(`"mode"`),
				"JSON field must be named 'mode' (lowercase)")
			Expect(jsonStr).To(ContainSubstring(`"startedAt"`),
				"JSON field must be named 'startedAt' (camelCase)")
		})
	})
})
