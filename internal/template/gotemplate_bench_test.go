package template_test

import (
	"testing"

	tmpl "github.com/zeroedin/alloy/internal/template"
)

// BenchmarkPartialRendering measures the cost of resolving and
// rendering a single partial via the Go template include function.
// The first iteration reads the file from disk and parses it (cold);
// subsequent iterations hit the sync.Map cache (warm). This benchmark
// catches performance regressions in include resolution/caching.
// Issue #884.
func BenchmarkPartialRendering(b *testing.B) {
	engine := tmpl.NewGoEngine()
	if setter, ok := engine.(interface{ SetIncludesDir(string) }); ok {
		setter.SetIncludesDir("testdata/layouts")
	}

	tpl, err := engine.Parse("bench", []byte(`{{ include "partials/greeting" }}`))
	if err != nil {
		b.Fatal(err)
	}

	ctx := map[string]interface{}{"name": "Benchmark"}

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		out, err := tpl.Render(ctx)
		if err != nil {
			b.Fatal(err)
		}
		sink = out
	}
}

// BenchmarkNestedPartialRendering measures the cost of rendering
// a partial that includes another partial (nav -> nav-links),
// exercising the recursive include path and cache lookup.
// Issue #884.
func BenchmarkNestedPartialRendering(b *testing.B) {
	engine := tmpl.NewGoEngine()
	if setter, ok := engine.(interface{ SetIncludesDir(string) }); ok {
		setter.SetIncludesDir("testdata/layouts")
	}

	tpl, err := engine.Parse("bench-nested", []byte(`{{ include "partials/nav" }}`))
	if err != nil {
		b.Fatal(err)
	}

	ctx := map[string]interface{}{}

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		out, err := tpl.Render(ctx)
		if err != nil {
			b.Fatal(err)
		}
		sink = out
	}
}
