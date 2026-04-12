package content

import "time"

// FilterByLifecycle removes draft, future, and expired pages based on the current time.
// In dev mode (includeDrafts=true), drafts are kept.
func FilterByLifecycle(pages []*Page, now time.Time, includeDrafts bool) []*Page {
	return nil
}
