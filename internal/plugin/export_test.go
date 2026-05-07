package plugin

var ParseScopeJSON = parseScopeJSON
var ParseScopeMap = parseScopeMap

// NewNodeRuntimeWithHooks creates a NodeRuntime with pre-set hooks and scopes for testing.
func NewNodeRuntimeWithHooks(hooks []string, scopes map[string]*HookScope, priorities map[string]int) *NodeRuntime {
	return &NodeRuntime{
		hooks:          hooks,
		hookScopes:     scopes,
		hookPriorities: priorities,
	}
}
