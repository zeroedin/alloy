package pipeline

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"runtime/pprof"
	"strings"
	"time"
)

// StageTiming records the name and duration of a single pipeline stage.
type StageTiming struct {
	Name     string
	Duration time.Duration
}

// StageTimer records durations of named pipeline stages.
type StageTimer struct {
	timings []StageTiming
	current string
	start   time.Time
}

// Start begins timing a named stage. If a previous stage is still running,
// it is stopped first.
func (t *StageTimer) Start(name string) {
	if t.current != "" {
		t.Stop()
	}
	t.current = name
	t.start = time.Now()
}

// Stop ends the current stage and records its duration.
func (t *StageTimer) Stop() {
	if t.current == "" {
		return
	}
	t.timings = append(t.timings, StageTiming{
		Name:     t.current,
		Duration: time.Since(t.start),
	})
	t.current = ""
}

// Timings returns the recorded stage timings.
func (t *StageTimer) Timings() []StageTiming {
	return t.timings
}

// Profiler manages pprof CPU and memory profiling for a build.
type Profiler struct {
	cpuFile *os.File
	dir     string
}

// StartProfiling begins CPU profiling in the given directory.
// The directory is created if it does not exist.
func StartProfiling(dir string) (*Profiler, error) {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("creating profile directory: %w", err)
	}
	cpuPath := filepath.Join(dir, "cpu.prof")
	f, err := os.Create(cpuPath)
	if err != nil {
		return nil, fmt.Errorf("creating %s: %w", cpuPath, err)
	}
	if err := pprof.StartCPUProfile(f); err != nil {
		f.Close()
		return nil, fmt.Errorf("starting CPU profile: %w", err)
	}
	return &Profiler{cpuFile: f, dir: dir}, nil
}

// StopProfiling stops CPU profiling and writes a heap profile to mem.prof.
func (p *Profiler) StopProfiling() error {
	pprof.StopCPUProfile()
	p.cpuFile.Close()

	runtime.GC()
	memPath := filepath.Join(p.dir, "mem.prof")
	mf, err := os.Create(memPath)
	if err != nil {
		return fmt.Errorf("creating %s: %w", memPath, err)
	}
	defer mf.Close()
	if err := pprof.WriteHeapProfile(mf); err != nil {
		return fmt.Errorf("writing memory profile: %w", err)
	}
	return nil
}

// Dir returns the directory where profile files are written.
func (p *Profiler) Dir() string {
	return p.dir
}

// Report prints a formatted timing table of this timer's recorded stages.
func (t *StageTimer) Report(w io.Writer) {
	PrintStageTimings(w, t.timings)
}

// PrintStageTimings prints a formatted timing table for the given stage timings.
func PrintStageTimings(w io.Writer, timings []StageTiming) {
	if len(timings) == 0 {
		return
	}

	var total time.Duration
	maxName := len("Stage")
	for _, s := range timings {
		total += s.Duration
		if len(s.Name) > maxName {
			maxName = len(s.Name)
		}
	}

	divider := strings.Repeat("─", maxName+24)
	fmt.Fprintf(w, "\n%-*s  %10s  %6s\n", maxName, "Stage", "Duration", "%Total")
	fmt.Fprintln(w, divider)

	for _, s := range timings {
		pct := float64(s.Duration) / float64(total) * 100
		fmt.Fprintf(w, "%-*s  %10s  %5.1f%%\n", maxName, s.Name, formatDuration(s.Duration), pct)
	}

	fmt.Fprintln(w, divider)
	fmt.Fprintf(w, "%-*s  %10s  %5.1f%%\n", maxName, "Total", formatDuration(total), 100.0)
	fmt.Fprintln(w)
}
