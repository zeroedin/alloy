package template

import (
	"regexp"
	"strings"
)

// tagName reads a Liquid tag's name: {% name ... %}, after any "-"
// whitespace control and spacing. The name is a word, as Liquid's own tag
// pattern has it.
var tagName = regexp.MustCompile(`^\{%-?\s*([A-Za-z_]\w*)`)

// textRegions are the Liquid tags whose body is not markup, with the
// pattern of their end tag: {% raw %}, {% comment %} and {% doc %}.
var textRegions = map[string]*regexp.Regexp{
	"raw":     regexp.MustCompile(`\{%-?\s*endraw\s*-?%\}`),
	"comment": regexp.MustCompile(`\{%-?\s*endcomment\s*-?%\}`),
	"doc":     regexp.MustCompile(`\{%-?\s*enddoc\s*-?%\}`),
}

// sourceTag is one tag of a template's source.
type sourceTag struct {
	start, end int // the indexes of its "{%" and just after its "%}"
	name       string
}

// closeInlineShortcodes returns src with an {% endname %} added after each
// use of a shortcode in names that src doesn't close, so that every use can
// be parsed as a block (issue #1367). Each template decides for itself,
// partials included.
//
// It reads the tags in order, as liquidgo parses them:
//   - A use is a block when an {% endname %} follows it with no other use of
//     the same name before that end tag: an {% endname %} closes the nearest
//     open use of the same name before it. The block's body runs to that end
//     tag and reaches the shortcode as raw text, so it is left as written,
//     {% raw %} included. A use of the same name inside a {% raw %},
//     {% comment %} or {% doc %} region that ends within the body is text,
//     and doesn't count.
//   - Otherwise the use is inline, and gets an {% endname %} right after it.
//   - Outside a block, a {% raw %}, {% comment %} or {% doc %} region is
//     text, and is left as it is. It runs to the first end tag of its kind
//     in the text, as liquidgo finds it, even after unfinished tag text
//     ({% raw %}{% oops {% endraw %}).
func closeInlineShortcodes(src string, names map[string]bool) string {
	if len(names) == 0 {
		return src
	}
	tags := scanTags(src)
	if len(tags) == 0 {
		return src
	}

	// nextEnd[i] is the index of the next "end"+tags[i].name tag, or
	// len(tags) if there is none.
	nextEnd := make([]int, len(tags))
	after := make(map[string]int)
	for i := len(tags) - 1; i >= 0; i-- {
		nextEnd[i] = len(tags)
		if j, ok := after["end"+tags[i].name]; ok {
			nextEnd[i] = j
		}
		after[tags[i].name] = i
	}

	var inserts []sourceTag // inline uses, each closed right after its tag
	for i := 0; i < len(tags); {
		t := tags[i]
		if names[t.name] {
			if closing := nextEnd[i]; closing < len(tags) && !useBetween(src, tags, i, closing) {
				// A block: its body, up to the end tag, is raw text.
				i = closing + 1
				continue
			}
			inserts = append(inserts, t)
			i++
			continue
		}
		if stop, ok := regionEnd(src, t, len(src)); ok {
			i = firstTagFrom(tags, i+1, stop)
			continue
		}
		i++
	}
	if len(inserts) == 0 {
		return src
	}

	out := make([]byte, 0, len(src)+len(inserts)*16)
	last := 0
	for _, t := range inserts {
		// A use's "-%}" still trims the text after the added end tag.
		out = append(out, src[last:t.end]...)
		out = append(out, "{% end"+t.name+" %}"...)
		last = t.end
	}
	out = append(out, src[last:]...)
	return string(out)
}

// useBetween reports whether another use of tags[i]'s shortcode comes
// between it and tags[closing], outside the {% raw %}, {% comment %} and
// {% doc %} regions that end before tags[closing].
func useBetween(src string, tags []sourceTag, i, closing int) bool {
	for k := i + 1; k < closing; {
		if tags[k].name == tags[i].name {
			return true
		}
		if stop, ok := regionEnd(src, tags[k], tags[closing].start); ok {
			k = firstTagFrom(tags, k+1, stop)
			continue
		}
		k++
	}
	return false
}

// regionEnd returns the index just after the end tag of the {% raw %},
// {% comment %} or {% doc %} region t opens, when that end tag ends by
// limit.
func regionEnd(src string, t sourceTag, limit int) (int, bool) {
	endTag, ok := textRegions[t.name]
	if !ok || t.end > limit {
		return 0, false
	}
	loc := endTag.FindStringIndex(src[t.end:limit])
	if loc == nil {
		return 0, false
	}
	return t.end + loc[1], true
}

// firstTagFrom returns the index of the first tag at or after from that
// starts at or after pos.
func firstTagFrom(tags []sourceTag, from, pos int) int {
	for from < len(tags) && tags[from].start < pos {
		from++
	}
	return from
}

// scanTags returns the tags of src in order. A tag runs from "{%" to the
// first "%}" after it.
func scanTags(src string) []sourceTag {
	var tags []sourceTag
	pos := 0
	for {
		start := strings.Index(src[pos:], "{%")
		if start < 0 {
			return tags
		}
		start += pos
		closeAt := strings.Index(src[start+2:], "%}")
		if closeAt < 0 {
			return tags
		}
		end := start + 2 + closeAt + 2
		pos = end
		if m := tagName.FindStringSubmatch(src[start:end]); m != nil {
			tags = append(tags, sourceTag{start: start, end: end, name: m[1]})
		}
	}
}
