package content_test

import (
	"fmt"
	"testing"

	"github.com/zeroedin/alloy/internal/content"
)

// sink prevents dead-code elimination of benchmark results.
var sink interface{}

// buildPages creates n Page structs, each with ~bodySize bytes of RenderedBody.
func buildPages(n, bodySize int) []*content.Page {
	body := make([]byte, bodySize)
	for i := range body {
		body[i] = 'x'
	}
	pages := make([]*content.Page, n)
	for i := 0; i < n; i++ {
		b := make([]byte, bodySize)
		copy(b, body)
		pages[i] = &content.Page{
			RenderedBody: b,
			URL:          fmt.Sprintf("/page-%d/", i),
		}
	}
	return pages
}

// BenchmarkStringConversion_Direct measures the cost of calling
// string(page.RenderedBody) three times per page (simulating the
// current pipeline: template context + result map + hook payload).
// This is the baseline that issue #360's HTML() cache eliminates.
func BenchmarkStringConversion_Direct(b *testing.B) {
	pages := buildPages(500, 20_000)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, p := range pages {
			s1 := string(p.RenderedBody)
			s2 := string(p.RenderedBody)
			s3 := string(p.RenderedBody)
			sink = s1
			sink = s2
			sink = s3
		}
	}
}

// BenchmarkStringConversion_HTMLCache measures the cost of calling
// page.HTML() three times per page. After the developer implements
// the lazy cache, the first call converts and subsequent calls return
// the cached string — total cost should be ~1/3 of direct conversion.
func BenchmarkStringConversion_HTMLCache(b *testing.B) {
	pages := buildPages(500, 20_000)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, p := range pages {
			s1 := p.HTML()
			s2 := p.HTML()
			s3 := p.HTML()
			sink = s1
			sink = s2
			sink = s3
		}
	}
}
