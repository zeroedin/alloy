package template_test

import (
	"fmt"
	"testing"

	"github.com/zeroedin/alloy/internal/ordered"
	tmpl "github.com/zeroedin/alloy/internal/template"
)

// buildCEMData creates a realistic CEM-scale dataset:
// numModules modules, each with numDecls declarations containing
// tagName, slots, attributes, and events arrays.
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
	// map "declarations" produces array of arrays
	mapped := tmpl.Map(modules, "declarations")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		tmpl.Flatten(mapped)
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
		tmpl.Where(flat, "tagName", "rh-element-50-5")
	}
}

// Benchmark_Map measures map on CEM-scale ordered.Map array.
func Benchmark_Map(b *testing.B) {
	modules := buildCEMData(96, 10)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		tmpl.Map(modules, "declarations")
	}
}

// Benchmark_FilterChain measures the full map | flatten | where pipeline.
func Benchmark_FilterChain(b *testing.B) {
	modules := buildCEMData(96, 10)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		mapped := tmpl.Map(modules, "declarations")
		flattened := tmpl.Flatten(mapped)
		tmpl.Where(flattened, "tagName", "rh-element-50-5")
	}
}

// Benchmark_FilterChainLiquid measures the filter chain through a full
// Liquid parse → render cycle, including liquidgo dispatch overhead.
func Benchmark_FilterChainLiquid(b *testing.B) {
	modules := buildCEMData(96, 10)

	engine := tmpl.NewLiquidEngine()
	tmpl.RegisterBuiltinFilters(engine)

	tpl, err := engine.Parse("bench", []byte(
		`{% assign decls = modules | map: "declarations" | flatten %}{% assign match = decls | where: "tagName", "rh-element-50-5" | first %}{{ match.tagName }}`,
	))
	if err != nil {
		b.Fatal(err)
	}

	ctx := map[string]interface{}{"modules": modules}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := tpl.Render(ctx)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// Benchmark_FilterChainRepeated simulates a partial called 96 times,
// each running the filter chain — measures cumulative cost.
func Benchmark_FilterChainRepeated(b *testing.B) {
	modules := buildCEMData(96, 10)

	engine := tmpl.NewLiquidEngine()
	tmpl.RegisterBuiltinFilters(engine)

	// Build a template that runs the filter chain 96 times (once per "page")
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
		_, err := tpl.Render(ctx)
		if err != nil {
			b.Fatal(err)
		}
	}
}
