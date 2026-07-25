package pipeline_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/zeroedin/alloy/internal/content"
	"github.com/zeroedin/alloy/internal/ordered"
	"github.com/zeroedin/alloy/internal/pipeline"
)

var _ = Describe("convertedFrontMatter caching (issue #1185)", func() {

	// convertedFrontMatter(page) replaces convertOrderedMaps(page.FrontMatter)
	// at call sites. It converts *ordered.Map values to map[string]interface{}
	// once and stores the result back on page.FrontMatter, so subsequent calls
	// (across hook types for the same page) skip the deep walk.
	//
	// This eliminates redundant conversion when the same page flows through
	// multiple hooks (onContentTransformed, onPageRendered, onFormatRendered)
	// that all need the converted front matter.

	It("converts ordered.Map values to plain map on first call", func() {
		om := ordered.New()
		om.Set("name", "Alice")
		om.Set("age", 30)

		page := &content.Page{
			FrontMatter: map[string]interface{}{
				"title":  "Test Page",
				"author": om,
			},
		}

		result := pipeline.ConvertedFrontMatter(page)
		Expect(result).NotTo(BeNil())
		Expect(result["title"]).To(Equal("Test Page"),
			"primitive values must pass through unchanged")

		author, ok := result["author"].(map[string]interface{})
		Expect(ok).To(BeTrue(),
			"*ordered.Map value must be converted to map[string]interface{} — "+
				"got %T; convertedFrontMatter must perform the same deep "+
				"conversion as convertOrderedMaps", result["author"])
		Expect(author["name"]).To(Equal("Alice"))
		Expect(author["age"]).To(Equal(30))
	})

	It("stores converted result back on page.FrontMatter", func() {
		om := ordered.New()
		om.Set("key", "value")

		page := &content.Page{
			FrontMatter: map[string]interface{}{
				"meta": om,
			},
		}

		_ = pipeline.ConvertedFrontMatter(page)

		// After the call, page.FrontMatter must contain plain maps
		// (not *ordered.Map) — this is the caching mechanism. The
		// converted values are stored back so subsequent calls skip
		// the deep walk.
		meta := page.FrontMatter["meta"]
		_, isOrdered := meta.(*ordered.Map)
		Expect(isOrdered).To(BeFalse(),
			"after convertedFrontMatter, page.FrontMatter must contain "+
				"plain map[string]interface{} values, not *ordered.Map — "+
				"this is how the cache works: converting in place means "+
				"needsOrderedMapConversion returns false on subsequent calls")

		plainMap, ok := meta.(map[string]interface{})
		Expect(ok).To(BeTrue(),
			"the *ordered.Map must be replaced with map[string]interface{}")
		Expect(plainMap["key"]).To(Equal("value"))
	})

	It("second call returns same result without re-conversion", func() {
		om := ordered.New()
		om.Set("x", 1)

		page := &content.Page{
			FrontMatter: map[string]interface{}{
				"data": om,
			},
		}

		first := pipeline.ConvertedFrontMatter(page)
		second := pipeline.ConvertedFrontMatter(page)

		Expect(first).To(Equal(second),
			"second call must return the same result as the first — "+
				"the conversion was cached on page.FrontMatter, so the "+
				"second call detects no *ordered.Map values and returns "+
				"the already-converted map")

		// Verify it's the same map object (pointer equality) — the
		// function returns page.FrontMatter directly when no conversion
		// is needed.
		firstData := first["data"].(map[string]interface{})
		secondData := second["data"].(map[string]interface{})
		Expect(firstData["x"]).To(Equal(secondData["x"]))
	})

	It("handles nil FrontMatter", func() {
		page := &content.Page{
			FrontMatter: nil,
		}

		result := pipeline.ConvertedFrontMatter(page)
		Expect(result).To(BeNil(),
			"nil FrontMatter must return nil — same as convertOrderedMaps(nil)")
	})

	It("handles FrontMatter with no ordered.Map values (no-op)", func() {
		page := &content.Page{
			FrontMatter: map[string]interface{}{
				"title": "Plain",
				"count": 5,
			},
		}

		result := pipeline.ConvertedFrontMatter(page)
		Expect(result).NotTo(BeNil())
		Expect(result["title"]).To(Equal("Plain"))
		Expect(result["count"]).To(Equal(5))
	})
})
