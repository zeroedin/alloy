package template

import "regexp"

// tagPattern finds a Liquid tag and its name: {% name ... %}, with any
// spacing and "-" whitespace control on either side. Group 1 is the name,
// group 2 the "-" of a closing "-%}".
var tagPattern = regexp.MustCompile(`(?s)\{%-?\s*([A-Za-z_]\w*).*?(-?)%\}`)

// skippedRegions are the Liquid tags whose body is not markup: a shortcode
// written inside them is text, and stays as it is.
var skippedRegions = map[string]string{
	"raw":     "endraw",
	"comment": "endcomment",
	"doc":     "enddoc",
}

// sourceTag is one tag of a template's source.
type sourceTag struct {
	start, end int
	name       string
	trimRight  bool // the tag ends with "-%}"
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
// Shortcodes in {% raw %}, {% comment %} and {% doc %} are text, and are
// left as they are.
func closeInlineShortcodes(src string, names map[string]bool) string {
	if len(names) == 0 {
		return src
	}
	tags := scanTags(src)
	if len(tags) == 0 {
		return src
	}

	var inserts []sourceTag // inline uses, each closed right after its tag
	for i := 0; i < len(tags); i++ {
		t := tags[i]
		if end, ok := skippedRegions[t.name]; ok {
			i = nextTagNamed(tags, i+1, end)
			continue
		}
		if !names[t.name] {
			continue
		}
		endName := "end" + t.name
		closing := nextTagNamed(tags, i+1, endName)
		if closing < len(tags) && nextTagNamed(tags[:closing], i+1, t.name) == closing {
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
		out = append(out, src[last:t.end]...)
		// The use's "-%}" trims the text after it; the added end tag
		// carries it, so the text after the end tag is still trimmed.
		if t.trimRight {
			out = append(out, "{% end"+t.name+" -%}"...)
		} else {
			out = append(out, "{% end"+t.name+" %}"...)
		}
		last = t.end
	}
	out = append(out, src[last:]...)
	return string(out)
}

// scanTags returns the tags of src in order.
func scanTags(src string) []sourceTag {
	matches := tagPattern.FindAllStringSubmatchIndex(src, -1)
	tags := make([]sourceTag, len(matches))
	for i, m := range matches {
		tags[i] = sourceTag{
			start:     m[0],
			end:       m[1],
			name:      src[m[2]:m[3]],
			trimRight: m[5] > m[4],
		}
	}
	return tags
}

// nextTagNamed returns the index of the first tag named name at or after
// from, or len(tags) when there is none.
func nextTagNamed(tags []sourceTag, from int, name string) int {
	for i := from; i < len(tags); i++ {
		if tags[i].name == name {
			return i
		}
	}
	return len(tags)
}
