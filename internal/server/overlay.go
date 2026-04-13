package server

import (
	"fmt"
	"html"
	"strings"
)

// BuildError represents a structured error from the build pipeline,
// displayed in the browser error overlay during dev mode (alloy serve only).
type BuildError struct {
	FilePath string // Source file that caused the error (e.g., "content/blog/my-post.md")
	Line     int    // Line number in the source file (0 if unavailable)
	Message  string // Human-readable error description
	Stage    string // Pipeline stage where the failure occurred (e.g., "template rendering")
	Snippet  string // Relevant source code lines around the error
}

// OverlayState tracks active build errors for the dev server error overlay.
// Errors are accumulated during a failed rebuild and cleared on success.
type OverlayState struct {
	errors   []BuildError
	warnings []string
}

// NewOverlayState creates an empty overlay state with no active errors.
func NewOverlayState() *OverlayState {
	return &OverlayState{}
}

// SetErrors records build errors from a failed rebuild.
func (s *OverlayState) SetErrors(errs []BuildError) {
	s.errors = errs
}

// ClearErrors removes all active errors after a successful rebuild.
func (s *OverlayState) ClearErrors() {
	s.errors = nil
}

// HasErrors returns true if there are active build errors.
func (s *OverlayState) HasErrors() bool {
	return len(s.errors) > 0
}

// Errors returns the active build errors.
func (s *OverlayState) Errors() []BuildError {
	return s.errors
}

// SetWarnings records persistent warnings (e.g., unreachable data sources).
func (s *OverlayState) SetWarnings(warnings []string) {
	s.warnings = warnings
}

// Warnings returns the active warnings.
func (s *OverlayState) Warnings() []string {
	return s.warnings
}

// RenderOverlay produces an HTML string for the browser error overlay,
// displaying all active build errors with file path, line number,
// error message, pipeline stage, and source code snippet.
// Used only in dev mode (alloy serve). Never included in alloy build output.
func RenderOverlay(errs []BuildError) string {
	var b strings.Builder
	b.WriteString(`<div id="alloy-error-overlay" style="position:fixed;top:0;left:0;width:100%;height:100%;background:rgba(0,0,0,0.85);color:#fff;z-index:99999;padding:2em;overflow:auto;font-family:monospace;">`)
	for _, e := range errs {
		b.WriteString(`<div class="alloy-error" style="margin-bottom:1em;padding:1em;border:1px solid #f44;">`)
		b.WriteString(fmt.Sprintf(`<div><strong>%s</strong>`, html.EscapeString(e.FilePath)))
		if e.Line > 0 {
			b.WriteString(fmt.Sprintf(` line %d`, e.Line))
		}
		b.WriteString(`</div>`)
		b.WriteString(fmt.Sprintf(`<div>Stage: %s</div>`, html.EscapeString(e.Stage)))
		b.WriteString(fmt.Sprintf(`<div>%s</div>`, html.EscapeString(e.Message)))
		if e.Snippet != "" {
			b.WriteString(fmt.Sprintf(`<pre>%s</pre>`, html.EscapeString(e.Snippet)))
		}
		b.WriteString(`</div>`)
	}
	b.WriteString(`</div>`)
	return b.String()
}

// RenderWarningBanner produces an HTML string for the persistent warning
// banner displayed when data sources are unreachable. Shows alongside
// the error overlay in the browser during dev mode.
func RenderWarningBanner(warnings []string) string {
	var b strings.Builder
	b.WriteString(`<div id="alloy-warning-banner" style="position:fixed;bottom:0;left:0;width:100%;background:#f90;color:#000;padding:0.5em 1em;z-index:99998;font-family:sans-serif;">`)
	for _, w := range warnings {
		b.WriteString(fmt.Sprintf(`<div>%s</div>`, html.EscapeString(w)))
	}
	b.WriteString(`</div>`)
	return b.String()
}
