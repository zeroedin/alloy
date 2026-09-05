package static

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/bmatcuk/doublestar/v4"
	"github.com/zeroedin/alloy/internal/config"
)

// PlannedCopy is one file the copy stage will write into the output directory.
// Rel is the destination path relative to the output directory, slash-separated
// so it compares directly against the output paths computed for rendered pages.
//
// Planning is separated from copying so output path conflict detection (issue
// #1238) can see exactly the set of files that will be written — including the
// effect of glob resolution and exclude patterns — without walking the trees a
// second time and drifting from what the copy stage actually does.
type PlannedCopy struct {
	Src string
	Rel string
}

// planDir walks src the way copyDirConcurrent does — same exclude semantics,
// same skip-directory behavior — and reports the files it would copy plus the
// directories it would create. dstRel prefixes every reported Rel.
func planDir(src, dstRel string, excludes []string) (files []PlannedCopy, dirs []string, err error) {
	normalized := NormalizeExcludePatterns(excludes)

	walkErr := filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		if rel != "." && len(normalized) > 0 {
			excluded, matchErr := MatchExcludeNormalized(normalized, rel)
			if matchErr != nil {
				return matchErr
			}
			if excluded {
				if d.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}
		}
		outRel := joinRel(dstRel, rel)
		if d.IsDir() {
			dirs = append(dirs, outRel)
			return nil
		}
		files = append(files, PlannedCopy{Src: path, Rel: outRel})
		return nil
	})
	if walkErr != nil {
		return nil, nil, walkErr
	}
	return files, dirs, nil
}

// joinRel joins an output-relative prefix with a walk-relative path. The result
// is cleaned and slash-separated so it equals the destination filepath.Join
// produces at copy time — an uncleaned "css/../js/x.css" claim would never match
// the "js/x.css" the copy actually writes.
func joinRel(prefix, rel string) string {
	rel = filepath.ToSlash(rel)
	prefix = filepath.ToSlash(prefix)
	if rel == "." {
		rel = ""
	}
	var joined string
	switch {
	case prefix == "" || prefix == ".":
		joined = rel
	case rel == "":
		joined = prefix
	default:
		joined = prefix + "/" + rel
	}
	if joined == "" {
		return ""
	}
	return filepath.ToSlash(filepath.Clean(joined))
}

// PlanStatic reports the files CopyStatic would write. A missing directory
// plans nothing, matching CopyStatic's behavior.
func PlanStatic(staticDir string) ([]PlannedCopy, error) {
	info, err := os.Stat(staticDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("static path %q is not a directory", staticDir)
	}
	files, _, err := planDir(staticDir, "", nil)
	return files, err
}

// PlanWalk reports the files a plain recursive copy of dir would write, with no
// exclude patterns. Used for the assets tree, which ProcessAssets writes
// one-for-one at its source-relative path.
func PlanWalk(dir string) ([]PlannedCopy, error) {
	info, err := os.Stat(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	// A plain file would walk as rel "." and claim an empty output path.
	if !info.IsDir() {
		return nil, nil
	}
	files, _, err := planDir(dir, "", nil)
	return files, err
}

// PlannedMapping pairs a passthrough mapping with the files it will write.
type PlannedMapping struct {
	Mapping config.PassthroughMapping
	Files   []PlannedCopy
}

// PlanPassthrough reports the files CopyPassthrough would write for each
// mapping, after glob resolution and exclude filtering. Mappings whose source
// does not exist plan nothing — the copy stage remains the authority on that
// error, so collection never reports a conflict for a build that will fail for
// a different reason first.
func PlanPassthrough(mappings []config.PassthroughMapping, projectRoot string) ([]PlannedMapping, error) {
	var planned []PlannedMapping

	for _, m := range mappings {
		if ContainsGlobChars(m.From) {
			files, err := planGlob(m, projectRoot)
			if err != nil {
				return nil, err
			}
			planned = append(planned, PlannedMapping{Mapping: m, Files: files})
			continue
		}

		fromPath := m.From
		if !filepath.IsAbs(fromPath) {
			fromPath = filepath.Join(projectRoot, fromPath)
		}

		info, err := os.Stat(fromPath)
		if err != nil {
			continue
		}

		if info.IsDir() {
			files, _, err := planDir(fromPath, m.To, m.Exclude)
			if err != nil {
				return nil, err
			}
			planned = append(planned, PlannedMapping{Mapping: m, Files: files})
			continue
		}

		// A file source writes to m.To exactly, not m.To/<basename>.
		if len(m.Exclude) > 0 {
			excluded, err := MatchExclude(m.Exclude, filepath.Base(fromPath))
			if err != nil {
				return nil, err
			}
			if excluded {
				continue
			}
		}
		planned = append(planned, PlannedMapping{
			Mapping: m,
			Files:   []PlannedCopy{{Src: fromPath, Rel: joinRel(m.To, "")}},
		})
	}

	return planned, nil
}

// planGlob mirrors copyGlob's resolution: match files under the glob root, drop
// excluded ones, and place each at m.To/<match>.
func planGlob(m config.PassthroughMapping, projectRoot string) ([]PlannedCopy, error) {
	absPattern := m.From
	if !filepath.IsAbs(absPattern) {
		absPattern = filepath.Join(projectRoot, absPattern)
	}

	root := GlobRoot(absPattern)
	relPattern, err := filepath.Rel(root, absPattern)
	if err != nil {
		return nil, fmt.Errorf("passthrough glob %q: %w", m.From, err)
	}
	relPattern = filepath.ToSlash(relPattern)

	matches, err := doublestar.Glob(os.DirFS(root), relPattern, doublestar.WithFilesOnly())
	if err != nil {
		return nil, fmt.Errorf("passthrough glob %q: %w", m.From, err)
	}

	normalized := NormalizeExcludePatterns(m.Exclude)
	var files []PlannedCopy
	for _, match := range matches {
		if len(normalized) > 0 {
			excluded, err := MatchExcludeNormalized(normalized, match)
			if err != nil {
				return nil, err
			}
			if excluded {
				continue
			}
		}
		files = append(files, PlannedCopy{
			Src: filepath.Join(root, filepath.FromSlash(match)),
			Rel: joinRel(m.To, match),
		})
	}
	return files, nil
}

// FilterManagedMappings drops mappings whose "from" resolves inside a managed
// directory, the source-side skip CopyPassthroughWithValidation applies. A
// mapping skipped at copy time must also be skipped when collecting claims, or
// it is reported as conflicting despite never being written.
func FilterManagedMappings(mappings []config.PassthroughMapping, projectRoot string, managedDirs []string) []config.PassthroughMapping {
	var filtered []config.PassthroughMapping
	for _, m := range mappings {
		if !isInManagedDir(m.From, projectRoot, managedDirs) {
			filtered = append(filtered, m)
		}
	}
	return filtered
}
