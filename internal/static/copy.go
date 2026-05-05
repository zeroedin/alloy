package static

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	"github.com/zeroedin/alloy/internal/config"
	"github.com/zeroedin/alloy/internal/fileutil"
)

// CopyStatic copies all files from staticDir to outputDir.
// If staticDir does not exist or is empty, it returns nil (no error).
func CopyStatic(staticDir, outputDir string) error {
	info, err := os.Stat(staticDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("static path %q is not a directory", staticDir)
	}

	return copyDirConcurrent(staticDir, outputDir)
}

// CopyPassthrough copies files according to passthrough mappings.
// From paths are resolved relative to projectRoot per §1h.
// Returns an error if a From path does not exist.
func CopyPassthrough(mappings []config.PassthroughMapping, projectRoot, outputDir string) error {
	for _, m := range mappings {
		fromPath := m.From
		if !filepath.IsAbs(fromPath) {
			fromPath = filepath.Join(projectRoot, fromPath)
		}

		info, err := os.Stat(fromPath)
		if err != nil {
			return fmt.Errorf("passthrough source %q does not exist: %w", m.From, err)
		}

		toPath := filepath.Join(outputDir, m.To)

		if info.IsDir() {
			if err := copyDirConcurrent(fromPath, toPath); err != nil {
				return err
			}
		} else {
			if err := fileutil.CopyFile(fromPath, toPath); err != nil {
				return err
			}
		}
	}
	return nil
}

// CopyPassthroughWithValidation copies files according to passthrough mappings,
// silently ignoring any mapping where the "from" path resolves to a managed directory
// (content, layouts, assets, static, data).
func CopyPassthroughWithValidation(mappings []config.PassthroughMapping, projectRoot, outputDir string, managedDirs []string) error {
	var filtered []config.PassthroughMapping
	for _, m := range mappings {
		fromAbs := m.From
		if !filepath.IsAbs(fromAbs) {
			fromAbs = filepath.Join(projectRoot, fromAbs)
		}
		fromAbs = filepath.Clean(fromAbs)

		skip := false
		for _, d := range managedDirs {
			managedAbs := d
			if !filepath.IsAbs(managedAbs) {
				managedAbs = filepath.Join(projectRoot, managedAbs)
			}
			managedAbs = filepath.Clean(managedAbs)

			if fromAbs == managedAbs || strings.HasPrefix(fromAbs, managedAbs+string(filepath.Separator)) {
				skip = true
				break
			}
		}
		if !skip {
			filtered = append(filtered, m)
		}
	}

	return CopyPassthrough(filtered, projectRoot, outputDir)
}

type copyJob struct {
	src string
	dst string
}

// copyDirConcurrent walks the source tree, creates directories synchronously,
// and copies files concurrently using a bounded worker pool.
func copyDirConcurrent(src, dst string) error {
	var jobs []copyJob

	if err := filepath.Walk(src, func(path string, fi os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if fi.IsDir() {
			return os.MkdirAll(target, 0755)
		}
		jobs = append(jobs, copyJob{src: path, dst: target})
		return nil
	}); err != nil {
		return err
	}

	if len(jobs) == 0 {
		return nil
	}

	workers := runtime.NumCPU()
	if workers > len(jobs) {
		workers = len(jobs)
	}

	var wg sync.WaitGroup
	sem := make(chan struct{}, workers)
	var mu sync.Mutex
	var firstErr error

	for _, job := range jobs {
		mu.Lock()
		if firstErr != nil {
			mu.Unlock()
			break
		}
		mu.Unlock()

		sem <- struct{}{}
		wg.Add(1)
		go func(j copyJob) {
			defer wg.Done()
			defer func() { <-sem }()
			if err := fileutil.CopyFile(j.src, j.dst); err != nil {
				mu.Lock()
				if firstErr == nil {
					firstErr = err
				}
				mu.Unlock()
			}
		}(job)
	}

	wg.Wait()
	return firstErr
}
