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
	dg.deps[parent] = append(dg.deps[parent], child)
}

// GetAffected returns all components that need re-SSR when the given component changes.
// Traverses the reverse dependency graph transitively.
func (dg *DepGraph) GetAffected(component string) []string {
	visited := make(map[string]bool)
	dg.collectAffected(component, visited)
	result := make([]string, 0, len(visited))
	for c := range visited {
		result = append(result, c)
	}
	return result
}

// collectAffected walks the reverse dependency graph, finding all parents
// that directly or transitively depend on the given component.
func (dg *DepGraph) collectAffected(component string, visited map[string]bool) {
	for parent, children := range dg.deps {
		if visited[parent] {
			continue
		}
		for _, child := range children {
			if child == component {
				visited[parent] = true
				// Recurse: anything that depends on parent is also affected
				dg.collectAffected(parent, visited)
				break
			}
		}
	}
}

// InvalidateByDefinition returns all components that need re-rendering when a
// component definition file changes (detected via file watcher).
func (dg *DepGraph) InvalidateByDefinition(componentTag string) []string {
	return dg.GetAffected(componentTag)
}
