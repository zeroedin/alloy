package output

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"strings"

	"github.com/zeroedin/alloy/internal/config"
	"github.com/zeroedin/alloy/internal/content"
)

// sitemapURLSet is the root element for sitemap XML.
type sitemapURLSet struct {
	XMLName xml.Name     `xml:"urlset"`
	XMLNS   string       `xml:"xmlns,attr"`
	URLs    []sitemapURL `xml:"url"`
}

type sitemapURL struct {
	Loc        string `xml:"loc"`
	ChangeFreq string `xml:"changefreq,omitempty"`
	Priority   string `xml:"priority,omitempty"`
}

// GenerateSitemap creates a sitemap.xml from the published pages.
func GenerateSitemap(pages []*content.Page, cfg config.SitemapConfig, baseURL string) ([]byte, error) {
	baseURL = strings.TrimRight(baseURL, "/")

	urlset := sitemapURLSet{
		XMLNS: "http://www.sitemaps.org/schemas/sitemap/0.9",
	}

	seen := make(map[string]bool)

	for _, page := range pages {
		// Exclude pages with sitemap: false
		if val, ok := page.FrontMatter["sitemap"]; ok {
			if b, ok := val.(bool); ok && !b {
				continue
			}
		}

		// Normalize: ensure consistent trailing slash on all URLs
		loc := strings.TrimRight(baseURL+page.URL, "/") + "/"
		if seen[loc] {
			continue
		}
		seen[loc] = true

		u := sitemapURL{
			Loc: loc,
		}

		// Apply per-page overrides or config defaults
		changeFreq := cfg.ChangeFreq
		priority := cfg.Priority

		if sm, ok := page.FrontMatter["sitemap"].(map[string]interface{}); ok {
			if cf, ok := sm["changefreq"].(string); ok {
				changeFreq = cf
			}
			if p, ok := sm["priority"].(float64); ok {
				priority = p
			}
		}

		if changeFreq != "" {
			u.ChangeFreq = changeFreq
		}
		if priority > 0 {
			u.Priority = fmt.Sprintf("%.1f", priority)
		}

		urlset.URLs = append(urlset.URLs, u)
	}

	var buf bytes.Buffer
	buf.WriteString(xml.Header)
	enc := xml.NewEncoder(&buf)
	enc.Indent("", "  ")
	if err := enc.Encode(urlset); err != nil {
		return nil, fmt.Errorf("sitemap generation error: %w", err)
	}

	return buf.Bytes(), nil
}
