package plugin

var ParseScopeJSON = parseScopeJSON
var ParseScopeMap = parseScopeMap

// AppendHook appends a hook name to a NodeRuntime's internal hooks slice for testing.
func AppendHook(r *NodeRuntime, name string) {
	r.hooks = append(r.hooks, name)
}

// RegisterRuntime exposes Registry.registerRuntime for testing EvalWarner forwarding.
func RegisterRuntime(reg *Registry, rt PluginFilterRuntime, name string, hooks *HookRegistry) {
	reg.registerRuntime(rt, name, hooks)
}

// HookPriorities returns the priorities of all registered hooks for a given event.
func HookPriorities(r *HookRegistry, event HookName) []int {
	hooks := r.hooks[event]
	priorities := make([]int, len(hooks))
	for i, h := range hooks {
		priorities[i] = h.priority
	}
	return priorities
}
