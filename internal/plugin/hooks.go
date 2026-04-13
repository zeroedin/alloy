package plugin

import (
	"errors"
	"fmt"
	"time"
)

// ErrNotImplemented is returned by all stub functions.
var ErrNotImplemented = errors.New("not implemented")

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
type HookFunc func(payload interface{}) (interface{}, error)

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
		result, err := fn(current)
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
func (r *HookRegistry) RunWithTimeout(event HookName, payload interface{}) (interface{}, error) {
	fns := r.hooks[event]
	current := payload
	for _, fn := range fns {
		preHook := current
		type hookResult struct {
			val interface{}
			err error
		}
		ch := make(chan hookResult, 1)
		go func() {
			result, err := fn(preHook)
			ch <- hookResult{result, err}
		}()

		select {
		case res := <-ch:
			if res.err != nil {
				return nil, res.err
			}
			current = res.val
		case <-time.After(time.Duration(r.timeout) * time.Millisecond):
			current = preHook
			r.warnings = append(r.warnings, fmt.Sprintf("hook timeout: %s exceeded %dms", string(event), r.timeout))
		}
	}
	return current, nil
}
