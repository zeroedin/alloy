//go:build !windows

package plugin

import (
	"fmt"
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

func pidFilePath(projectRoot string) string {
	return filepath.Join(projectRoot, ".alloy", "workers.pid")
}

func addPIDToFile(projectRoot string, pid int) {
	if projectRoot == "" {
		return
	}
	path := pidFilePath(projectRoot)
	alloyDir := filepath.Dir(path)
	if err := os.MkdirAll(alloyDir, 0755); err != nil {
		return
	}

	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0644)
	if err != nil {
		return
	}
	defer f.Close()

	syscall.Flock(int(f.Fd()), syscall.LOCK_EX)
	defer syscall.Flock(int(f.Fd()), syscall.LOCK_UN)

	data, _ := os.ReadFile(path)
	line := fmt.Sprintf("%d\n", pid)
	content := string(data) + line
	f.Truncate(0)
	f.Seek(0, 0)
	f.WriteString(content)
}

func removePIDFromFile(projectRoot string, pid int) {
	if projectRoot == "" {
		return
	}
	path := pidFilePath(projectRoot)

	f, err := os.OpenFile(path, os.O_RDWR, 0644)
	if err != nil {
		return
	}
	defer f.Close()

	syscall.Flock(int(f.Fd()), syscall.LOCK_EX)
	defer syscall.Flock(int(f.Fd()), syscall.LOCK_UN)

	data, err := os.ReadFile(path)
	if err != nil {
		return
	}

	pidStr := fmt.Sprintf("%d", pid)
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	var kept []string
	for _, line := range lines {
		if strings.TrimSpace(line) != pidStr {
			kept = append(kept, line)
		}
	}

	f.Truncate(0)
	f.Seek(0, 0)
	if len(kept) > 0 {
		f.WriteString(strings.Join(kept, "\n") + "\n")
	}
}

func cleanStalePIDs(projectRoot string) {
	if projectRoot == "" {
		return
	}
	path := pidFilePath(projectRoot)

	f, err := os.OpenFile(path, os.O_RDWR, 0644)
	if err != nil {
		return
	}
	defer f.Close()

	syscall.Flock(int(f.Fd()), syscall.LOCK_EX)
	defer syscall.Flock(int(f.Fd()), syscall.LOCK_UN)

	data, err := os.ReadFile(path)
	if err != nil {
		return
	}

	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	var kept []string
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		pid, err := strconv.Atoi(line)
		if err != nil {
			continue
		}
		if syscall.Kill(pid, 0) == nil {
			syscall.Kill(pid, syscall.SIGTERM)
		}
	}

	f.Truncate(0)
	f.Seek(0, 0)
	if len(kept) > 0 {
		f.WriteString(strings.Join(kept, "\n") + "\n")
	}
}
