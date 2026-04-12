package output

import (
	"github.com/zeroedin/alloy/internal/config"
	"github.com/zeroedin/alloy/internal/content"
)

// GenerateSitemap creates a sitemap.xml from the published pages.
func GenerateSitemap(pages []*content.Page, cfg config.SitemapConfig, baseURL string) ([]byte, error) {
	return nil, ErrNotImplemented
}
