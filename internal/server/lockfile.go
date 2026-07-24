package server

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
)

// LockfileInfo holds metadata about a running alloy server process.
type LockfileInfo struct {
	PID       int    `json:"pid"`
	Port      int    `json:"port"`
	Mode      string `json:"mode"`
	StartedAt string `json:"startedAt"`
}

// LockfilePath returns the path to the server lockfile for a project.
func LockfilePath(projectRoot string) string {
	return filepath.Join(projectRoot, ".alloy", "server.lock")
}

// WriteLockfile creates the .alloy/ directory if needed and writes the lockfile.
// Overwrites any existing lockfile.
func WriteLockfile(projectRoot string, info LockfileInfo) error {
	lockPath := LockfilePath(projectRoot)
	dir := filepath.Dir(lockPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create lockfile directory: %w", err)
	}
	data, err := jsonCodec.Marshal(info)
	if err != nil {
		return fmt.Errorf("marshal lockfile: %w", err)
	}
	tmp := lockPath + ".tmp"
	if err := os.WriteFile(tmp, data, 0644); err != nil {
		return fmt.Errorf("write lockfile: %w", err)
	}
	if err := os.Rename(tmp, lockPath); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("write lockfile: %w", err)
	}
	return nil
}

// ReadLockfile reads the server lockfile. Returns (nil, nil) when the file
// does not exist. Returns a parse error for corrupt JSON.
func ReadLockfile(projectRoot string) (*LockfileInfo, error) {
	lockPath := LockfilePath(projectRoot)
	data, err := os.ReadFile(lockPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read lockfile: %w", err)
	}
	var info LockfileInfo
	if err := jsonCodec.Unmarshal(data, &info); err != nil {
		return nil, fmt.Errorf("parse lockfile: %w", err)
	}
	return &info, nil
}

// RemoveLockfile removes the server lockfile. Does not remove the .alloy/ directory.
func RemoveLockfile(projectRoot string) {
	os.Remove(LockfilePath(projectRoot))
}

// RemoveLockfileIfOwned removes the lockfile only if it belongs to the given PID.
// Returns true if the lockfile was removed. Used by shutdown paths to avoid
// deleting a lockfile that was overwritten by a newer process.
func RemoveLockfileIfOwned(projectRoot string, pid int) bool {
	info, _ := ReadLockfile(projectRoot)
	if info != nil && info.PID == pid {
		RemoveLockfile(projectRoot)
		return true
	}
	return false
}

// CheckAndWarnLockfile checks for an existing lockfile and returns warning
// messages if another alloy process is actively running. Returns nil if no
// lockfile exists, if the lockfile is stale (dead PID), or if the lockfile
// is corrupt. Stale and corrupt lockfiles are removed.
func CheckAndWarnLockfile(projectRoot string) []string {
	info, err := ReadLockfile(projectRoot)
	if err != nil {
		RemoveLockfile(projectRoot)
		return nil
	}
	if info == nil {
		return nil
	}

	if info.PID <= 0 || !isPIDAlive(info.PID) {
		RemoveLockfile(projectRoot)
		return nil
	}

	pidStr := strconv.Itoa(info.PID)
	return []string{
		fmt.Sprintf("another alloy process (PID %s, alloy %s on port %d, started %s) is watching this directory", pidStr, info.Mode, info.Port, info.StartedAt),
		"concurrent instances writing to _site/ will cause missing pages and 404s",
		fmt.Sprintf("kill the other process with: kill %s", pidStr),
	}
}
