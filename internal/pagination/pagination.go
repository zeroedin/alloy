package pagination

import (
	"fmt"
	"reflect"
	"strings"
)

// PaginationContext holds pagination state for a single paginated page.
type PaginationContext struct {
	PageNumber   int
	TotalPages   int
	PreviousPage string
	NextPage     string
	First        string
	Last         string
	Items        []interface{}
}

// Paginate splits data into pages and returns pagination contexts and output paths.
func Paginate(data []interface{}, perPage int, basePath string, pathSegment string) ([]PaginationContext, []string, error) {
	if len(data) == 0 {
		return nil, nil, nil
	}
	if perPage <= 0 {
		perPage = 1
	}

	// Chunk the data
	var chunks [][]interface{}
	for i := 0; i < len(data); i += perPage {
		end := i + perPage
		if end > len(data) {
			end = len(data)
		}
		chunks = append(chunks, data[i:end])
	}

	totalPages := len(chunks)

	// Build output paths
	paths := make([]string, totalPages)
	paths[0] = basePath
	for i := 1; i < totalPages; i++ {
		paths[i] = fmt.Sprintf("%s%s/%d/", basePath, pathSegment, i+1)
	}

	// Build contexts
	contexts := make([]PaginationContext, totalPages)
	for i, chunk := range chunks {
		ctx := PaginationContext{
			PageNumber: i + 1,
			TotalPages: totalPages,
			First:      paths[0],
			Last:       paths[totalPages-1],
			Items:      chunk,
		}
		if i > 0 {
			ctx.PreviousPage = paths[i-1]
		}
		if i < totalPages-1 {
			ctx.NextPage = paths[i+1]
		}
		contexts[i] = ctx
	}

	return contexts, paths, nil
}

// ResolveDataSource looks up a data source reference (e.g., "site.data.team",
// "collections.articles") and returns the resolved data slice.
func ResolveDataSource(ref string, siteData map[string]interface{}, collections map[string]interface{}) ([]interface{}, error) {
	parts := strings.Split(ref, ".")

	if len(parts) >= 3 && parts[0] == "site" && parts[1] == "data" {
		key := parts[2]
		if siteData == nil {
			return nil, fmt.Errorf("data source %q not found: site data is nil", ref)
		}
		val, ok := siteData[key]
		if !ok {
			return nil, fmt.Errorf("data source %q not found in site data", ref)
		}
		slice, ok := toInterfaceSlice(val)
		if !ok {
			return nil, fmt.Errorf("data source %q is not a slice", ref)
		}
		return slice, nil
	}

	if len(parts) >= 2 && parts[0] == "collections" {
		key := parts[1]
		if collections == nil {
			return nil, fmt.Errorf("data source %q not found: collections is nil", ref)
		}
		val, ok := collections[key]
		if !ok {
			return nil, fmt.Errorf("data source %q not found in collections", ref)
		}
		slice, ok := toInterfaceSlice(val)
		if !ok {
			return nil, fmt.Errorf("data source %q is not a slice", ref)
		}
		return slice, nil
	}

	return nil, fmt.Errorf("unknown data source reference: %q", ref)
}

// PaginateWithLiquidPermalink creates virtual pages using a Liquid permalink
// pattern for each page (e.g., "/team/{{ member.slug }}/"). Each item in data
// gets its own page with a URL rendered from the Liquid template.
func PaginateWithLiquidPermalink(data []interface{}, permalinkTemplate string, as string) ([]PaginationContext, []string, error) {
	if len(data) == 0 {
		return nil, nil, nil
	}

	contexts := make([]PaginationContext, len(data))
	paths := make([]string, len(data))

	for i, item := range data {
		// Render the permalink by simple template substitution
		rendered := RenderSimpleLiquid(permalinkTemplate, as, item)
		paths[i] = rendered
		contexts[i] = PaginationContext{
			PageNumber: i + 1,
			TotalPages: len(data),
			Items:      []interface{}{item},
		}
		if i > 0 {
			contexts[i].PreviousPage = paths[i-1]
		}
		contexts[i].First = paths[0]
	}

	// Set last and next for all contexts now that all paths are computed
	lastPath := paths[len(paths)-1]
	for i := range contexts {
		contexts[i].Last = lastPath
		if i < len(contexts)-1 {
			contexts[i].NextPage = paths[i+1]
		}
	}

	return contexts, paths, nil
}

// TemplateRenderer renders a template string with the given context variables.
type TemplateRenderer func(source string, ctx map[string]interface{}) (string, error)

// PaginateWithTemplatePermalink generates one virtual page per data item,
// using the provided renderer callback to resolve the permalink template.
// This works with any template engine (Liquid, Go templates, etc.).
func PaginateWithTemplatePermalink(data []interface{}, permalinkTemplate string, as string, renderer TemplateRenderer) ([]PaginationContext, []string, error) {
	if len(data) == 0 {
		return nil, nil, nil
	}

	contexts := make([]PaginationContext, len(data))
	paths := make([]string, len(data))

	for i, item := range data {
		ctx := map[string]interface{}{as: item}
		rendered, err := renderer(permalinkTemplate, ctx)
		if err != nil {
			return nil, nil, fmt.Errorf("template permalink render for item %d: %w", i, err)
		}
		paths[i] = strings.TrimSpace(rendered)
		contexts[i] = PaginationContext{
			PageNumber: i + 1,
			TotalPages: len(data),
			Items:      []interface{}{item},
		}
		if i > 0 {
			contexts[i].PreviousPage = paths[i-1]
		}
		contexts[i].First = paths[0]
	}

	lastPath := paths[len(paths)-1]
	for i := range contexts {
		contexts[i].Last = lastPath
		if i < len(contexts)-1 {
			contexts[i].NextPage = paths[i+1]
		}
	}

	return contexts, paths, nil
}

// RenderSimpleLiquid renders a simple Liquid-style template by replacing
// {{ varName.field }} with the corresponding value from the item.
func RenderSimpleLiquid(tmpl string, varName string, item interface{}) string {
	result := tmpl
	itemMap, isMap := item.(map[string]interface{})
	if !isMap {
		placeholder := fmt.Sprintf("{{ %s }}", varName)
		result = strings.ReplaceAll(result, placeholder, fmt.Sprintf("%v", item))
		return result
	}

	// Replace {{ varName.field }} patterns
	for key, val := range itemMap {
		placeholder := fmt.Sprintf("{{ %s.%s }}", varName, key)
		result = strings.ReplaceAll(result, placeholder, fmt.Sprintf("%v", val))
	}
	return result
}

// toInterfaceSlice converts a value to []interface{}.
// Handles both []interface{} directly and typed slices (e.g., []*content.Page)
// via reflection.
func toInterfaceSlice(val interface{}) ([]interface{}, bool) {
	if slice, ok := val.([]interface{}); ok {
		return slice, true
	}
	rv := reflect.ValueOf(val)
	if rv.Kind() != reflect.Slice {
		return nil, false
	}
	result := make([]interface{}, rv.Len())
	for i := 0; i < rv.Len(); i++ {
		result[i] = rv.Index(i).Interface()
	}
	return result, true
}
