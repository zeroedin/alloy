//go:build !windows

package plugin

import (
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
)

func setProcGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

func killProcGroup(pid int) error {
	pgid, err := syscall.Getpgid(pid)
	if err != nil {
		return err
	}
	return syscall.Kill(-pgid, syscall.SIGTERM)
}

func withPIDFile(projectRoot string, create bool, fn func(f *os.File, lines []string) []string) {
	if projectRoot == "" {
		return
	}
	path := pidFilePath(projectRoot)

	if create {
		alloyDir := filepath.Dir(path)
		if err := os.MkdirAll(alloyDir, 0755); err != nil {
			return
		}
	}

	flags := os.O_RDWR
	if create {
		flags |= os.O_CREATE
	}
	f, err := os.OpenFile(path, flags, 0644)
	if err != nil {
		return
	}
	defer f.Close()

	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		log.Printf("warning: PID file lock: %v", err)
		return
	}
	defer syscall.Flock(int(f.Fd()), syscall.LOCK_UN)

	data, _ := io.ReadAll(f)
	var lines []string
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		if strings.TrimSpace(line) != "" {
			lines = append(lines, strings.TrimSpace(line))
		}
	}

	result := fn(f, lines)

	if err := f.Truncate(0); err != nil {
		log.Printf("warning: PID file truncate: %v", err)
		return
	}
	f.Seek(0, 0)
	if len(result) > 0 {
		f.WriteString(strings.Join(result, "\n") + "\n")
	}
}

func addPIDToFile(projectRoot string, pid int) {
	withPIDFile(projectRoot, true, func(f *os.File, lines []string) []string {
		return append(lines, fmt.Sprintf("%d", pid))
	})
}

func removePIDFromFile(projectRoot string, pid int) {
	pidStr := fmt.Sprintf("%d", pid)
	withPIDFile(projectRoot, false, func(f *os.File, lines []string) []string {
		var kept []string
		for _, line := range lines {
			if line != pidStr {
				kept = append(kept, line)
			}
		}
		return kept
	})
}

func cleanStalePIDs(projectRoot string) {
	withPIDFile(projectRoot, false, func(f *os.File, lines []string) []string {
		var kept []string
		for _, line := range lines {
			pid, err := strconv.Atoi(line)
			if err != nil || pid <= 0 {
				continue
			}
			if syscall.Kill(pid, 0) == nil {
				syscall.Kill(pid, syscall.SIGTERM)
			}
		}
		return kept
	})
}
