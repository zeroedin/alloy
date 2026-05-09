package plugin

import (
	"context"
	"fmt"
	"log"
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
	OnPagesReady         HookName = "onPagesReady"
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

// PagesScopeMode determines how pages are filtered for a hook.
type PagesScopeMode int

const (
	PagesScopeNone     PagesScopeMode = iota // skip pages entirely
	PagesScopeAll                            // send all pages
	PagesScopeGlob                           // filter by path glob
	PagesScopeTaxonomy                       // filter by taxonomy terms
)

// PagesScope controls which pages a hook receives.
type PagesScope struct {
	Mode       PagesScopeMode
	Glob       string                       // path pattern when Mode == PagesScopeGlob
	Taxonomies map[string][]string          // taxonomy → terms when Mode == PagesScopeTaxonomy
}

// HookScope declares what data subset a plugin hook needs.
type HookScope struct {
	Data       []string   `json:"data"`       // siteData keys; nil = omit, ["*"] = all
	Pages      PagesScope                     // page filtering mode
	PageFields []string   `json:"pageFields"` // per-page fields; nil = all
}

// HookRegistration pairs a hook name with its priority and optional scope.
type HookRegistration struct {
	Name     string
	Priority int
	Scope    *HookScope
}

type priorityHook struct {
	fn       HookFunc
	batchFn  BatchHookFunc  // optional batch-capable variant
	priority int
	index    int            // registration order for stable sort
	scope    *HookScope     // nil = unscoped (full payload)
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

// Warnings returns warnings from hook execution (e.g., timeouts) and plugin loading (e.g., duplicate registrations).
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

// insertHook adds a priorityHook into the event's slice in sorted order.
func (r *HookRegistry) insertHook(event HookName, h priorityHook) {
	h.index = r.nextIdx
	r.nextIdx++
	hooks := r.hooks[event]
	i := sort.Search(len(hooks), func(i int) bool {
		return hooks[i].priority > h.priority || (hooks[i].priority == h.priority && hooks[i].index > h.index)
	})
	hooks = append(hooks, priorityHook{})
	copy(hooks[i+1:], hooks[i:])
	hooks[i] = h
	r.hooks[event] = hooks
}

// RegisterBatchWithPriority adds a hook with both single and batch dispatch functions.
// The batch function is used by RunBatchWithTimeout to distribute work across workers.
func (r *HookRegistry) RegisterBatchWithPriority(event HookName, fn HookFunc, batchFn BatchHookFunc, priority int) {
	r.insertHook(event, priorityHook{fn: fn, batchFn: batchFn, priority: priority})
}

// RegisterWithPriority adds a hook function for the given event with explicit priority.
// Lower priority runs first. Hooks with the same priority preserve registration order.
func (r *HookRegistry) RegisterWithPriority(event HookName, fn HookFunc, priority int) {
	r.insertHook(event, priorityHook{fn: fn, priority: priority})
}

// RegisterWithOptions adds a hook with scope and explicit priority.
func (r *HookRegistry) RegisterWithOptions(event HookName, fn HookFunc, scope HookScope, priority int) {
	r.insertHook(event, priorityHook{fn: fn, priority: priority, scope: &scope})
}

// RegisterBatchWithOptions adds a batch-capable hook with scope and explicit priority.
func (r *HookRegistry) RegisterBatchWithOptions(event HookName, singleFn HookFunc, batchFn BatchHookFunc, scope HookScope, priority int) {
	r.insertHook(event, priorityHook{fn: singleFn, batchFn: batchFn, priority: priority, scope: &scope})
}

// ScopeFor returns the scope for each hook registered on the event, in priority order.
// Returns nil when no hooks are registered. Entries are nil for unscoped hooks.
func (r *HookRegistry) ScopeFor(event HookName) []*HookScope {
	hooks := r.hooks[event]
	if len(hooks) == 0 {
		return nil
	}
	scopes := make([]*HookScope, len(hooks))
	for i, h := range hooks {
		scopes[i] = h.scope
	}
	return scopes
}

// pagelessHooks are hooks whose payload does not contain pages.
var pagelessHooks = map[HookName]bool{
	OnConfig:           true,
	OnBeforeValidation: true,
	OnAfterValidation:  true,
	OnDataFetched:      true,
	OnAssetProcess:     true,
	OnBuildComplete:    true,
	OnDevServerStart:   true,
	OnFileChanged:      true,
}

// preTaxonomyHooks fire before taxonomy indices are built.
var preTaxonomyHooks = map[HookName]bool{
	OnPagesReady: true,
}

// WantsField returns true if name is in PageFields, or PageFields is nil or contains "*".
func (s *HookScope) WantsField(name string) bool {
	if s.PageFields == nil {
		return true
	}
	for _, f := range s.PageFields {
		if f == "*" || f == name {
			return true
		}
	}
	return false
}

// WantsAllData returns true if Data contains "*".
func (s *HookScope) WantsAllData() bool {
	for _, d := range s.Data {
		if d == "*" {
			return true
		}
	}
	return false
}

// ValidateScope checks that a scope is valid for the given hook event.
func ValidateScope(event HookName, scope HookScope) error {
	if scope.Pages.Mode == PagesScopeNone {
		return nil
	}
	if pagelessHooks[event] {
		return fmt.Errorf("hook %s does not receive pages — pages scope is not supported", event)
	}
	if scope.Pages.Mode == PagesScopeTaxonomy && preTaxonomyHooks[event] {
		return fmt.Errorf("hook %s fires before taxonomy indices are built — taxonomy filtering is not supported", event)
	}
	return nil
}

// parseScopeMap builds a HookScope directly from a Go map without JSON serialization.
// Handles the same polymorphic pages field as parseScopeJSON.
func parseScopeMap(m map[string]interface{}) (*HookScope, error) {
	scope := &HookScope{}

	if raw, exists := m["data"]; exists && raw != nil {
		data, ok := raw.([]interface{})
		if !ok {
			return nil, fmt.Errorf("data: expected array, got %T", raw)
		}
		scope.Data = make([]string, 0, len(data))
		for _, d := range data {
			s, ok := d.(string)
			if !ok {
				return nil, fmt.Errorf("data: expected string element, got %T", d)
			}
			scope.Data = append(scope.Data, s)
		}
	}

	if raw, exists := m["pageFields"]; exists && raw != nil {
		pf, ok := raw.([]interface{})
		if !ok {
			return nil, fmt.Errorf("pageFields: expected array, got %T", raw)
		}
		scope.PageFields = make([]string, 0, len(pf))
		for _, f := range pf {
			s, ok := f.(string)
			if !ok {
				return nil, fmt.Errorf("pageFields: expected string element, got %T", f)
			}
			scope.PageFields = append(scope.PageFields, s)
		}
	}

	switch v := m["pages"].(type) {
	case bool:
		if v {
			scope.Pages.Mode = PagesScopeAll
		} else {
			scope.Pages.Mode = PagesScopeNone
		}
	case string:
		if v == "**" {
			scope.Pages.Mode = PagesScopeAll
		} else {
			scope.Pages.Mode = PagesScopeGlob
			scope.Pages.Glob = v
		}
	case map[string]interface{}:
		scope.Pages.Mode = PagesScopeTaxonomy
		scope.Pages.Taxonomies = make(map[string][]string)
		for k, val := range v {
			arr, ok := val.([]interface{})
			if !ok {
				return nil, fmt.Errorf("taxonomy %q: expected array of terms, got %T", k, val)
			}
			terms := make([]string, 0, len(arr))
			for _, item := range arr {
				if s, ok := item.(string); ok {
					terms = append(terms, s)
				} else {
					// Lenient: taxonomy values come from user content where non-string terms are a data quality issue, not a structural error.
					log.Printf("warning: taxonomy %q: ignoring non-string term %v (%T)", k, item, item)
				}
			}
			scope.Pages.Taxonomies[k] = terms
		}
	case nil:
		scope.Pages.Mode = PagesScopeAll
	default:
		return nil, fmt.Errorf("unsupported pages type %T — expected boolean, string, or object", m["pages"])
	}

	return scope, nil
}

// parseScopeJSON unmarshals a JSON scope string and delegates to parseScopeMap.
// Retained for the WASM plugin path which receives scope as a JSON string.
func parseScopeJSON(raw string) (*HookScope, error) {
	var m map[string]interface{}
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		return nil, err
	}
	return parseScopeMap(m)
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
						return nil, fmt.Errorf("%s item %d: %w", string(event), j, res.err)
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
