package plugin

import "runtime"

// AutoWorkerCount returns the auto-scaled worker count: min(NumCPU/2, 8) with floor 2.
func AutoWorkerCount() int {
	n := runtime.NumCPU() / 2
	if n < 2 {
		n = 2
	}
	if n > 8 {
		n = 8
	}
	return n
}

// ResolveWorkerCount resolves a configured worker count to a concrete integer.
// Accepts "auto" (string), 0, or negative values → auto-scaled.
// Positive integers are returned as-is (no floor applied).
func ResolveWorkerCount(configured interface{}) int {
	switch v := configured.(type) {
	case int:
		if v > 0 {
			return v
		}
		return AutoWorkerCount()
	case float64:
		if int(v) > 0 {
			return int(v)
		}
		return AutoWorkerCount()
	default:
		return AutoWorkerCount()
	}
}
