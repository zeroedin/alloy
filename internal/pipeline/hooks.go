package pipeline

import (
	"fmt"
	"log"
	"path"
	"strings"

	"github.com/zeroedin/alloy/internal/content"
	"github.com/zeroedin/alloy/internal/plugin"
)

func computeUnionScope(scopes []*plugin.HookScope) *plugin.HookScope {
	if len(scopes) == 0 {
		return nil
	}
	for _, s := range scopes {
		if s == nil {
			return nil
		}
	}

	union := &plugin.HookScope{}

	hasAll := false
	hasGlob := false
	hasTaxonomy := false
	var globs []string
	for _, s := range scopes {
		switch s.Pages.Mode {
		case plugin.PagesScopeNone:
			// Hook opted out of pages — does not widen the union.
		case plugin.PagesScopeAll:
			hasAll = true
		case plugin.PagesScopeGlob:
			hasGlob = true
			globs = append(globs, s.Pages.Glob)
		case plugin.PagesScopeTaxonomy:
			hasTaxonomy = true
			if union.Pages.Taxonomies == nil {
				union.Pages.Taxonomies = make(map[string][]string)
			}
			for k, v := range s.Pages.Taxonomies {
				existing := union.Pages.Taxonomies[k]
				seen := make(map[string]bool, len(existing)+len(v))
				for _, t := range existing {
					seen[t] = true
				}
				for _, t := range v {
					if !seen[t] {
						existing = append(existing, t)
						seen[t] = true
					}
				}
				union.Pages.Taxonomies[k] = existing
			}
		}
	}
	if hasAll || (hasGlob && hasTaxonomy) {
		union.Pages.Mode = plugin.PagesScopeAll
	} else if hasGlob {
		allSame := true
		for _, g := range globs {
			if g != globs[0] {
				allSame = false
				break
			}
		}
		if allSame {
			union.Pages.Mode = plugin.PagesScopeGlob
			union.Pages.Glob = globs[0]
		} else {
			union.Pages.Mode = plugin.PagesScopeAll
		}
	} else if hasTaxonomy {
		union.Pages.Mode = plugin.PagesScopeTaxonomy
	}

	allFields := false
	fieldSet := make(map[string]bool)
	for _, s := range scopes {
		if s.PageFields == nil {
			allFields = true
			break
		}
		for _, f := range s.PageFields {
			if f == "*" {
				allFields = true
				break
			}
			fieldSet[f] = true
		}
		if allFields {
			break
		}
	}
	if allFields {
		union.PageFields = nil
	} else {
		union.PageFields = make([]string, 0, len(fieldSet))
		for f := range fieldSet {
			union.PageFields = append(union.PageFields, f)
		}
	}

	allData := false
	dataSet := make(map[string]bool)
	hasData := false
	for _, s := range scopes {
		if s.Data != nil {
			hasData = true
			for _, d := range s.Data {
				if d == "*" {
					allData = true
					break
				}
				dataSet[d] = true
			}
			if allData {
				break
			}
		}
	}
	if allData {
		union.Data = []string{"*"}
	} else if hasData {
		union.Data = make([]string, 0, len(dataSet))
		for d := range dataSet {
			union.Data = append(union.Data, d)
		}
	}

	return union
}

func matchPageGlob(pattern, pageURL string) bool {
	if strings.Contains(pattern, "**") {
		parts := strings.SplitN(pattern, "**", 2)
		prefix := parts[0]
		suffix := parts[1]
		if !strings.HasPrefix(pageURL, prefix) {
			return false
		}
		if suffix == "" || suffix == "/" {
			return true
		}
		rest := pageURL[len(prefix):]
		// Zero-segment match: ** matches nothing, check rest against suffix directly.
		trimSuffix := strings.TrimPrefix(suffix, "/")
		if matched, err := path.Match(trimSuffix, rest); err != nil {
			log.Printf("warning: invalid glob pattern %q: %v", pattern, err)
			return false
		} else if matched {
			return true
		}
		suffixSegments := strings.Count(suffix, "/")
		restParts := strings.Split(rest, "/")
		for i := 0; i <= len(restParts)-suffixSegments-1; i++ {
			candidate := strings.Join(restParts[i:], "/")
			matched, err := path.Match("*"+suffix, candidate)
			if err != nil {
				log.Printf("warning: invalid glob pattern %q: %v", pattern, err)
				return false
			}
			if matched {
				return true
			}
		}
		return false
	}
	matched, err := path.Match(pattern, pageURL)
	if err != nil {
		log.Printf("warning: invalid glob pattern %q: %v", pattern, err)
		return false
	}
	return matched
}

func serializePagesForHook(pages []*content.Page, scope *plugin.HookScope) []plugin.HookPagePayload {
	if scope != nil && scope.Pages.Mode == plugin.PagesScopeNone {
		return nil
	}
	result := make([]plugin.HookPagePayload, 0, len(pages))
	for _, page := range pages {
		if scope != nil && scope.Pages.Mode == plugin.PagesScopeGlob {
			if !matchPageGlob(scope.Pages.Glob, page.URL) {
				continue
			}
		}
		p := plugin.HookPagePayload{
			Path: page.RelPath,
			URL:  page.URL,
		}
		if scope == nil || scope.WantsField("frontMatter") {
			p.FrontMatter = convertOrderedMaps(page.FrontMatter)
		}
		if scope == nil || scope.WantsField("content") {
			p.Content = string(page.Content)
		}
		if scope == nil || scope.WantsField("html") {
			p.HTML = page.HTML()
		}
		result = append(result, p)
	}
	return result
}

func serializePagesForCascadeHook(pages []*content.Page, scope *plugin.HookScope) []plugin.HookCascadePayload {
	if scope != nil && scope.Pages.Mode == plugin.PagesScopeNone {
		return nil
	}
	result := make([]plugin.HookCascadePayload, 0, len(pages))
	for _, page := range pages {
		if scope != nil && scope.Pages.Mode == plugin.PagesScopeGlob {
			if !matchPageGlob(scope.Pages.Glob, page.URL) {
				continue
			}
		}
		result = append(result, plugin.HookCascadePayload{
			Path: page.RelPath,
			Data: convertOrderedMaps(page.FrontMatter),
		})
	}
	return result
}

// virtualPageFromMap creates a Page from a plugin-returned map with path, url, frontMatter,
// and html fields. Only sets RenderedBody from html — callers must populate Content/Body
// if the page needs to flow through the markdown renderer.
func virtualPageFromMap(m map[string]interface{}) (*content.Page, error) {
	path, _ := m["path"].(string)
	url, _ := m["url"].(string)
	if path == "" || url == "" {
		return nil, fmt.Errorf("virtual page must have both path and url fields")
	}
	fm, _ := m["frontMatter"].(map[string]interface{})
	if fm == nil {
		fm = make(map[string]interface{})
	}
	htmlBody, _ := m["html"].(string)
	page := &content.Page{
		RelPath:      path,
		URL:          url,
		FrontMatter:  fm,
		RenderedBody: []byte(htmlBody),
	}
	if layout, ok := fm["layout"]; ok {
		if s, ok := layout.(string); ok {
			page.Layout = s
		}
	}
	return page, nil
}

// fireContentTransformedHooks fires onContentTransformed once per page
// with a page object payload containing html, toc, path, url, and frontMatter.
// Applies returned modifications back to page fields.
func fireContentTransformedHooks(pages []*content.Page, hooks *plugin.HookRegistry) error {
	if !hooks.HasHooks(plugin.OnContentTransformed) {
		return nil
	}
	scope := computeUnionScope(hooks.ScopeFor(plugin.OnContentTransformed))
	if scope != nil && scope.Pages.Mode == plugin.PagesScopeNone {
		return nil
	}
	for _, page := range pages {
		if scope != nil && scope.Pages.Mode == plugin.PagesScopeGlob {
			if !matchPageGlob(scope.Pages.Glob, page.URL) {
				continue
			}
		}
		payload := plugin.HookTransformPayload{
			Path: page.RelPath,
			URL:  page.URL,
		}
		if scope == nil || scope.WantsField("frontMatter") {
			payload.FrontMatter = convertOrderedMaps(page.FrontMatter)
		}
		if scope == nil || scope.WantsField("html") {
			payload.HTML = page.HTML()
		}
		if scope == nil || scope.WantsField("toc") {
			payload.TOC = page.TOC
		}

		result, err := hooks.RunWithTimeout(plugin.OnContentTransformed, payload)
		if err != nil {
			return fmt.Errorf("plugin hook onContentTransformed (%s): %w", page.RelPath, err)
		}

		if modified, ok := toGoMap(result); ok {
			if scope == nil || scope.WantsField("html") {
				if html, ok := modified["html"].(string); ok {
					page.SetRenderedBody([]byte(html))
				}
			}
			if scope == nil || scope.WantsField("toc") {
				if tocSlice, ok := modified["toc"].([]interface{}); ok {
					page.TOC = deserializeTOC(tocSlice)
				}
			}
			if scope == nil || scope.WantsField("frontMatter") {
				if returnedFM, ok := toGoMap(modified["frontMatter"]); ok {
					page.FrontMatter = returnedFM
				}
			}
		} else if s, ok := result.(string); ok {
			page.SetRenderedBody([]byte(s))
		} else if b, ok := result.([]byte); ok {
			page.SetRenderedBody(b)
		}
	}
	return nil
}

func deserializeTOC(items []interface{}) []content.TOCEntry {
	var entries []content.TOCEntry
	for _, item := range items {
		entry, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		id, _ := entry["id"].(string)
		text, _ := entry["text"].(string)
		level := 0
		switch v := entry["level"].(type) {
		case int:
			level = v
		case float64:
			level = int(v)
		}
		tocEntry := content.TOCEntry{ID: id, Text: text, Level: level}
		if children, ok := entry["children"].([]interface{}); ok {
			tocEntry.Children = deserializeTOC(children)
		}
		entries = append(entries, tocEntry)
	}
	return entries
}

func runOnPagesReady(pages []*content.Page, ps *PipelineState) ([]*content.Page, error) {
	originalCount := len(pages)
	scope := computeUnionScope(ps.Hooks.ScopeFor(plugin.OnPagesReady))
	// Pages.Mode intentionally ignored — virtual page injection needs the full set.
	serialized := make([]plugin.HookPagePayload, len(pages))
	for i, page := range pages {
		p := plugin.HookPagePayload{
			Path: page.RelPath,
			URL:  page.URL,
		}
		if scope == nil || scope.WantsField("frontMatter") {
			p.FrontMatter = convertOrderedMaps(page.FrontMatter)
		}
		if scope == nil || scope.WantsField("content") {
			p.Content = string(page.Body)
		}
		serialized[i] = p
	}
	payload := plugin.HookPagesReadyPayload{
		Pages: serialized,
	}
	if scope == nil || scope.Data == nil || scope.WantsAllData() {
		payload.SiteData = ps.SiteData
	} else if len(scope.Data) > 0 {
		filtered := make(map[string]interface{})
		for _, key := range scope.Data {
			if v, ok := ps.SiteData[key]; ok {
				filtered[key] = v
			}
		}
		payload.SiteData = filtered
	}

	result, err := ps.Hooks.RunWithTimeout(plugin.OnPagesReady, payload)
	if err != nil {
		return nil, fmt.Errorf("plugin hook onPagesReady: %w", err)
	}

	resultMap, ok := toGoMap(result)
	if !ok {
		return pages, nil
	}
	returnedPages, ok := resultMap["pages"].([]interface{})
	if !ok {
		return pages, nil
	}
	if len(returnedPages) < originalCount {
		return nil, fmt.Errorf("plugin hook onPagesReady: returned %d pages but input had %d — plugins must not remove pages", len(returnedPages), originalCount)
	}

	// urlIndex tracks all known URLs. Seeded with original pages, then updated as
	// each virtual page is appended — so virtual-to-virtual collisions are caught too.
	urlIndex := make(map[string]string, len(pages))
	for _, p := range pages {
		if p.URL != "" {
			urlIndex[p.URL] = p.RelPath
		}
	}

	for i := originalCount; i < len(returnedPages); i++ {
		pageMap, ok := toGoMap(returnedPages[i])
		if !ok {
			return nil, fmt.Errorf("plugin hook onPagesReady: virtual page %d: expected map, got %T", i-originalCount, returnedPages[i])
		}
		vp, err := virtualPageFromMap(pageMap)
		if err != nil {
			return nil, fmt.Errorf("plugin hook onPagesReady: virtual page %d: %w", i-originalCount, err)
		}
		if rawContent, ok := pageMap["content"].(string); ok {
			vp.Content = []byte(rawContent)
			vp.Body = []byte(rawContent)
		}
		if existingPath, ok := urlIndex[vp.URL]; ok {
			return nil, fmt.Errorf("plugin hook onPagesReady: virtual page URL %q collides with existing page %s", vp.URL, existingPath)
		}
		urlIndex[vp.URL] = vp.RelPath
		pages = append(pages, vp)
	}

	return pages, nil
}
