package template

import (
	"fmt"
	"sort"
	"sync"

	"github.com/Notifuse/liquidgo/v5/liquid"

	"github.com/zeroedin/alloy/internal/content"
)

// SitePages builds site.pages, a template map for every page, the first time
// a template reads it (issue #1360). The renders that share a SitePages share
// the list it builds: it is built again only for a different list of pages.
//
// A pipeline uses one SitePages per render pass, so that a step between
// passes that changes a page, such as a plugin hook, gets a new list. It is
// safe for renders that run at the same time.
type SitePages struct {
	mu    sync.Mutex
	pages []*content.Page // the pages list was built from
	list  []interface{}
}

// NewSitePages returns a SitePages that has built no list yet.
func NewSitePages() *SitePages {
	return &SitePages{}
}

// templateMaps returns the template map of each of pages, built once for
// the same list of pages. A list that starts with the pages of the last one,
// as when a language's taxonomy pages are added to it, reuses their maps.
func (s *SitePages) templateMaps(pages []*content.Page) []interface{} {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.list != nil && samePages(s.pages, pages) {
		return s.list
	}
	reused := 0
	if s.list != nil && startsWith(pages, s.pages) {
		reused = len(s.pages)
	}
	list := make([]interface{}, len(pages))
	copy(list, s.list[:reused])
	for i := reused; i < len(pages); i++ {
		list[i] = pages[i].ToTemplateMap()
	}
	s.pages, s.list = pages, list
	return list
}

// samePages reports whether a and b are the same list: the same pages of the
// same backing array.
func samePages(a, b []*content.Page) bool {
	return len(a) == len(b) && (len(a) == 0 || &a[0] == &b[0])
}

// startsWith reports whether pages begins with the pages of prefix.
func startsWith(pages, prefix []*content.Page) bool {
	if len(pages) < len(prefix) {
		return false
	}
	for i, p := range prefix {
		if pages[i] != p {
			return false
		}
	}
	return true
}

// siteDrop is site in a template's context. It gives the same keys as the
// map it replaces, but builds site.pages only when a template reads it
// (issue #1360).
type siteDrop struct {
	values   map[string]interface{} // site's keys other than pages
	pages    []*content.Page
	hasPages bool // site has a pages key: the context has a list of pages
	built    *SitePages
}

// LiquidGet is how liquidgo reads a key of site in a template:
// {{ site.title }}, {{ site.pages }}. As for a map, size is the number of
// keys, and a key site doesn't have reads as nothing.
func (d *siteDrop) LiquidGet(_ *liquid.Context, key string) (interface{}, bool, error) {
	if key == "pages" && d.hasPages {
		return d.built.templateMaps(d.pages), true, nil
	}
	if v, ok := d.values[key]; ok {
		return v, true, nil
	}
	if key == "size" {
		return d.size(), true, nil
	}
	return nil, false, nil
}

// keys returns site's keys in key order, pages among them.
func (d *siteDrop) keys() []string {
	keys := make([]string, 0, d.size())
	for k := range d.values {
		keys = append(keys, k)
	}
	if d.hasPages {
		keys = append(keys, "pages")
	}
	sort.Strings(keys)
	return keys
}

// value returns the value of site's key k, building site.pages for pages.
func (d *siteDrop) value(k string) interface{} {
	if k == "pages" && d.hasPages {
		return d.built.templateMaps(d.pages)
	}
	return d.values[k]
}

// size returns the number of site's keys.
func (d *siteDrop) size() int {
	if d.hasPages {
		return len(d.values) + 1
	}
	return len(d.values)
}

// toMap returns site as the map it replaces, site.pages included.
func (d *siteDrop) toMap() map[string]interface{} {
	m := make(map[string]interface{}, d.size())
	for k, v := range d.values {
		m[k] = v
	}
	if d.hasPages {
		m["pages"] = d.built.templateMaps(d.pages)
	}
	return m
}

// Each gives site's keys and values as [key, value] pairs, in key order, as
// liquidgo loops over a map: {% for pair in site %}.
func (d *siteDrop) Each(fn func(interface{})) {
	for _, k := range d.keys() {
		fn([]interface{}{k, d.value(k)})
	}
}

// LoadSlice gives the pairs from index from up to to, as Each does, for a
// loop with limit or offset. site.pages is built only when its pair is one
// of them.
func (d *siteDrop) LoadSlice(from int, to *int) []interface{} {
	var pairs []interface{}
	for i, k := range d.keys() {
		if to != nil && i >= *to {
			break
		}
		if i >= from {
			pairs = append(pairs, []interface{}{k, d.value(k)})
		}
	}
	return pairs
}

// MarshalJSON writes site as the map it replaces: {{ site | json }}.
func (d *siteDrop) MarshalJSON() ([]byte, error) {
	return jsonCodec.Marshal(d.toMap())
}

// String is the text of {{ site }}: the map's, as liquidgo writes a map.
func (d *siteDrop) String() string {
	return liquid.ToS(d.toMap(), nil)
}

// Format writes site as fmt writes the map it replaces, so that a value
// handed on as Go text, such as a QuickJS plugin filter's input, reads as
// before.
func (d *siteDrop) Format(f fmt.State, verb rune) {
	fmt.Fprintf(f, fmt.FormatString(f, verb), d.toMap())
}
