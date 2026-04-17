package ssr

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// ComponentMap holds the full component tracking state persisted to .alloy/components.json.
type ComponentMap struct {
	// PageToComponents maps page source path → list of component tag names on that page
	PageToComponents map[string][]string `json:"pageToComponents"`
	// ComponentToPages maps component tag → list of page source paths using it
	ComponentToPages map[string][]string `json:"componentToPages"`
	// ComponentDeps maps component tag → list of child component tags
	ComponentDeps map[string][]string `json:"componentDeps"`
	// DefinitionHashes maps component tag → hash of its definition source
	DefinitionHashes map[string]string `json:"definitionHashes"`
}

// NewComponentMap creates an empty component map.
func NewComponentMap() *ComponentMap {
	return &ComponentMap{
		PageToComponents: make(map[string][]string),
		ComponentToPages: make(map[string][]string),
		ComponentDeps:    make(map[string][]string),
		DefinitionHashes: make(map[string]string),
	}
}

// SaveTo writes the component map to components.json in the given directory.
func (cm *ComponentMap) SaveTo(dir string) error {
	b, err := json.MarshalIndent(cm, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "components.json"), b, 0644)
}

// LoadComponentMap reads components.json from the given directory.
// Returns an empty ComponentMap if the file does not exist (fresh build).
func LoadComponentMap(dir string) (*ComponentMap, error) {
	path := filepath.Join(dir, "components.json")
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return NewComponentMap(), nil
		}
		return nil, err
	}

	cm := NewComponentMap()
	if err := json.Unmarshal(b, cm); err != nil {
		return nil, err
	}
	return cm, nil
}

// ShouldSkipSSR returns true if the component's definition hash hasn't changed,
// meaning Phase 2 SSR can be skipped for pages using only unchanged components.
func (cm *ComponentMap) ShouldSkipSSR(componentTag string, currentDefHash string) bool {
	stored, ok := cm.DefinitionHashes[componentTag]
	if !ok {
		return false
	}
	return stored == currentDefHash
}
