package pipeline

import (
	"fmt"
	"io"
	"strings"
	"time"
)

// ProgressReporter receives pipeline stage updates for build progress output.
// Set via SetReporter(). Nil means no progress output. Currently wired into
// Build() only; BuildIncremental() does not yet call the reporter.
type ProgressReporter interface {
	StartStage(name string, total int)
	Message(text string)
	Update(current int, filePath string, elapsed time.Duration)
	EndStage()
	Summary(pageCount int, duration time.Duration, pagesSkipped int)
}

// TTYProgress displays a progress bar with carriage return for interactive terminals.
type TTYProgress struct {
	w         io.Writer
	width     int
	stageName string
	total     int
}

// NewTTYProgress creates a progress reporter for interactive terminals.
// width is the terminal width for progress bar sizing.
func NewTTYProgress(w io.Writer, width int) *TTYProgress {
	if width <= 0 {
		width = 80
	}
	return &TTYProgress{w: w, width: width}
}

func (p *TTYProgress) StartStage(name string, total int) {
	p.stageName = name
	p.total = total
	if total < 0 {
		fmt.Fprintf(p.w, "\r[alloy] %s...", name)
	}
}

func (p *TTYProgress) Message(text string) {
	fmt.Fprintf(p.w, "\r[alloy] %s... %s\n", p.stageName, text)
}

func (p *TTYProgress) Update(current int, filePath string, elapsed time.Duration) {
	if p.total <= 0 {
		return
	}
	pct := current * 100 / p.total

	// Build progress bar
	barWidth := 25
	filled := barWidth * current / p.total
	if filled > barWidth {
		filled = barWidth
	}
	bar := strings.Repeat("▰", filled) + strings.Repeat("▱", barWidth-filled)

	// Truncate file path if needed.
	// The bar uses multi-byte Unicode runes that each occupy one terminal
	// column, so visual width differs from byte length.
	meta := fmt.Sprintf("[alloy] %-12s  %3d%% (%d/%d) ",
		p.stageName, pct, current, p.total)
	visualWidth := len(meta) + barWidth
	prefix := fmt.Sprintf("[alloy] %-12s %s %3d%% (%d/%d) ",
		p.stageName, bar, pct, current, p.total)
	maxPath := p.width - visualWidth
	display := filePath
	if maxPath <= 0 {
		display = ""
	} else if len(display) > maxPath {
		if maxPath > 4 {
			display = "..." + display[len(display)-maxPath+3:]
		} else {
			display = display[:maxPath]
		}
	}

	fmt.Fprintf(p.w, "\r%s%s", prefix, display)
	// Clear any trailing characters from previous longer lines
	if remaining := p.width - visualWidth - len(display); remaining > 0 {
		fmt.Fprintf(p.w, "%s", strings.Repeat(" ", remaining))
	}
}

func (p *TTYProgress) EndStage() {
	if p.total != 0 {
		fmt.Fprintln(p.w)
	}
}

func (p *TTYProgress) Summary(pageCount int, duration time.Duration, pagesSkipped int) {
	if pagesSkipped > 0 {
		fmt.Fprintf(p.w, "[alloy] Built %d pages in %s (%d cached)\n",
			pageCount, formatDuration(duration), pagesSkipped)
	} else {
		fmt.Fprintf(p.w, "[alloy] Built %d pages in %s\n",
			pageCount, formatDuration(duration))
	}
}

// VerboseProgress displays per-file output with timing.
type VerboseProgress struct {
	w         io.Writer
	stageName string
}

// NewVerboseProgress creates a progress reporter for --verbose mode.
func NewVerboseProgress(w io.Writer) *VerboseProgress {
	return &VerboseProgress{w: w}
}

func (p *VerboseProgress) StartStage(name string, total int) {
	p.stageName = strings.ToLower(name)
}

func (p *VerboseProgress) Message(text string) {
	fmt.Fprintf(p.w, "[alloy] %s %s\n", p.stageName, text)
}

func (p *VerboseProgress) Update(current int, filePath string, elapsed time.Duration) {
	fmt.Fprintf(p.w, "[alloy] %-8s %s (%s)\n", p.stageName, filePath, formatDuration(elapsed))
}

func (p *VerboseProgress) EndStage() {}

func (p *VerboseProgress) Summary(pageCount int, duration time.Duration, pagesSkipped int) {
	if pagesSkipped > 0 {
		fmt.Fprintf(p.w, "[alloy] Built %d pages in %s (%d cached)\n",
			pageCount, formatDuration(duration), pagesSkipped)
	} else {
		fmt.Fprintf(p.w, "[alloy] Built %d pages in %s\n",
			pageCount, formatDuration(duration))
	}
}

func formatDuration(d time.Duration) string {
	if d < time.Millisecond {
		return fmt.Sprintf("%dµs", d.Microseconds())
	}
	if d < time.Second {
		return fmt.Sprintf("%dms", d.Milliseconds())
	}
	return fmt.Sprintf("%.1fs", d.Seconds())
}
