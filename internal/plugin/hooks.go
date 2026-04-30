package plugin

import (
	"context"
	"fmt"
	"time"
)

// HookName identifies a lifecycle event.
type HookName string

const (
	OnConfig             HookName = "onConfig"
	OnBeforeValidation   HookName = "onBeforeValidation"
	OnAfterValidation    HookName = "onAfterValidation"
	OnDataFetched        HookName = "onDataFetched"
	OnDataCascadeReady   HookName = "onDataCascadeReady"
	OnContentLoaded      HookName = "onContentLoaded"
	OnContentTransformed HookName = "onContentTransformed"
	OnPageRendered       HookName = "onPageRendered"
	OnAssetProcess       HookName = "onAssetProcess"
	OnBuildComplete      HookName = "onBuildComplete"
	OnDevServerStart     HookName = "onDevServerStart"
	OnFileChanged        HookName = "onFileChanged"
)

// HookFunc processes a hook payload and returns a (potentially modified) result.
// The context carries the per-hook timeout deadline for cooperative cancellation.
type HookFunc func(ctx context.Context, payload interface{}) (interface{}, error)

// HookRegistry manages lifecycle hook registrations and execution.
type HookRegistry struct {
	hooks    map[HookName][]HookFunc
	timeout  int // per-hook timeout in milliseconds (default 5000)
	warnings []string
}

// NewHookRegistry creates an empty hook registry with a default timeout of 5000ms.
func NewHookRegistry() *HookRegistry {
	return &HookRegistry{
		hooks:   make(map[HookName][]HookFunc),
		timeout: 5000,
	}
}

// SetTimeout configures the per-hook execution timeout in milliseconds.
func (r *HookRegistry) SetTimeout(ms int) {
	r.timeout = ms
}

// Timeout returns the current per-hook timeout in milliseconds.
func (r *HookRegistry) Timeout() int {
	return r.timeout
}

// Warnings returns any warnings produced during hook execution (e.g., timeouts).
func (r *HookRegistry) Warnings() []string {
	return r.warnings
}

// Register adds a hook function for the given event.
func (r *HookRegistry) Register(event HookName, fn HookFunc) {
	r.hooks[event] = append(r.hooks[event], fn)
}

// Run executes all hooks for an event in registration order, chaining results.
func (r *HookRegistry) Run(event HookName, payload interface{}) (interface{}, error) {
	fns := r.hooks[event]
	current := payload
	for _, fn := range fns {
		result, err := fn(context.Background(), current)
		if err != nil {
			return nil, err
		}
		current = result
	}
	return current, nil
}

// RunWithTimeout executes all hooks for an event with timeout enforcement.
// If a hook exceeds the timeout, its modifications are discarded (the pre-hook
// payload is kept), a warning is logged, and the build continues.
// Each hook receives a context with the timeout deadline for cooperative cancellation.
func (r *HookRegistry) RunWithTimeout(event HookName, payload interface{}) (interface{}, error) {
	fns := r.hooks[event]
	current := payload
	for _, fn := range fns {
		preHook := current
		timeout := time.Duration(r.timeout) * time.Millisecond
		ctx, cancel := context.WithTimeout(context.Background(), timeout)

		type hookResult struct {
			val interface{}
			err error
		}
		ch := make(chan hookResult, 1)
		go func() {
			result, err := fn(ctx, preHook)
			ch <- hookResult{result, err}
		}()

		select {
		case res := <-ch:
			cancel()
			if res.err != nil {
				return nil, res.err
			}
			current = res.val
		case <-ctx.Done():
			cancel()
			current = preHook
			r.warnings = append(r.warnings, fmt.Sprintf("hook timeout: %s exceeded %dms", string(event), r.timeout))
		}
	}
	return current, nil
}
