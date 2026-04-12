package ssr

// ComponentMap holds the full component tracking state persisted to .alloy/components.json.
type ComponentMap struct {
	// Instances maps hash → ComponentInstance (unique component occurrences)
	Instances map[string]ComponentInstance
	// PageToInstances maps page source path → list of component hashes on that page
	PageToInstances map[string][]string
	// ComponentToPages maps component tag → list of page source paths using it
	ComponentToPages map[string][]string
	// ComponentDeps maps component tag → list of child component tags
	ComponentDeps map[string][]string
	// DefinitionHashes maps component tag → hash of its definition source
	DefinitionHashes map[string]string
}

// NewComponentMap creates an empty component map.
func NewComponentMap() *ComponentMap {
	return &ComponentMap{
		Instances:        make(map[string]ComponentInstance),
		PageToInstances:  make(map[string][]string),
		ComponentToPages: make(map[string][]string),
		ComponentDeps:    make(map[string][]string),
		DefinitionHashes: make(map[string]string),
	}
}

// SaveTo writes the component map to components.json in the given directory.
func (cm *ComponentMap) SaveTo(dir string) error {
	return ErrNotImplemented
}

// LoadComponentMap reads components.json from the given directory.
// Returns an empty ComponentMap if the file does not exist (fresh build).
func LoadComponentMap(dir string) (*ComponentMap, error) {
	return nil, ErrNotImplemented
}

// ShouldSkipSSR returns true if the component's definition hash hasn't changed,
// meaning Phase 2 SSR can be skipped for pages using only unchanged components.
func (cm *ComponentMap) ShouldSkipSSR(componentTag string, currentDefHash string) bool {
	return false
}
