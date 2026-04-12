package cascade

// PageContext provides layered data lookup for a single page.
// Global and directory data are shared by pointer across pages.
type PageContext struct {
	Global      *map[string]interface{}
	Directory   *map[string]interface{}
	FrontMatter map[string]interface{}
	Computed    map[string]interface{}
}

// Get looks up a key through the cascade levels (computed > front matter > directory > global).
func (pc *PageContext) Get(key string) interface{} {
	return nil
}

// BuildContext creates a PageContext from the cascade levels.
func BuildContext(global, directory, frontMatter map[string]interface{}) *PageContext {
	return nil
}

// BuildContextFull creates a PageContext with all 5 cascade levels:
//  1. Global data (data/ directory files)
//  2. Directory _data.yaml
//  3. Front matter
//  4. Computed data (post-render)
//  5. Plugin-injected data (onContentTransformed hooks)
func BuildContextFull(global, directory, frontMatter, computed, pluginData map[string]interface{}) *PageContext {
	return nil
}
