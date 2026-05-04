package plugin

import (
	"context"
	"fmt"
	"sort"
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

// BatchHookFunc processes multiple payloads in a single call, returning one
// result per input. Used by subprocess plugins to distribute work across
// multiple worker processes.
type BatchHookFunc func(ctx context.Context, payloads []interface{}) ([]interface{}, error)

type priorityHook struct {
	fn       HookFunc
	batchFn  BatchHookFunc // optional batch-capable variant
	priority int
	index    int // registration order for stable sort
}

// HookRegistry manages lifecycle hook registrations and execution.
type HookRegistry struct {
	hooks    map[HookName][]priorityHook
	timeout  int // per-hook timeout in milliseconds (default 5000)
	warnings []string
	nextIdx  int // monotonic counter for registration order
}

// NewHookRegistry creates an empty hook registry with a default timeout of 5000ms.
func NewHookRegistry() *HookRegistry {
	return &HookRegistry{
		hooks:   make(map[HookName][]priorityHook),
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

// HasHooks returns true if any hooks are registered for the given event.
func (r *HookRegistry) HasHooks(event HookName) bool {
	return len(r.hooks[event]) > 0
}

// Register adds a hook function for the given event with default priority (50).
func (r *HookRegistry) Register(event HookName, fn HookFunc) {
	r.RegisterWithPriority(event, fn, 50)
}

// RegisterBatchWithPriority adds a hook with both single and batch dispatch functions.
// The batch function is used by RunBatchWithTimeout to distribute work across workers.
func (r *HookRegistry) RegisterBatchWithPriority(event HookName, fn HookFunc, batchFn BatchHookFunc, priority int) {
	h := priorityHook{fn: fn, batchFn: batchFn, priority: priority, index: r.nextIdx}
	r.nextIdx++
	hooks := r.hooks[event]
	i := sort.Search(len(hooks), func(i int) bool {
		return hooks[i].priority > priority || (hooks[i].priority == priority && hooks[i].index > h.index)
	})
	hooks = append(hooks, priorityHook{})
	copy(hooks[i+1:], hooks[i:])
	hooks[i] = h
	r.hooks[event] = hooks
}

// RegisterWithPriority adds a hook function for the given event with explicit priority.
// Lower priority runs first. Hooks with the same priority preserve registration order.
func (r *HookRegistry) RegisterWithPriority(event HookName, fn HookFunc, priority int) {
	h := priorityHook{fn: fn, priority: priority, index: r.nextIdx}
	r.nextIdx++
	hooks := r.hooks[event]
	i := sort.Search(len(hooks), func(i int) bool {
		return hooks[i].priority > priority || (hooks[i].priority == priority && hooks[i].index > h.index)
	})
	hooks = append(hooks, priorityHook{})
	copy(hooks[i+1:], hooks[i:])
	hooks[i] = h
	r.hooks[event] = hooks
}

// Run executes all hooks for an event in priority order, chaining results.
func (r *HookRegistry) Run(event HookName, payload interface{}) (interface{}, error) {
	hooks := r.hooks[event]
	current := payload
	for _, h := range hooks {
		result, err := h.fn(context.Background(), current)
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
	hooks := r.hooks[event]
	current := payload
	for _, h := range hooks {
		preHook := current
		timeout := time.Duration(r.timeout) * time.Millisecond
		ctx, cancel := context.WithTimeout(context.Background(), timeout)

		type hookResult struct {
			val interface{}
			err error
		}
		ch := make(chan hookResult, 1)
		go func() {
			result, err := h.fn(ctx, preHook)
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

// RunBatchWithTimeout dispatches multiple payloads through all hooks for an event.
// Hooks with a batchFn use batch dispatch (distributing across workers).
// Hooks without batchFn fall back to per-item dispatch with timeout enforcement.
// Batch timeout scales linearly with payload count (timeout × itemCount).
func (r *HookRegistry) RunBatchWithTimeout(event HookName, payloads []interface{}) ([]interface{}, error) {
	hooks := r.hooks[event]
	current := make([]interface{}, len(payloads))
	copy(current, payloads)

	for _, h := range hooks {
		if h.batchFn != nil {
			preHook := make([]interface{}, len(current))
			copy(preHook, current)
			itemCount := len(current)
			effectiveTimeout := r.timeout * itemCount
			timeout := time.Duration(effectiveTimeout) * time.Millisecond
			ctx, cancel := context.WithTimeout(context.Background(), timeout)

			type batchResult struct {
				val []interface{}
				err error
			}
			ch := make(chan batchResult, 1)
			go func() {
				result, err := h.batchFn(ctx, current)
				ch <- batchResult{result, err}
			}()

			select {
			case res := <-ch:
				cancel()
				if res.err != nil {
					return nil, res.err
				}
				if len(res.val) != itemCount {
					return nil, fmt.Errorf("batch hook %s returned %d results for %d inputs",
						string(event), len(res.val), itemCount)
				}
				current = res.val
			case <-ctx.Done():
				cancel()
				current = preHook
				r.warnings = append(r.warnings, fmt.Sprintf("batch hook timeout: %s exceeded %dms for %d items",
					string(event), effectiveTimeout, itemCount))
			}
		} else {
			for j, payload := range current {
				preItem := current[j]
				timeout := time.Duration(r.timeout) * time.Millisecond
				ctx, cancel := context.WithTimeout(context.Background(), timeout)

				type hookResult struct {
					val interface{}
					err error
				}
				ch := make(chan hookResult, 1)
				go func() {
					result, err := h.fn(ctx, payload)
					ch <- hookResult{result, err}
				}()

				select {
				case res := <-ch:
					cancel()
					if res.err != nil {
						return nil, res.err
					}
					current[j] = res.val
				case <-ctx.Done():
					cancel()
					current[j] = preItem
					r.warnings = append(r.warnings, fmt.Sprintf("hook timeout: %s item %d exceeded %dms",
						string(event), j, r.timeout))
				}
			}
		}
	}
	return current, nil
}
