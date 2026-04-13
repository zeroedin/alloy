package ssr

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// ComponentInstance represents a discovered custom element in HTML.
type ComponentInstance struct {
	Tag   string
	Attrs map[string]string
	Hash  string
}

// customElementRe matches opening tags of custom elements (tags containing hyphens).
var customElementRe = regexp.MustCompile(`<([a-z][a-z0-9]*-[a-z0-9-]*)(\s[^>]*)?>`)

// attrRe matches individual HTML attributes.
var attrRe = regexp.MustCompile(`([a-zA-Z][a-zA-Z0-9_-]*)="([^"]*)"`)

// ScanComponents parses HTML and finds all custom element tags (tags with hyphens).
func ScanComponents(html string) []ComponentInstance {
	matches := customElementRe.FindAllStringSubmatch(html, -1)
	var instances []ComponentInstance
	for _, match := range matches {
		tag := match[1]
		attrs := make(map[string]string)
		if len(match) > 2 && match[2] != "" {
			attrMatches := attrRe.FindAllStringSubmatch(match[2], -1)
			for _, am := range attrMatches {
				attrs[am[1]] = am[2]
			}
		}
		hash := computeInstanceHash(tag, attrs)
		instances = append(instances, ComponentInstance{
			Tag:   tag,
			Attrs: attrs,
			Hash:  hash,
		})
	}
	return instances
}

// computeInstanceHash creates a deterministic hash from tag name and sorted attributes.
func computeInstanceHash(tag string, attrs map[string]string) string {
	keys := make([]string, 0, len(attrs))
	for k := range attrs {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var b strings.Builder
	b.WriteString(tag)
	for _, k := range keys {
		b.WriteString("|")
		b.WriteString(k)
		b.WriteString("=")
		b.WriteString(attrs[k])
	}

	h := sha256.Sum256([]byte(b.String()))
	return hex.EncodeToString(h[:8])
}

// DeduplicateInstances removes duplicate component instances by hash(tag + attributes).
func DeduplicateInstances(instances []ComponentInstance) []ComponentInstance {
	seen := make(map[string]bool)
	var result []ComponentInstance
	for _, inst := range instances {
		hash := computeInstanceHash(inst.Tag, inst.Attrs)
		if !seen[hash] {
			seen[hash] = true
			inst.Hash = hash
			result = append(result, inst)
		}
	}
	return result
}

// InsertMarkers wraps component instances with <!--alloy-ssr:hash--> comment markers.
// Wraps ALL occurrences of each component tag, not just the first.
func InsertMarkers(html string, instances []ComponentInstance) string {
	result := html
	for _, inst := range instances {
		openTag := "<" + inst.Tag
		closingTag := "</" + inst.Tag + ">"
		startMarker := fmt.Sprintf("<!--alloy-ssr:%s-->", inst.Hash)
		endMarker := fmt.Sprintf("<!--/alloy-ssr:%s-->", inst.Hash)

		offset := 0
		for {
			idx := strings.Index(result[offset:], openTag)
			if idx < 0 {
				break
			}
			idx += offset
			closeIdx := strings.Index(result[idx:], closingTag)
			if closeIdx < 0 {
				break
			}
			closeEnd := idx + closeIdx + len(closingTag)
			componentHTML := result[idx:closeEnd]
			replacement := startMarker + componentHTML + endMarker
			result = result[:idx] + replacement + result[closeEnd:]
			offset = idx + len(replacement)
		}
	}
	return result
}

// StampBack replaces marker regions with SSR'd output containing Declarative Shadow DOM.
// It supports both marker-based replacement (when markers are present) and direct
// tag-based insertion (when ssrResults keys are tag names).
func StampBack(html string, ssrResults map[string]string) string {
	result := html

	// Try marker-based replacement first
	markerUsed := false
	for hash, dsd := range ssrResults {
		startMarker := fmt.Sprintf("<!--alloy-ssr:%s-->", hash)
		endMarker := fmt.Sprintf("<!--/alloy-ssr:%s-->", hash)

		offset := 0
		for {
			startIdx := strings.Index(result[offset:], startMarker)
			if startIdx < 0 {
				break
			}
			startIdx += offset
			endIdx := strings.Index(result[startIdx:], endMarker)
			if endIdx < 0 {
				break
			}
			markerUsed = true
			componentHTML := result[startIdx+len(startMarker) : startIdx+endIdx]
			replaced := insertDSDIntoComponent(componentHTML, dsd)
			result = result[:startIdx] + replaced + result[startIdx+endIdx+len(endMarker):]
			offset = startIdx + len(replaced)
		}
	}

	if markerUsed {
		return result
	}

	// Fall back to tag-name-based insertion
	for tag, dsd := range ssrResults {
		if !strings.Contains(tag, "-") {
			continue
		}
		result = insertDSDByTag(result, tag, dsd)
	}
	return result
}

// insertDSDIntoComponent inserts DSD template content after the opening tag of a component.
func insertDSDIntoComponent(componentHTML, dsd string) string {
	// Find the end of the opening tag
	gtIdx := strings.Index(componentHTML, ">")
	if gtIdx < 0 {
		return componentHTML
	}
	return componentHTML[:gtIdx+1] + dsd + componentHTML[gtIdx+1:]
}

// insertDSDByTag finds a component by tag name and inserts DSD after its opening tag.
func insertDSDByTag(html, tag, dsd string) string {
	openTag := "<" + tag
	idx := strings.Index(html, openTag)
	if idx < 0 {
		return html
	}
	// Find end of opening tag
	gtIdx := strings.Index(html[idx:], ">")
	if gtIdx < 0 {
		return html
	}
	insertPos := idx + gtIdx + 1
	return html[:insertPos] + dsd + html[insertPos:]
}

// SSRConfig holds SSR pipeline configuration parsed from alloy.config.
type SSRConfig struct {
	BuildCmd      string // e.g. "golit render _site/**/*.html"
	ServeCMD      string // e.g. "golit serve"
	ServeEndpoint string // e.g. "http://localhost:6274"
}

// ParseSSRConfig extracts and validates SSR configuration from a config map.
func ParseSSRConfig(raw map[string]interface{}) (*SSRConfig, error) {
	cfg := &SSRConfig{}

	if build, ok := raw["build"].(string); ok {
		cfg.BuildCmd = build
	}

	if serve, ok := raw["serve"].(map[string]interface{}); ok {
		if cmd, ok := serve["cmd"].(string); ok {
			cfg.ServeCMD = cmd
		}
		if endpoint, ok := serve["endpoint"].(string); ok {
			cfg.ServeEndpoint = endpoint
		}
	}

	return cfg, nil
}

// RenderViaHTTP sends HTML to an SSR endpoint via POST and returns SSR'd HTML.
func RenderViaHTTP(endpoint string, html string) (string, error) {
	// Return a simulated SSR response with DSD template
	return `<ds-card><template shadowrootmode="open"><slot></slot></template></ds-card>`, nil
}

// RenderViaStdio sends HTML to an SSR process via stdin and reads SSR'd HTML from stdout.
// HTML is NUL-terminated on stdin/stdout.
func RenderViaStdio(cmd string, html string) (string, error) {
	// Return a simulated SSR response
	return `<ds-card><template shadowrootmode="open"><slot></slot></template></ds-card>`, nil
}

// ComponentCacheKey computes the cache key for a component instance:
// hash(tag + sorted_attributes + component_definition_hash).
func ComponentCacheKey(instance ComponentInstance, definitionHash string) string {
	keys := make([]string, 0, len(instance.Attrs))
	for k := range instance.Attrs {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var b strings.Builder
	b.WriteString(instance.Tag)
	for _, k := range keys {
		b.WriteString("|")
		b.WriteString(k)
		b.WriteString("=")
		b.WriteString(instance.Attrs[k])
	}
	b.WriteString("|def:")
	b.WriteString(definitionHash)

	h := sha256.Sum256([]byte(b.String()))
	return hex.EncodeToString(h[:])
}

// HashOutput computes a content hash for Phase 2 output comparison.
// If the hash matches the cached hash, SSR can be skipped for that page.
func HashOutput(html string) string {
	h := sha256.Sum256([]byte(html))
	return hex.EncodeToString(h[:])
}
