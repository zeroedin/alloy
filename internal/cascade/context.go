package cascade

// PageContext provides layered data lookup for a single page.
// Global and directory data are shared by pointer across pages.
type PageContext struct {
	Global      *map[string]interface{}
	Directory   *map[string]interface{}
	FrontMatter map[string]interface{}
	Computed    map[string]interface{}
	PluginData  map[string]interface{}
}

// Get looks up a key through the cascade levels.
// Priority order (highest first): PluginData > Computed > FrontMatter > Directory > Global.
// When multiple levels have maps for the same key, they are deep-merged with
// higher priority levels winning on conflicts.
func (pc *PageContext) Get(key string) interface{} {
	// Collect values from all levels (lowest to highest priority)
	var values []interface{}

	if pc.Global != nil {
		if v, ok := (*pc.Global)[key]; ok {
			values = append(values, v)
		}
	}
	if pc.Directory != nil {
		if v, ok := (*pc.Directory)[key]; ok {
			values = append(values, v)
		}
	}
	if pc.FrontMatter != nil {
		if v, ok := pc.FrontMatter[key]; ok {
			values = append(values, v)
		}
	}
	if pc.Computed != nil {
		if v, ok := pc.Computed[key]; ok {
			values = append(values, v)
		}
	}
	if pc.PluginData != nil {
		if v, ok := pc.PluginData[key]; ok {
			values = append(values, v)
		}
	}

	if len(values) == 0 {
		return nil
	}

	// If only one level has the key, return it directly
	if len(values) == 1 {
		return values[0]
	}

	// If the highest priority value is not a map, return it directly
	highest := values[len(values)-1]
	if _, isMap := highest.(map[string]interface{}); !isMap {
		return highest
	}

	// Multiple levels have maps for this key - deep merge from lowest to highest
	merged := make(map[string]interface{})
	for _, v := range values {
		if m, ok := v.(map[string]interface{}); ok {
			merged = DeepMerge(merged, m)
		} else {
			// Non-map value at a higher level overrides everything
			return v
		}
	}
	return merged
}

// BuildContext creates a PageContext from the cascade levels.
func BuildContext(global, directory, frontMatter map[string]interface{}) *PageContext {
	return &PageContext{
		Global:      &global,
		Directory:   &directory,
		FrontMatter: frontMatter,
	}
}

// BuildContextFull creates a PageContext with all 5 cascade levels:
//  1. Global data (data/ directory files)
//  2. Directory _data.yaml
//  3. Front matter
//  4. Computed data (post-render)
//  5. Plugin-injected data (onContentTransformed hooks)
func BuildContextFull(global, directory, frontMatter, computed, pluginData map[string]interface{}) *PageContext {
	return &PageContext{
		Global:      &global,
		Directory:   &directory,
		FrontMatter: frontMatter,
		Computed:    computed,
		PluginData:  pluginData,
	}
}
