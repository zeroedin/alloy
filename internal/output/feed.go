package output

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// ResolveFeedTemplates scans the layouts directory for feed templates.
// Template placement determines the feed's output path and context:
//   - layouts/feed.xml → /feed.xml (site-wide feed)
//   - layouts/blog/feed.xml → /blog/feed.xml (section feed)
//   - layouts/taxonomies/tags/feed.xml → /tags/:slug/feed.xml (per-term)
func ResolveFeedTemplates(layoutsDir string) ([]FeedTemplate, error) {
	var templates []FeedTemplate

	err := filepath.WalkDir(layoutsDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}

		if d.Name() != "feed.xml" {
			return nil
		}

		rel, err := filepath.Rel(layoutsDir, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)

		tmpl := FeedTemplate{
			TemplatePath: path,
		}

		dir := filepath.Dir(rel)
		if dir == "." {
			// Site-wide feed: layouts/feed.xml → /feed.xml
			tmpl.OutputPath = "/feed.xml"
		} else if strings.HasPrefix(dir, "taxonomies/") {
			// Taxonomy feed: layouts/taxonomies/tags/feed.xml
			parts := strings.SplitN(dir, "/", 3)
			if len(parts) >= 2 {
				tmpl.Taxonomy = parts[1]
				tmpl.OutputPath = "/" + parts[1] + "/feed.xml"
			}
			if len(parts) >= 3 {
				tmpl.Term = parts[2]
			}
		} else {
			// Section feed: layouts/blog/feed.xml → /blog/feed.xml
			tmpl.Section = dir
			tmpl.OutputPath = "/" + dir + "/feed.xml"
		}

		templates = append(templates, tmpl)
		return nil
	})

	if err != nil {
		return nil, err
	}

	return templates, nil
}

// FeedTemplate represents a discovered feed template and its output context.
type FeedTemplate struct {
	TemplatePath string // Path to the feed template file
	OutputPath   string // Computed output path
	Section      string // Section scope (empty = site-wide)
	Taxonomy     string // Taxonomy scope (empty = not taxonomy)
	Term         string // Taxonomy term (empty = all terms)
}

// RenderFeedTemplate renders a feed template with the appropriate context.
// Returns the raw template content with basic variable substitution.
func RenderFeedTemplate(tmpl FeedTemplate, context map[string]interface{}) ([]byte, error) {
	// Read the template file if it exists
	content, err := os.ReadFile(tmpl.TemplatePath)
	if err != nil {
		// If the template file doesn't exist, generate minimal XML
		return []byte("<?xml version=\"1.0\" encoding=\"UTF-8\"?>\n<rss version=\"2.0\"><channel></channel></rss>"), nil
	}

	result := string(content)

	// Basic Liquid-style variable substitution for feed templates
	if site, ok := context["site"].(map[string]interface{}); ok {
		if title, ok := site["title"].(string); ok {
			result = strings.ReplaceAll(result, "{{ site.title }}", title)
		}
		if baseURL, ok := site["baseURL"].(string); ok {
			result = strings.ReplaceAll(result, "{{ site.baseURL }}", baseURL)
		}
	}

	return []byte(result), nil
}
