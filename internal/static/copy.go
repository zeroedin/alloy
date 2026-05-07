package static

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	"github.com/bmatcuk/doublestar/v4"
	"github.com/zeroedin/alloy/internal/config"
	"github.com/zeroedin/alloy/internal/fileutil"
)

// ContainsGlobChars reports whether path contains glob metacharacters.
func ContainsGlobChars(path string) bool {
	return strings.ContainsAny(path, "*?[{")
}

// GlobRoot returns the longest directory prefix before any glob metacharacter.
func GlobRoot(pattern string) string {
	slashed := filepath.ToSlash(pattern)
	for i, c := range slashed {
		if c == '*' || c == '?' || c == '[' || c == '{' {
			dir := slashed[:i]
			if last := strings.LastIndex(dir, "/"); last >= 0 {
				return filepath.FromSlash(dir[:last])
			}
			return "."
		}
	}
	return filepath.FromSlash(slashed)
}

func normalizeExcludePattern(pattern string) string {
	if !strings.Contains(pattern, "/") {
		return "**/" + pattern
	}
	if strings.HasSuffix(pattern, "/") {
		return pattern + "**"
	}
	return pattern
}

// NormalizeExcludePatterns pre-normalizes exclude patterns for repeated matching.
func NormalizeExcludePatterns(patterns []string) []string {
	normalized := make([]string, len(patterns))
	for i, p := range patterns {
		normalized[i] = normalizeExcludePattern(p)
	}
	return normalized
}

// MatchExclude reports whether relPath matches any of the exclude patterns.
func MatchExclude(patterns []string, relPath string) (bool, error) {
	return matchExcludeNormalized(NormalizeExcludePatterns(patterns), relPath)
}

func matchExcludeNormalized(normalized []string, relPath string) (bool, error) {
	slashed := filepath.ToSlash(relPath)
	for _, norm := range normalized {
		matched, err := doublestar.Match(norm, slashed)
		if err != nil {
			return false, fmt.Errorf("exclude pattern %q: %w", norm, err)
		}
		if matched {
			return true, nil
		}
	}
	return false, nil
}

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

	return copyDirConcurrent(staticDir, outputDir, nil)
}

// CopyPassthrough copies files according to passthrough mappings.
// From paths are resolved relative to projectRoot per §1h.
// Returns an error if a From path does not exist.
func CopyPassthrough(mappings []config.PassthroughMapping, projectRoot, outputDir string) error {
	for _, m := range mappings {
		if ContainsGlobChars(m.From) {
			if err := copyGlob(m, projectRoot, outputDir); err != nil {
				return err
			}
			continue
		}

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
			if err := copyDirConcurrent(fromPath, toPath, m.Exclude); err != nil {
				return err
			}
		} else {
			if len(m.Exclude) > 0 {
				excluded, err := MatchExclude(m.Exclude, filepath.Base(fromPath))
				if err != nil {
					return err
				}
				if excluded {
					continue
				}
			}
			if err := fileutil.CopyFile(fromPath, toPath); err != nil {
				return err
			}
		}
	}
	return nil
}

func copyGlob(m config.PassthroughMapping, projectRoot, outputDir string) error {
	absPattern := m.From
	if !filepath.IsAbs(absPattern) {
		absPattern = filepath.Join(projectRoot, absPattern)
	}

	root := GlobRoot(absPattern)
	relPattern, err := filepath.Rel(root, absPattern)
	if err != nil {
		return fmt.Errorf("passthrough glob %q: %w", m.From, err)
	}
	relPattern = filepath.ToSlash(relPattern)

	matches, err := doublestar.Glob(os.DirFS(root), relPattern, doublestar.WithFilesOnly())
	if err != nil {
		return fmt.Errorf("passthrough glob %q: %w", m.From, err)
	}

	normalized := NormalizeExcludePatterns(m.Exclude)
	for _, match := range matches {
		if len(normalized) > 0 {
			excluded, err := matchExcludeNormalized(normalized, match)
			if err != nil {
				return err
			}
			if excluded {
				continue
			}
		}
		srcPath := filepath.Join(root, filepath.FromSlash(match))
		dstPath := filepath.Join(outputDir, m.To, filepath.FromSlash(match))
		if err := os.MkdirAll(filepath.Dir(dstPath), 0755); err != nil {
			return err
		}
		if err := fileutil.CopyFile(srcPath, dstPath); err != nil {
			return err
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
		fromDir := m.From
		if ContainsGlobChars(fromDir) {
			fromDir = GlobRoot(fromDir)
		}
		fromAbs := fromDir
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
func copyDirConcurrent(src, dst string, excludes []string) error {
	normalized := NormalizeExcludePatterns(excludes)
	var jobs []copyJob

	if err := filepath.Walk(src, func(path string, fi os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		if rel != "." && len(normalized) > 0 {
			excluded, matchErr := matchExcludeNormalized(normalized, rel)
			if matchErr != nil {
				return matchErr
			}
			if excluded {
				if fi.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}
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
