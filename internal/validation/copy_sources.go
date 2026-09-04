package validation

import (
	"path/filepath"

	"github.com/zeroedin/alloy/internal/config"
	"github.com/zeroedin/alloy/internal/static"
)

// CopySources describes every filesystem input that is copied verbatim into the
// output directory. These are the sources that PLAN.md §Pre-Build Validation
// lists as rows 2-5 and that never reached the conflict detector before issue
// #1238, so collisions between them resolved silently by write order.
//
// Directories are absolute paths to walk. ProjectRoot anchors relative
// passthrough sources and is also used to label each claim with the path the
// author wrote in their config rather than an absolute path.
type CopySources struct {
	ProjectRoot string

	StaticDir  string
	AssetsDir  string
	ContentDir string

	Passthrough []config.PassthroughMapping
	ManagedDirs []string

	// ContentPassthroughs are output-relative paths of non-content files found
	// under ContentDir by content discovery and copied verbatim.
	ContentPassthroughs []string
}

// CollectCopyClaims returns one OutputPathEntry per file the copy stage will
// write, ready to be appended to the rendered-page entries before
// DetectConflicts runs.
//
// The returned set is exactly the set that will be copied: passthrough glob
// resolution, exclude patterns, and the managed-directory skip are all applied
// here through the same helpers the copy stage uses. A path recorded here that
// would never be written is as much a defect as one that is missed, because it
// fails a build that would otherwise be correct.
func CollectCopyClaims(src CopySources) ([]OutputPathEntry, error) {
	var entries []OutputPathEntry

	staticFiles, err := static.PlanStatic(src.StaticDir)
	if err != nil {
		return nil, err
	}
	staticLabel := SourceDirLabel(src.ProjectRoot, src.StaticDir)
	for _, f := range staticFiles {
		entries = append(entries, OutputPathEntry{
			Path:   f.Rel,
			Source: joinLabel(staticLabel, f.Rel),
		})
	}

	assetFiles, err := static.PlanWalk(src.AssetsDir)
	if err != nil {
		return nil, err
	}
	assetLabel := SourceDirLabel(src.ProjectRoot, src.AssetsDir)
	for _, f := range assetFiles {
		entries = append(entries, OutputPathEntry{
			Path:   f.Rel,
			Source: joinLabel(assetLabel, f.Rel),
		})
	}

	mappings := static.FilterManagedMappings(src.Passthrough, src.ProjectRoot, src.ManagedDirs)
	planned, err := static.PlanPassthrough(mappings, src.ProjectRoot)
	if err != nil {
		return nil, err
	}
	for _, pm := range planned {
		label := "passthrough \"" + pm.Mapping.From + "\" → \"" + pm.Mapping.To + "\""
		for _, f := range pm.Files {
			entries = append(entries, OutputPathEntry{Path: f.Rel, Source: label})
		}
	}

	contentLabel := SourceDirLabel(src.ProjectRoot, src.ContentDir)
	for _, rel := range src.ContentPassthroughs {
		rel = filepath.ToSlash(rel)
		entries = append(entries, OutputPathEntry{
			Path:   rel,
			Source: joinLabel(contentLabel, rel) + " (colocated)",
		})
	}

	return entries, nil
}

// SourceDirLabel renders a source directory the way the author configured it —
// relative to the project root — so errors name "static/css/styles.css" rather
// than an absolute build path. Exported because rendered-page sources need the
// same content-directory prefix.
func SourceDirLabel(projectRoot, dir string) string {
	if dir == "" {
		return ""
	}
	if rel, err := filepath.Rel(projectRoot, dir); err == nil && rel != "." && !filepath.IsAbs(rel) {
		return filepath.ToSlash(rel)
	}
	return filepath.ToSlash(filepath.Base(dir))
}

func joinLabel(dirLabel, rel string) string {
	if dirLabel == "" {
		return rel
	}
	return dirLabel + "/" + rel
}
