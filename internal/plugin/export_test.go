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

// SetBridgeCommand overrides the command a NodeBridge will spawn on Start().
func SetBridgeCommand(b *NodeBridge, name string, args ...string) {
	b.overrideCmdName = name
	b.overrideCmdArgs = args
}
