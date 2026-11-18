package template

import (
	"regexp"
	"strings"
)

// tagName reads a Liquid tag's name: {% name ... %}, after any "-"
// whitespace control and spacing. The name is a word, as Liquid's own tag
// pattern has it.
var tagName = regexp.MustCompile(`^\{%-?\s*([A-Za-z_]\w*)`)

// skippedRegions are the Liquid tags whose body is not markup, with the
// pattern of their end tag: a shortcode written inside them is text, and
// stays as it is.
var skippedRegions = map[string]*regexp.Regexp{
	"raw":     regexp.MustCompile(`\{%-?\s*endraw\s*-?%\}`),
	"comment": regexp.MustCompile(`\{%-?\s*endcomment\s*-?%\}`),
	"doc":     regexp.MustCompile(`\{%-?\s*enddoc\s*-?%\}`),
}

// sourceTag is one tag of a template's source.
type sourceTag struct {
	end  int // the index just after the tag's %}
	name string
}

// closeInlineShortcodes returns src with an {% endname %} added after each
// use of a shortcode in names that src doesn't close, so that every use can
// be parsed as a block (issue #1367). Each template decides for itself,
// partials included.
//
// An {% endname %} closes the nearest open use of the same name before it,
// as the block tag parses: a block's body runs to the first {% endname %}
// after it, and reaches the shortcode as raw text, so it is left as written.
// A use with another use of the same name before that end tag is inline.
// Shortcodes in {% raw %}, {% comment %} and {% doc %} are text: they are
// left as they are, and take no part in pairing.
func closeInlineShortcodes(src string, names map[string]bool) string {
	if len(names) == 0 {
		return src
	}
	tags := scanTags(src)
	if len(tags) == 0 {
		return src
	}

	// next[i] is the index of the next tag named like tags[i], and
	// nextEnd[i] the index of the next "end"+name tag; len(tags) if none.
	next := make([]int, len(tags))
	nextEnd := make([]int, len(tags))
	after := make(map[string]int)
	for i := len(tags) - 1; i >= 0; i-- {
		name := tags[i].name
		next[i] = indexOr(after, name, len(tags))
		nextEnd[i] = indexOr(after, "end"+name, len(tags))
		after[name] = i
	}

	var inserts []sourceTag // inline uses, each closed right after its tag
	for i := 0; i < len(tags); i++ {
		t := tags[i]
		if !names[t.name] {
			continue
		}
		if closing := nextEnd[i]; closing < len(tags) && next[i] > closing {
			// A block: its body, up to the end tag, is raw text.
			i = closing
			continue
		}
		inserts = append(inserts, t)
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

// scanTags returns the tags of src in order, outside {% raw %},
// {% comment %} and {% doc %} regions.
//
// A tag runs from "{%" to the first "%}" after it. A region runs from its
// tag to the first end tag of its kind in the text, as liquidgo finds it,
// even after unfinished tag text ({% raw %}{% oops {% endraw %}). A region
// tag with no end tag after it is kept as an ordinary tag: inside a
// shortcode's body, {% raw %} alone is text, and the body still ends at the
// shortcode's end tag.
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
		m := tagName.FindStringSubmatch(src[start:end])
		if m == nil {
			continue
		}
		if endTag, ok := skippedRegions[m[1]]; ok {
			if loc := endTag.FindStringIndex(src[end:]); loc != nil {
				pos = end + loc[1]
				continue
			}
		}
		tags = append(tags, sourceTag{end: end, name: m[1]})
	}
}

func indexOr(m map[string]int, key string, def int) int {
	if v, ok := m[key]; ok {
		return v
	}
	return def
}
