package content

import "time"

// FilterByLifecycle removes draft, future, and expired pages based on the current time.
// In dev mode (includeDrafts=true), drafts are kept and draft pages bypass date constraints.
// publishDate and expiryDate filtering applies in both build and serve modes.
func FilterByLifecycle(pages []*Page, now time.Time, includeDrafts bool) []*Page {
	var result []*Page
	for _, page := range pages {
		if shouldInclude(page, now, includeDrafts) {
			result = append(result, page)
		}
	}
	return result
}

func shouldInclude(page *Page, now time.Time, includeDrafts bool) bool {
	// In dev mode, draft pages bypass all date constraints
	if includeDrafts && page.Draft {
		return true
	}

	// In build mode, exclude drafts
	if page.Draft && !includeDrafts {
		return false
	}

	// Exclude pages with future publishDate (applies in both build and serve modes)
	if page.PublishDate != nil && page.PublishDate.After(now) {
		return false
	}

	// Exclude pages with past expiryDate (applies in both build and serve modes)
	if page.ExpiryDate != nil && page.ExpiryDate.Before(now) {
		return false
	}

	return true
}
