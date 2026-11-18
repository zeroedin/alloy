package template

import (
	"regexp"
	"strings"

	"github.com/Notifuse/liquidgo/v5/liquid"
)

// tagName reads a Liquid tag's name: {% name ... %}, after any "-"
// whitespace control and spacing. The name is a word, as Liquid's own tag
// pattern has it.
var tagName = regexp.MustCompile(`^\{%-?\s*([A-Za-z_]\w*)`)

// rawEnds are the end tags of {% raw %} and {% doc %}, the regions that end
// at their first end tag. A {% comment %} counts the comments nested in it.
var rawEnds = map[string]string{"raw": "endraw", "doc": "enddoc"}

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
//     text, and is left as it is. It runs to its end tag as liquidgo's tag
//     finds it (see regionEnds): by name, whatever follows the name
//     ({% endraw ignored %}), and for a comment, the endcomment that
//     matches it, past the comments nested in it.
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
	ends := regionEnds(src, tags)

	var inserts []sourceTag // inline uses, each closed right after its tag
	for i := 0; i < len(tags); {
		t := tags[i]
		if names[t.name] {
			if closing := nextEnd[i]; closing < len(tags) && !useBetween(tags, ends, i, closing) {
				// A block: its body, up to the end tag, is raw text.
				i = closing + 1
				continue
			}
			inserts = append(inserts, t)
			i++
			continue
		}
		if stop := ends[i]; stop >= 0 {
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
func useBetween(tags []sourceTag, ends []int, i, closing int) bool {
	for k := i + 1; k < closing; {
		if tags[k].name == tags[i].name {
			return true
		}
		if stop := ends[k]; stop >= 0 && stop <= tags[closing].start {
			k = firstTagFrom(tags, k+1, stop)
			continue
		}
		k++
	}
	return false
}

// regionEnds returns, for each of tags, the index just after the end tag
// of the {% raw %}, {% comment %} or {% doc %} region it opens, or -1 when
// it opens none or the region has no end tag. It reads src with liquidgo's
// tokenizer, and ends each region as liquidgo's tag does:
//   - raw and doc at the first token whose last tag is named endraw or
//     enddoc, whatever follows the name ({% endraw ignored %}) or comes
//     before the tag ({% oops {% endraw %});
//   - comment at the endcomment that matches it, counting the comments
//     nested in it and skipping the body of a raw in it, as liquidgo's
//     comment tag reads the tags at the start of its tokens.
func regionEnds(src string, tags []sourceTag) []int {
	ends := make([]int, len(tags))
	var tz *tokenList
	for i, t := range tags {
		ends[i] = -1
		if t.name != "comment" && rawEnds[t.name] == "" {
			continue
		}
		if tz == nil {
			tz = newTokenList(src)
		}
		// A tag that doesn't start a token, inside {{ ... {% raw %},
		// opens no region for liquidgo either.
		k, ok := tz.at[t.start]
		if !ok {
			continue
		}
		var j int
		if t.name == "comment" {
			j = tz.commentEnd(k)
		} else {
			j = tz.next(rawEnds[t.name])[k+1]
		}
		if j < len(tz.tokens) {
			ends[i] = tz.offset[j] + len(tz.tokens[j])
		}
	}
	return ends
}

// tokenList is src split into liquidgo's tokens, with what regionEnds
// needs to find each region's end once.
type tokenList struct {
	tokens []string
	offset []int       // each token's offset in src
	at     map[int]int // a token's index by its offset
	// nexts["endraw"][k] is the index of the first token at or after k
	// whose last tag is endraw, or len(tokens): see next.
	nexts map[string][]int
	// bodyEnds[j]-1 is the index of the endcomment that ends a comment
	// body starting at token j, or len(tokens); 0 when not worked out yet:
	// see commentEnd.
	bodyEnds []int
}

func newTokenList(src string) *tokenList {
	tz := &tokenList{
		at:    make(map[int]int),
		nexts: make(map[string][]int),
	}
	tokenizer := liquid.NewTokenizer(src, nil, false, nil, false)
	pos := 0
	for !tokenizer.Exhausted() {
		token := tokenizer.Shift()
		tz.at[pos] = len(tz.tokens)
		tz.tokens = append(tz.tokens, token)
		tz.offset = append(tz.offset, pos)
		pos += len(token)
	}
	tz.bodyEnds = make([]int, len(tz.tokens))
	return tz
}

// next returns, for each token index k, the index of the first token at or
// after k whose last tag is named end, as liquidgo's raw and doc tags find
// their end tag, or len(tokens). Index len(tokens) is there too.
func (tz *tokenList) next(end string) []int {
	if n, ok := tz.nexts[end]; ok {
		return n
	}
	n := make([]int, len(tz.tokens)+1)
	n[len(tz.tokens)] = len(tz.tokens)
	for k := len(tz.tokens) - 1; k >= 0; k-- {
		n[k] = n[k+1]
		if _, name, ok := liquid.MatchTagPossiblyInvalid(tz.tokens[k]); ok && name == end {
			n[k] = k
		}
	}
	tz.nexts[end] = n
	return n
}

// commentEnd returns the index of the endcomment token that closes the
// comment token k opens, or len(tokens), reading the tokens after k as
// liquidgo's comment tag does: a nested comment is read to its own end, and
// the body of a raw to its endraw.
//
// Where a comment body starting at a token ends depends only on that token,
// so each token's answer is recorded and worked out once, whichever comment
// reaches it: the tokens read on the way share the answer of the token they
// lead to.
func (tz *tokenList) commentEnd(k int) int {
	n := len(tz.tokens)
	var path []int    // tokens whose body end is the one j leads to
	var outer [][]int // the paths of enclosing bodies, each waiting for a nested comment's end
	for j := k + 1; ; {
		if j >= n || tz.bodyEnds[j] != 0 {
			end := n
			if j < n {
				end = tz.bodyEnds[j] - 1
			}
			for _, p := range path {
				tz.bodyEnds[p] = end + 1
			}
			if len(outer) == 0 {
				return end
			}
			// A nested comment ended: its enclosing body goes on after it.
			path, outer = outer[len(outer)-1], outer[:len(outer)-1]
			j = end + 1
			continue
		}
		switch commentTagName(tz.tokens[j]) {
		case "endcomment":
			tz.bodyEnds[j] = j + 1
		case "comment":
			outer = append(outer, append(path, j))
			path = nil
			j++
		case "raw":
			path = append(path, j)
			j = tz.next("endraw")[j+1] + 1
		default:
			path = append(path, j)
			j++
		}
	}
}

// commentTagName returns the name of the tag token starts with, as
// liquidgo's comment tag reads the tokens of its body, or "".
func commentTagName(token string) string {
	if !strings.HasSuffix(token, "%}") {
		return ""
	}
	if m := tagName.FindStringSubmatch(token); m != nil {
		return m[1]
	}
	return ""
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
