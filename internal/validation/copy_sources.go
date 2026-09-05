package validation

import (
	"path/filepath"
	"strings"

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

// CopyClaim is one output path a copy source will write. Label is the
// human-readable source shown in a conflict error; Origin is the
// project-relative path of the file that produces it.
//
// Label and Origin differ for passthrough mappings — the label names the
// mapping ("passthrough \"vendor-css\" → \"css\"") while the origin names the
// individual file ("vendor-css/vendor.css"). The dev server matches on Origin,
// so a watcher recopy recognizes a path as its own claim rather than treating
// it as a collision with itself.
type CopyClaim struct {
	Path   string
	Label  string
	Origin string
}

// Entry renders the claim for the conflict detector.
func (c CopyClaim) Entry() OutputPathEntry {
	return OutputPathEntry{Path: c.Path, Source: c.Label}
}

// CollectCopyClaims returns one claim per file the copy stage will write, ready
// to be appended to the rendered-page entries before DetectConflicts runs.
//
// The returned set is exactly the set that will be copied: passthrough glob
// resolution, exclude patterns, and the managed-directory skip are all applied
// here through the same helpers the copy stage uses. A path recorded here that
// would never be written is as much a defect as one that is missed, because it
// fails a build that would otherwise be correct.
func CollectCopyClaims(src CopySources) ([]CopyClaim, error) {
	var claims []CopyClaim

	staticFiles, err := static.PlanStatic(src.StaticDir)
	if err != nil {
		return nil, err
	}
	staticLabel := SourceDirLabel(src.ProjectRoot, src.StaticDir)
	for _, f := range staticFiles {
		label := joinLabel(staticLabel, f.Rel)
		claims = append(claims, CopyClaim{Path: f.Rel, Label: label, Origin: label})
	}

	assetFiles, err := static.PlanWalk(src.AssetsDir)
	if err != nil {
		return nil, err
	}
	assetLabel := SourceDirLabel(src.ProjectRoot, src.AssetsDir)
	for _, f := range assetFiles {
		label := joinLabel(assetLabel, f.Rel)
		claims = append(claims, CopyClaim{Path: f.Rel, Label: label, Origin: label})
	}

	mappings := static.FilterManagedMappings(src.Passthrough, src.ProjectRoot, src.ManagedDirs)
	planned, err := static.PlanPassthrough(mappings, src.ProjectRoot)
	if err != nil {
		return nil, err
	}
	for _, pm := range planned {
		label := "passthrough \"" + pm.Mapping.From + "\" → \"" + pm.Mapping.To + "\""
		for _, f := range pm.Files {
			claims = append(claims, CopyClaim{
				Path:   f.Rel,
				Label:  label,
				Origin: originPath(src.ProjectRoot, f.Src),
			})
		}
	}

	contentLabel := SourceDirLabel(src.ProjectRoot, src.ContentDir)
	for _, rel := range src.ContentPassthroughs {
		rel = filepath.ToSlash(rel)
		origin := joinLabel(contentLabel, rel)
		claims = append(claims, CopyClaim{
			Path:   rel,
			Label:  origin + " (colocated)",
			Origin: origin,
		})
	}

	return claims, nil
}

// originPath renders an absolute source file as the project-relative path the
// file watcher reports, so a claim can be matched against a change event.
func originPath(projectRoot, abs string) string {
	if rel, err := filepath.Rel(projectRoot, abs); err == nil && !strings.HasPrefix(rel, "..") {
		return filepath.ToSlash(rel)
	}
	return filepath.ToSlash(abs)
}

// SourceDirLabel renders a source directory the way the author configured it —
// relative to the project root — so errors name "static/css/styles.css" rather
// than an absolute build path. Exported because rendered-page sources need the
// same content-directory prefix.
func SourceDirLabel(projectRoot, dir string) string {
	if dir == "" {
		return ""
	}
	if rel, err := filepath.Rel(projectRoot, dir); err == nil && rel != "." &&
		!filepath.IsAbs(rel) && !strings.HasPrefix(filepath.ToSlash(rel), "../") {
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
