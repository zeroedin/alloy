package static

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/zeroedin/alloy/internal/config"
)

// ErrNotImplemented is returned by all stub functions.
var ErrNotImplemented = errors.New("not implemented")

// CopyStatic copies all files from staticDir to outputDir.
// If staticDir does not exist or is empty, it returns nil (no error).
func CopyStatic(staticDir, outputDir string) error {
	info, err := os.Stat(staticDir)
	if err != nil {
		// Missing directory is not an error.
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("static path %q is not a directory", staticDir)
	}

	return filepath.Walk(staticDir, func(path string, fi os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if fi.IsDir() {
			return nil
		}

		rel, err := filepath.Rel(staticDir, path)
		if err != nil {
			return err
		}

		dst := filepath.Join(outputDir, rel)
		return copyFile(path, dst)
	})
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
			if err := copyDir(fromPath, toPath); err != nil {
				return err
			}
		} else {
			if err := copyFile(fromPath, toPath); err != nil {
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

// copyDir recursively copies a directory tree from src to dst.
func copyDir(src, dst string) error {
	return filepath.Walk(src, func(path string, fi os.FileInfo, err error) error {
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

		return copyFile(path, target)
	})
}

// copyFile copies a single file from src to dst, creating parent directories
// as needed. Preserves source file permissions.
func copyFile(src, dst string) error {
	srcInfo, err := os.Stat(src)
	if err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return err
	}

	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, srcInfo.Mode())
	if err != nil {
		return err
	}
	defer out.Close()

	if _, err := io.Copy(out, in); err != nil {
		return err
	}

	return out.Close()
}
