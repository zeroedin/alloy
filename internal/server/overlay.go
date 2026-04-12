package server

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
	// stub — no-op
}

// ClearErrors removes all active errors after a successful rebuild.
func (s *OverlayState) ClearErrors() {
	// stub — no-op
}

// HasErrors returns true if there are active build errors.
func (s *OverlayState) HasErrors() bool {
	return false
}

// Errors returns the active build errors.
func (s *OverlayState) Errors() []BuildError {
	return nil
}

// SetWarnings records persistent warnings (e.g., unreachable data sources).
func (s *OverlayState) SetWarnings(warnings []string) {
	// stub — no-op
}

// Warnings returns the active warnings.
func (s *OverlayState) Warnings() []string {
	return nil
}

// RenderOverlay produces an HTML string for the browser error overlay,
// displaying all active build errors with file path, line number,
// error message, pipeline stage, and source code snippet.
// Used only in dev mode (alloy serve). Never included in alloy build output.
func RenderOverlay(errs []BuildError) string {
	return ""
}

// RenderWarningBanner produces an HTML string for the persistent warning
// banner displayed when data sources are unreachable. Shows alongside
// the error overlay in the browser during dev mode.
func RenderWarningBanner(warnings []string) string {
	return ""
}
