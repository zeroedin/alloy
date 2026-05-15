package template_test

import (
	"fmt"
	"testing"

	"github.com/zeroedin/alloy/internal/ordered"
	tmpl "github.com/zeroedin/alloy/internal/template"
)

// sink prevents dead-code elimination of benchmark results.
var sink interface{}

// buildCEMData creates a realistic CEM-scale dataset:
// numModules modules, each with numDecls declarations containing
// tagName, slots, attributes, events, cssProperties, and members arrays.
func buildCEMData(numModules, numDecls int) []interface{} {
	modules := make([]interface{}, numModules)
	for i := 0; i < numModules; i++ {
		decls := make([]interface{}, numDecls)
		for j := 0; j < numDecls; j++ {
			decl := ordered.New()
			decl.Set("tagName", fmt.Sprintf("rh-element-%d-%d", i, j))
			decl.Set("kind", "class")

			slots := make([]interface{}, 3)
			for s := 0; s < 3; s++ {
				slot := ordered.New()
				slot.Set("name", fmt.Sprintf("slot-%d", s))
				slot.Set("description", fmt.Sprintf("Slot %d for element %d-%d", s, i, j))
				slots[s] = slot
			}
			decl.Set("slots", slots)

			attrs := make([]interface{}, 2)
			for a := 0; a < 2; a++ {
				attr := ordered.New()
				attr.Set("name", fmt.Sprintf("attr-%d", a))
				attr.Set("type", "string")
				attrs[a] = attr
			}
			decl.Set("attributes", attrs)

			events := make([]interface{}, 2)
			for e := 0; e < 2; e++ {
				ev := ordered.New()
				ev.Set("name", fmt.Sprintf("event-%d", e))
				ev.Set("type", "CustomEvent")
				events[e] = ev
			}
			decl.Set("events", events)

			cssProps := make([]interface{}, 3)
			for c := 0; c < 3; c++ {
				cp := ordered.New()
				cp.Set("name", fmt.Sprintf("--rh-prop-%d", c))
				cp.Set("default", "initial")
				cssProps[c] = cp
			}
			decl.Set("cssProperties", cssProps)

			members := make([]interface{}, 2)
			for m := 0; m < 2; m++ {
				mem := ordered.New()
				mem.Set("name", fmt.Sprintf("member-%d", m))
				mem.Set("kind", "field")
				members[m] = mem
			}
			decl.Set("members", members)

			decls[j] = decl
		}

		mod := ordered.New()
		mod.Set("path", fmt.Sprintf("elements/rh-element-%d.js", i))
		mod.Set("declarations", decls)
		modules[i] = mod
	}
	return modules
}

// Benchmark_Flatten measures flatten on CEM-scale nested arrays.
func Benchmark_Flatten(b *testing.B) {
	modules := buildCEMData(96, 10)
	mapped := tmpl.Map(modules, "declarations")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		sink = tmpl.Flatten(mapped)
	}
}

// Benchmark_Where measures where on a flat array of ordered.Map items.
func Benchmark_Where(b *testing.B) {
	modules := buildCEMData(96, 10)
	mapped := tmpl.Map(modules, "declarations")
	flattened := tmpl.Flatten(mapped)
	flat := flattened.([]interface{})

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		sink = tmpl.Where(flat, "tagName", "rh-element-50-5")
	}
}

// Benchmark_Map measures map on CEM-scale ordered.Map array.
func Benchmark_Map(b *testing.B) {
	modules := buildCEMData(96, 10)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		sink = tmpl.Map(modules, "declarations")
	}
}

// Benchmark_FilterChain measures the full map | flatten | where pipeline.
func Benchmark_FilterChain(b *testing.B) {
	modules := buildCEMData(96, 10)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		mapped := tmpl.Map(modules, "declarations")
		flattened := tmpl.Flatten(mapped)
		sink = tmpl.Where(flattened, "tagName", "rh-element-50-5")
	}
}

// Benchmark_FilterChainLiquid measures the filter chain through a
// Liquid render cycle, including liquidgo dispatch overhead.
// Parse happens before ResetTimer — this measures render only.
func Benchmark_FilterChainLiquid(b *testing.B) {
	modules := buildCEMData(96, 10)

	engine := tmpl.NewLiquidEngine()
	if err := tmpl.RegisterBuiltinFilters(engine); err != nil {
		b.Fatal(err)
	}

	tpl, err := engine.Parse("bench", []byte(
		`{% assign decls = modules | map: "declarations" | flatten %}{% assign match = decls | where: "tagName", "rh-element-50-5" | first %}{{ match.tagName }}`,
	))
	if err != nil {
		b.Fatal(err)
	}

	ctx := map[string]interface{}{"modules": modules}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		out, err := tpl.Render(ctx)
		if err != nil {
			b.Fatal(err)
		}
		sink = out
	}
}

// Benchmark_FilterChainRepeated measures 96 filter chain invocations
// in a single template render — simulates the cumulative cost of
// running the chain once per element page.
func Benchmark_FilterChainRepeated(b *testing.B) {
	modules := buildCEMData(96, 10)

	engine := tmpl.NewLiquidEngine()
	if err := tmpl.RegisterBuiltinFilters(engine); err != nil {
		b.Fatal(err)
	}

	templateSrc := ""
	for i := 0; i < 96; i++ {
		templateSrc += fmt.Sprintf(
			`{%% assign decls_%d = modules | map: "declarations" | flatten %%}{%% assign match_%d = decls_%d | where: "tagName", "rh-element-%d-5" | first %%}{{ match_%d.tagName }},`,
			i, i, i, i, i,
		)
	}

	tpl, err := engine.Parse("bench-repeated", []byte(templateSrc))
	if err != nil {
		b.Fatal(err)
	}

	ctx := map[string]interface{}{"modules": modules}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		out, err := tpl.Render(ctx)
		if err != nil {
			b.Fatal(err)
		}
		sink = out
	}
}

// BenchmarkParseWithDynamicFilters measures the cost of Parse() when
// multiple dynamic/plugin filters require template pre-processing.
// With per-call regex compilation, this compiles N regexes per Parse call.
// With cached patterns (issue #362), compilation happens once per filter.
func BenchmarkParseWithDynamicFilters(b *testing.B) {
	filterNames := []string{"alpha", "beta", "gamma", "delta", "epsilon"}
	engine := tmpl.NewLiquidEngine()
	for _, name := range filterNames {
		n := name
		if err := engine.AddFilter(n, func(input interface{}, args ...interface{}) interface{} {
			return fmt.Sprintf("[%s:%v]", n, input)
		}); err != nil {
			b.Fatal(err)
		}
	}

	src := []byte(`{{ x | alpha }} {{ x | beta: "a" }} {{ x | gamma }} {{ x | delta: "b", "c" }} {{ x | epsilon }}`)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		tpl, err := engine.Parse("bench-page", src)
		if err != nil {
			b.Fatal(err)
		}
		sink = tpl
	}
}
