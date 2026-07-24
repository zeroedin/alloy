package plugin_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zeroedin/alloy/internal/content"
	"github.com/zeroedin/alloy/internal/plugin"
)

// sink prevents dead-code elimination of benchmark results.
var benchSink interface{}

// setupBenchQuickJS creates a QuickJS runtime with a plugin that registers
// an onPageRendered hook using the object API. Returns the runtime; caller
// must call rt.Close().
func setupBenchQuickJS(b *testing.B) *plugin.QuickJSRuntime {
	b.Helper()
	tmpDir := b.TempDir()
	pluginPath := filepath.Join(tmpDir, "bench-hook.js")
	pluginJS := `export default function(alloy) {
  alloy.hook('onPageRendered', {}, function(page) {
    return { html: page.html + '<!-- processed -->' };
  });
}`
	if err := os.WriteFile(pluginPath, []byte(pluginJS), 0644); err != nil {
		b.Fatal(err)
	}

	rt := plugin.NewQuickJSRuntime()
	if err := rt.Init(); err != nil {
		b.Fatal(err)
	}
	if err := rt.EvalFile(pluginPath); err != nil {
		rt.Close()
		b.Fatal(err)
	}
	return rt
}

// buildBenchHTML generates a deterministic HTML string of approximately n cards.
// Each card is ~186 bytes. Used by both struct and string benchmarks to ensure
// identical payloads for comparable measurements.
func buildBenchHTML(n int) string {
	var sb strings.Builder
	for i := 0; i < n; i++ {
		fmt.Fprintf(&sb, "<div class=\"card\" id=\"card-%d\"><h3>Card %d</h3>"+
			"<p>Lorem ipsum dolor sit amet, consectetur adipiscing elit. "+
			"Sed do eiusmod tempor incididunt ut labore et dolore magna aliqua.</p></div>\n", i, i)
	}
	return sb.String()
}

// BenchmarkCallHookRenderedPayload_1KB measures CallHook performance with
// a ~1KB HTML payload through a QuickJS onPageRendered hook. This is the
// baseline for small pages. The fast path (issue #1180) should show
// minimal improvement here since JSON serialization of 1KB is cheap.
func BenchmarkCallHookRenderedPayload_1KB(b *testing.B) {
	rt := setupBenchQuickJS(b)
	defer rt.Close()

	html := strings.Repeat("<p>Short paragraph of content.</p>\n", 20) // ~700B
	payload := plugin.HookRenderedPayload{
		HTML:        html,
		FrontMatter: map[string]interface{}{"title": "Bench Page", "draft": false},
		URL:         "/bench/",
		Path:        "content/bench.md",
	}

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		result, err := rt.CallHook("onPageRendered", payload)
		if err != nil {
			b.Fatal(err)
		}
		benchSink = result
	}
}

// BenchmarkCallHookRenderedPayload_100KB measures CallHook performance with
// a ~100KB HTML payload. This is representative of medium-sized SSR pages.
// The fast path (issue #1180) should show significant improvement: 100KB
// of HTML avoids 4 JSON serialization passes (~400KB of JSON churn per call).
func BenchmarkCallHookRenderedPayload_100KB(b *testing.B) {
	rt := setupBenchQuickJS(b)
	defer rt.Close()

	html := buildBenchHTML(1000) // ~78KB

	payload := plugin.HookRenderedPayload{
		HTML:        html,
		FrontMatter: map[string]interface{}{"title": "Large Bench Page", "tags": []interface{}{"go", "perf"}},
		URL:         "/bench-large/",
		Path:        "content/bench-large.md",
	}

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		result, err := rt.CallHook("onPageRendered", payload)
		if err != nil {
			b.Fatal(err)
		}
		benchSink = result
	}
}

// BenchmarkCallHookRenderedPayload_800KB measures CallHook performance with
// a ~800KB HTML payload — the average page size from the RHDS benchmark in
// issue #1180. This is the key benchmark: the fast path should eliminate
// ~3.2MB of JSON churn per call (800KB × 4 serialization passes).
func BenchmarkCallHookRenderedPayload_800KB(b *testing.B) {
	rt := setupBenchQuickJS(b)
	defer rt.Close()

	html := buildBenchHTML(4300) // ~800KB

	payload := plugin.HookRenderedPayload{
		HTML: html,
		FrontMatter: map[string]interface{}{
			"title":  "RHDS-Scale Page",
			"layout": "component-demo",
			"tags":   []interface{}{"web-components", "ssr", "performance"},
		},
		URL:  "/components/rh-card/demo/",
		Path: "content/components/rh-card/demo.md",
	}

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		result, err := rt.CallHook("onPageRendered", payload)
		if err != nil {
			b.Fatal(err)
		}
		benchSink = result
	}
}

// BenchmarkCallHookRenderedPayload_String_Baseline measures the old v0.5.0
// raw string path for comparison. This is the performance target — the fast
// path should approach this speed since it also uses native string transfer.
// Uses a different plugin script than setupBenchQuickJS because the string
// API hook receives a raw string (not an object) and returns a raw string.
func BenchmarkCallHookRenderedPayload_String_Baseline(b *testing.B) {
	tmpDir := b.TempDir()
	pluginPath := filepath.Join(tmpDir, "bench-string-hook.js")
	pluginJS := `export default function(alloy) {
  alloy.hook('onPageRendered', {}, function(html) {
    return html + '<!-- processed -->';
  });
}`
	if err := os.WriteFile(pluginPath, []byte(pluginJS), 0644); err != nil {
		b.Fatal(err)
	}

	rt := plugin.NewQuickJSRuntime()
	if err := rt.Init(); err != nil {
		b.Fatal(err)
	}
	if err := rt.EvalFile(pluginPath); err != nil {
		rt.Close()
		b.Fatal(err)
	}
	defer rt.Close()

	html := buildBenchHTML(4300) // ~800KB, same size as the struct benchmark

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		result, err := rt.CallHook("onPageRendered", html)
		if err != nil {
			b.Fatal(err)
		}
		benchSink = result
	}
}

// setupBenchQuickJSWithPlugin creates a QuickJS runtime with a custom plugin script.
func setupBenchQuickJSWithPlugin(b *testing.B, pluginJS string) *plugin.QuickJSRuntime {
	b.Helper()
	tmpDir := b.TempDir()
	pluginPath := filepath.Join(tmpDir, "bench-hook.js")
	if err := os.WriteFile(pluginPath, []byte(pluginJS), 0644); err != nil {
		b.Fatal(err)
	}
	rt := plugin.NewQuickJSRuntime()
	if err := rt.Init(); err != nil {
		b.Fatal(err)
	}
	if err := rt.EvalFile(pluginPath); err != nil {
		rt.Close()
		b.Fatal(err)
	}
	return rt
}

// BenchmarkCallHookFormatRenderedPayload_800KB measures CallHook performance
// with a ~800KB content payload through onFormatRendered. This exercises the
// HookFormatRenderedPayload fast path which has an additional string field
// (format) compared to HookRenderedPayload.
func BenchmarkCallHookFormatRenderedPayload_800KB(b *testing.B) {
	rt := setupBenchQuickJSWithPlugin(b, `export default function(alloy) {
  alloy.hook('onFormatRendered', {}, function(payload) {
    return { content: payload.content + '<!-- processed -->' };
  });
}`)
	defer rt.Close()

	content := buildBenchHTML(4300) // ~800KB

	payload := plugin.HookFormatRenderedPayload{
		Format:      "json",
		Content:     content,
		URL:         "/api/data/",
		Path:        "content/api/data.md",
		FrontMatter: map[string]interface{}{"title": "API Data", "layout": "api"},
	}

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		result, err := rt.CallHook("onFormatRendered", payload)
		if err != nil {
			b.Fatal(err)
		}
		benchSink = result
	}
}

// BenchmarkCallHookTransformPayload_800KB measures CallHook performance
// with a ~800KB HTML payload through onContentTransformed. This exercises
// the HookTransformPayload fast path which has an additional TOC array
// field — revealing whether the small-field JSON path is a bottleneck.
func BenchmarkCallHookTransformPayload_800KB(b *testing.B) {
	rt := setupBenchQuickJSWithPlugin(b, `export default function(alloy) {
  alloy.hook('onContentTransformed', {}, function(page) {
    return { html: page.html + '<!-- transformed -->', toc: page.toc };
  });
}`)
	defer rt.Close()

	html := buildBenchHTML(4300) // ~800KB

	payload := plugin.HookTransformPayload{
		HTML: html,
		TOC: []content.TOCEntry{
			{Text: "Introduction", ID: "introduction", Level: 2},
			{Text: "Setup", ID: "setup", Level: 2},
			{Text: "Usage", ID: "usage", Level: 2,
				Children: []content.TOCEntry{
					{Text: "Basic", ID: "basic", Level: 3},
					{Text: "Advanced", ID: "advanced", Level: 3},
				}},
		},
		URL:         "/guide/",
		Path:        "content/guide.md",
		FrontMatter: map[string]interface{}{"title": "Guide", "layout": "doc"},
	}

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		result, err := rt.CallHook("onContentTransformed", payload)
		if err != nil {
			b.Fatal(err)
		}
		benchSink = result
	}
}
