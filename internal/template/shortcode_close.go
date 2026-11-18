package template

import (
	"regexp"
	"strings"
)

// tagName reads a Liquid tag's name: {% name ... %}, after any "-"
// whitespace control and spacing. The name is a word, as Liquid's own tag
// pattern has it.
var tagName = regexp.MustCompile(`^\{%-?\s*([A-Za-z_]\w*)`)

// skippedRegions are the Liquid tags whose body is not markup: a shortcode
// written inside them is text, and stays as it is.
var skippedRegions = map[string]string{
	"raw":     "endraw",
	"comment": "endcomment",
	"doc":     "enddoc",
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
	tags := liveTags(scanTags(src))
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

// scanTags returns the tags of src in order. A tag ends at the first "%}"
// after its "{%". When another "{%" comes first, the text before it isn't a
// tag, and scanning starts again there, as liquidgo's raw tag finds its
// {% endraw %} after unfinished tag text.
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
		if inner := strings.Index(src[start+2:end-2], "{%"); inner >= 0 {
			pos = start + 2 + inner
			continue
		}
		if m := tagName.FindStringSubmatch(src[start:end]); m != nil {
			tags = append(tags, sourceTag{end: end, name: m[1]})
		}
		pos = end
	}
}

// liveTags returns the tags outside {% raw %}, {% comment %} and {% doc %}
// regions, with the tags that open and close those regions.
func liveTags(all []sourceTag) []sourceTag {
	live := make([]sourceTag, 0, len(all))
	for i := 0; i < len(all); i++ {
		live = append(live, all[i])
		endName, ok := skippedRegions[all[i].name]
		if !ok {
			continue
		}
		for i++; i < len(all) && all[i].name != endName; i++ {
		}
		if i < len(all) {
			live = append(live, all[i])
		}
	}
	return live
}

func indexOr(m map[string]int, key string, def int) int {
	if v, ok := m[key]; ok {
		return v
	}
	return def
}
