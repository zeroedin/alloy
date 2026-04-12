package pagination

import "errors"

// ErrNotImplemented is returned by all stub functions.
var ErrNotImplemented = errors.New("not implemented")

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
	return nil, nil, ErrNotImplemented
}

// ResolveDataSource looks up a data source reference (e.g., "site.data.team",
// "collections.articles") and returns the resolved data slice.
func ResolveDataSource(ref string, siteData map[string]interface{}, collections map[string]interface{}) ([]interface{}, error) {
	return nil, ErrNotImplemented
}

// PaginateWithLiquidPermalink creates virtual pages using a Liquid permalink
// pattern for each page (e.g., "/team/{{ member.slug }}/"). Each item in data
// gets its own page with a URL rendered from the Liquid template.
func PaginateWithLiquidPermalink(data []interface{}, permalinkTemplate string, as string) ([]PaginationContext, []string, error) {
	return nil, nil, ErrNotImplemented
}
