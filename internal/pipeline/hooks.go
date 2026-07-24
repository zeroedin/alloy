package pipeline

import (
	"fmt"
	"log"
	"path"
	"path/filepath"
	"strings"

	"github.com/zeroedin/alloy/internal/cache"
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
	allPagesExplicit := true
	var globs []string
	for _, s := range scopes {
		if !s.Pages.Explicit {
			allPagesExplicit = false
		}
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
	union.Pages.Explicit = allPagesExplicit

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
// Applies returned modifications back to page fields. When buildCache is
// non-nil, extracts addDependencies from return values and tracks them
// in the cache (issue #1100).
func fireContentTransformedHooks(pages []*content.Page, hooks *plugin.HookRegistry, buildCache ...*cache.Cache) error {
	if !hooks.HasHooks(plugin.OnContentTransformed) {
		return nil
	}
	scope := computeUnionScope(hooks.ScopeFor(plugin.OnContentTransformed))
	if scope != nil && scope.Pages.Mode == plugin.PagesScopeNone && scope.Pages.Explicit {
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
			if len(buildCache) > 0 && buildCache[0] != nil {
				extractAddDependencies(modified, page.RelPath, buildCache[0])
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

// knownPagesReadyKeys are keys recognized in the onPagesReady return map.
var knownPagesReadyKeys = map[string]bool{
	"pages":    true,
	"addPages": true,
	"siteData": true,
}

func runOnPagesReady(pages []*content.Page, ps *PipelineState) ([]*content.Page, error) {
	unionScope := computeUnionScope(ps.Hooks.ScopeFor(plugin.OnPagesReady))

	urlIndex := make(map[string]string, len(pages))
	for _, p := range pages {
		if p.URL != "" {
			urlIndex[p.URL] = p.RelPath
		}
	}

	err := ps.Hooks.RunEachWithTimeout(plugin.OnPagesReady,
		func(_ int, _ *plugin.HookScope) interface{} {
			return buildPagesReadyPayload(pages, unionScope, ps.SiteData)
		},
		func(_ int, _ *plugin.HookScope, result interface{}) error {
			resultMap, ok := toGoMap(result)
			if !ok {
				return nil
			}

			pagesArr, pagesOK := resultMap["pages"].([]interface{})
			addPagesRaw, addPagesExists := resultMap["addPages"]
			addPagesExists = addPagesExists && addPagesRaw != nil

			if pagesOK && addPagesExists {
				return fmt.Errorf("returned both 'pages' and 'addPages' — use one or the other")
			}

			if pagesOK {
				preHookCount := len(pages)
				if len(pagesArr) < preHookCount {
					return fmt.Errorf("returned %d pages but input had %d — plugins must not remove pages", len(pagesArr), preHookCount)
				}
				for j := preHookCount; j < len(pagesArr); j++ {
					vp, err := extractVirtualPage(pagesArr[j], j-preHookCount)
					if err != nil {
						return err
					}
					if existingPath, ok := urlIndex[vp.URL]; ok {
						return fmt.Errorf("virtual page URL %q collides with existing page %s", vp.URL, existingPath)
					}
					urlIndex[vp.URL] = vp.RelPath
					pages = append(pages, vp)
				}
				return nil
			}

			if addPagesExists {
				addPagesArr, ok := addPagesRaw.([]interface{})
				if !ok {
					return fmt.Errorf("addPages must be an array, got %T", addPagesRaw)
				}
				for j, entry := range addPagesArr {
					vp, err := extractVirtualPage(entry, j)
					if err != nil {
						return err
					}
					if existingPath, ok := urlIndex[vp.URL]; ok {
						return fmt.Errorf("virtual page URL %q collides with existing page %s", vp.URL, existingPath)
					}
					urlIndex[vp.URL] = vp.RelPath
					pages = append(pages, vp)
				}
				return nil
			}

			for key := range resultMap {
				if !knownPagesReadyKeys[key] {
					return fmt.Errorf("returned unrecognized shape — expected \"pages\" or \"addPages\"")
				}
			}
			return nil
		},
	)
	if err != nil {
		return nil, fmt.Errorf("plugin hook onPagesReady: %w", err)
	}

	return pages, nil
}

func extractVirtualPage(raw interface{}, index int) (*content.Page, error) {
	pageMap, ok := toGoMap(raw)
	if !ok {
		return nil, fmt.Errorf("virtual page %d: expected map, got %T", index, raw)
	}
	vp, err := virtualPageFromMap(pageMap)
	if err != nil {
		return nil, fmt.Errorf("virtual page %d: %w", index, err)
	}
	if rawContent, ok := pageMap["content"].(string); ok {
		vp.Content = []byte(rawContent)
		vp.Body = []byte(rawContent)
	}
	if depsRaw, ok := pageMap["dependencies"]; ok {
		depsArr, ok := depsRaw.([]interface{})
		if !ok {
			return nil, fmt.Errorf("dependencies must be an array, got %T", depsRaw)
		}
		deps := make([]string, 0, len(depsArr))
		for i, d := range depsArr {
			s, ok := d.(string)
			if !ok {
				return nil, fmt.Errorf("dependencies[%d] must be a string, got %T", i, d)
			}
			cleaned := filepath.ToSlash(filepath.Clean(s))
			if cleaned == "" || cleaned == "." {
				return nil, fmt.Errorf("dependencies[%d]: empty or current-directory path", i)
			}
			if filepath.IsAbs(s) {
				return nil, fmt.Errorf("dependencies[%d]: absolute paths not allowed (%q)", i, s)
			}
			if strings.HasPrefix(cleaned, "../") || cleaned == ".." {
				return nil, fmt.Errorf("dependencies[%d]: path escapes project root (%q)", i, s)
			}
			deps = append(deps, cleaned)
		}
		vp.Dependencies = deps
	}
	return vp, nil
}

func buildPagesReadyPayload(pages []*content.Page, scope *plugin.HookScope, siteData map[string]interface{}) plugin.HookPagesReadyPayload {
	var serialized []plugin.HookPagePayload
	if scope == nil || scope.Pages.Mode != plugin.PagesScopeNone {
		serialized = make([]plugin.HookPagePayload, len(pages))
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
	}

	payload := plugin.HookPagesReadyPayload{
		Pages: serialized,
	}

	if scope == nil || scope.Data == nil || scope.WantsAllData() {
		payload.SiteData = siteData
	} else if len(scope.Data) > 0 {
		filtered := make(map[string]interface{})
		for _, key := range scope.Data {
			if v, ok := siteData[key]; ok {
				filtered[key] = v
			}
		}
		payload.SiteData = filtered
	}

	return payload
}

// buildPageRenderedPayload constructs the onPageRendered hook payload for a page.
func buildPageRenderedPayload(page *content.Page) plugin.HookRenderedPayload {
	fm := convertOrderedMaps(page.FrontMatter)
	if fm == nil {
		fm = map[string]interface{}{}
	}
	return plugin.HookRenderedPayload{
		HTML:        page.HTML(),
		FrontMatter: fm,
		URL:         page.URL,
		Path:        page.RelPath,
	}
}

// extractPageRenderedHTML extracts the html field from an onPageRendered hook result.
// The result may be a map[string]interface{} or *ordered.Map depending on the runtime.
func extractPageRenderedHTML(result interface{}) (string, bool) {
	if m, ok := toGoMap(result); ok {
		html, ok := m["html"].(string)
		return html, ok
	}
	return "", false
}

// extractAddDependencies extracts the addDependencies array from a hook
// result map and tracks each valid path in the build cache (issue #1100).
// Non-array values are ignored with a warning. Non-string entries within
// the array are filtered out with a warning.
func extractAddDependencies(resultMap map[string]interface{}, pageRelPath string, buildCache *cache.Cache) {
	raw, exists := resultMap["addDependencies"]
	if !exists {
		return
	}
	arr, ok := raw.([]interface{})
	if !ok {
		log.Printf("warning: addDependencies for %s: expected array, got %T — ignoring", pageRelPath, raw)
		return
	}
	for _, entry := range arr {
		if depPath, ok := entry.(string); ok {
			buildCache.TrackDependency(pageRelPath, filepath.ToSlash(filepath.Clean(depPath)))
		} else {
			log.Printf("warning: addDependencies for %s: non-string entry %T — skipping", pageRelPath, entry)
		}
	}
}

// buildFormatRenderedPayload constructs the onFormatRendered hook payload for a
// single non-HTML format body (issue #1102). The caller pre-converts
// front matter once per page and passes it in to avoid repeated
// conversion across hooks × formats.
func buildFormatRenderedPayload(page *content.Page, format, body string, fm map[string]interface{}) plugin.HookFormatRenderedPayload {
	return plugin.HookFormatRenderedPayload{
		Format:      format,
		Content:     body,
		URL:         page.URL,
		Path:        page.RelPath,
		FrontMatter: fm,
	}
}

// extractFormatRenderedContent extracts the content field from an onFormatRendered
// hook result. Only the content field is mutable; format, url, path, and frontMatter
// are read-only (issue #1102).
func extractFormatRenderedContent(result interface{}) (string, bool) {
	if m, ok := toGoMap(result); ok {
		c, ok := m["content"].(string)
		return c, ok
	}
	return "", false
}

// pageHasHTMLOutput returns true if the page's Outputs includes "html" or is empty
// (empty defaults to HTML).
func pageHasHTMLOutput(page *content.Page) bool {
	if len(page.Outputs) == 0 {
		return true
	}
	for _, o := range page.Outputs {
		if o == "html" {
			return true
		}
	}
	return false
}

// pageHasNonHTMLOutput returns true if the page's Outputs includes at least one
// non-HTML format (e.g., "json", "xml").
func pageHasNonHTMLOutput(page *content.Page) bool {
	for _, o := range page.Outputs {
		if o != "html" {
			return true
		}
	}
	return false
}
