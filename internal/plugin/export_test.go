package plugin

var ParseScopeJSON = parseScopeJSON
var ParseScopeMap = parseScopeMap

// AppendHook appends a hook name to a NodeRuntime's internal hooks slice for testing.
func AppendHook(r *NodeRuntime, name string) {
	r.hooks = append(r.hooks, name)
}
