//go:build windows

package plugin

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
)

var pidFileMu sync.Mutex

func setProcGroup(cmd *exec.Cmd) {}

func killProcGroup(pid int) error {
	p, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	return p.Kill()
}

func withPIDFile(projectRoot string, create bool, fn func(lines []string) []string) {
	if projectRoot == "" {
		return
	}
	pidFileMu.Lock()
	defer pidFileMu.Unlock()

	path := pidFilePath(projectRoot)

	if create {
		alloyDir := filepath.Dir(path)
		if err := os.MkdirAll(alloyDir, 0755); err != nil {
			return
		}
	}

	var lines []string
	if data, err := os.ReadFile(path); err == nil {
		for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
			if strings.TrimSpace(line) != "" {
				lines = append(lines, strings.TrimSpace(line))
			}
		}
	}

	result := fn(lines)

	if len(result) > 0 {
		os.WriteFile(path, []byte(strings.Join(result, "\n")+"\n"), 0644)
	} else {
		os.WriteFile(path, []byte{}, 0644)
	}
}

func addPIDToFile(projectRoot string, pid int) {
	withPIDFile(projectRoot, true, func(lines []string) []string {
		return append(lines, fmt.Sprintf("%d", pid))
	})
}

func removePIDFromFile(projectRoot string, pid int) {
	pidStr := fmt.Sprintf("%d", pid)
	withPIDFile(projectRoot, false, func(lines []string) []string {
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
	withPIDFile(projectRoot, false, func(lines []string) []string {
		for _, line := range lines {
			pid, err := strconv.Atoi(line)
			if err != nil || pid <= 0 {
				continue
			}
			p, err := os.FindProcess(pid)
			if err == nil {
				p.Kill()
			}
		}
		return nil
	})
}
