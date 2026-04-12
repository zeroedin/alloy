package ssr

// DepGraph tracks component-to-component dependencies for invalidation.
type DepGraph struct {
	deps map[string][]string // parent -> children
}

// NewDepGraph creates an empty component dependency graph.
func NewDepGraph() *DepGraph {
	return &DepGraph{deps: make(map[string][]string)}
}

// AddDependency records that parent uses child in its shadow root.
func (dg *DepGraph) AddDependency(parent, child string) {
	// stub — no-op
}

// GetAffected returns all components that need re-SSR when the given component changes.
func (dg *DepGraph) GetAffected(component string) []string {
	return nil
}

// InvalidateByDefinition returns all pages that need re-rendering when a
// component definition file changes (detected via file watcher).
func (dg *DepGraph) InvalidateByDefinition(componentTag string) []string {
	return nil
}
