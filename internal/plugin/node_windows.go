//go:build windows

package plugin

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func setProcGroup(cmd *exec.Cmd) {}

func killProcGroup(pid int) error { return nil }

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
	data, _ := os.ReadFile(path)
	line := fmt.Sprintf("%d\n", pid)
	os.WriteFile(path, []byte(string(data)+line), 0644)
}

func removePIDFromFile(projectRoot string, pid int) {
	if projectRoot == "" {
		return
	}
	path := pidFilePath(projectRoot)
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
	if len(kept) > 0 {
		os.WriteFile(path, []byte(strings.Join(kept, "\n")+"\n"), 0644)
	} else {
		os.WriteFile(path, []byte{}, 0644)
	}
}

func cleanStalePIDs(projectRoot string) {
	if projectRoot == "" {
		return
	}
	path := pidFilePath(projectRoot)
	os.WriteFile(path, []byte{}, 0644)
}
