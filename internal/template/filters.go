package template

import (
	"crypto/sha256"
	"crypto/sha512"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"html"
	"math"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/lestrrat-go/strftime"
	"github.com/yuin/goldmark"
	"github.com/zeroedin/alloy/internal/content"
	"github.com/zeroedin/alloy/internal/ordered"
)

var (
	strftimeCache   = make(map[string]*strftime.Strftime)
	strftimeCacheMu sync.RWMutex
)

func getStrftimePattern(format string) (*strftime.Strftime, error) {
	strftimeCacheMu.RLock()
	p, ok := strftimeCache[format]
	strftimeCacheMu.RUnlock()
	if ok {
		return p, nil
	}

	strftimeCacheMu.Lock()
	defer strftimeCacheMu.Unlock()

	if p, ok := strftimeCache[format]; ok {
		return p, nil
	}

	p, err := strftime.New(format)
	if err != nil {
		return nil, err
	}

	strftimeCache[format] = p
	return p, nil
}

var assetSource struct {
	root    string
	subdirs []string
}

func RegisterAssetFilters(projectRoot string, subdirs ...string) {
	assetSource.root = projectRoot
	if len(subdirs) == 0 {
		assetSource.subdirs = []string{"static", "assets", "content"}
	} else {
		assetSource.subdirs = subdirs
	}
}

func resolveAssetFile(relPath string) ([]byte, error) {
	for _, sub := range assetSource.subdirs {
		data, err := os.ReadFile(filepath.Join(assetSource.root, sub, relPath))
		if err == nil {
			return data, nil
		}
	}
	return nil, fmt.Errorf("asset not found: %s", relPath)
}

// RegisterBuiltinFilters registers all Tier 1 built-in filters on the given engine.
func RegisterBuiltinFilters(engine TemplateEngine) error {
	filters := map[string]FilterFunc{
		"slugify":        Slugify,
		"upcase":         Upcase,
		"downcase":       Downcase,
		"capitalize":     Capitalize,
		"truncate":       Truncate,
		"truncatewords":  TruncateWords,
		"strip_html":     StripHTML,
		"escape":         Escape,
		"replace":        Replace,
		"replace_first":  ReplaceFirst,
		"split":          Split,
		"join":           Join,
		"strip":          Strip,
		"append":         Append,
		"prepend":        Prepend,
		"newline_to_br":  NewlineToBr,
		"contains":       Contains,
		"date":           DateFormat,
		"sort":           Sort,
		"reverse":        Reverse,
		"first":          First,
		"last":           Last,
		"where":          Where,
		"group_by":       GroupBy,
		"size":           Size,
		"map":            Map,
		"flatten":        Flatten,
		"uniq":           Uniq,
		"compact":        Compact,
		"concat":         Concat,
		"intersect":      Intersect,
		"union":          Union,
		"complement":     Complement,
		"url":            URLFilter,
		"absolute_url":   AbsoluteURL,
		"url_encode":     URLEncode,
		"url_decode":     URLDecode,
		"plus":           Plus,
		"minus":          Minus,
		"times":          Times,
		"divided_by":     DividedBy,
		"modulo":         Modulo,
		"ceil":           Ceil,
		"floor":          Floor,
		"round":          Round,
		"abs":            Abs,
		"markdownify":    Markdownify,
		"findRE": func(input interface{}, args ...interface{}) interface{} {
			// Liquid convention: input=text, args[0]=pattern
			// Go function: input=pattern, args[0]=text
			if len(args) == 0 {
				return []string{}
			}
			return FindRE(args[0], input)
		},
		"replaceRE": func(input interface{}, args ...interface{}) interface{} {
			// Liquid: input=text, args[0]=pattern, args[1]=replacement
			// Go: input=pattern, args[0]=text, args[1]=replacement
			if len(args) < 2 {
				return toString(input)
			}
			return ReplaceRE(args[0], input, args[1])
		},
		"json":           JSONFilter,
		"default":        Default,
		"fingerprint":    Fingerprint,
		"safeHTML":       SafeHTML,
		"cachebust":      CacheBust,
		"get_hash":       GetHash,
	}
	for name, fn := range filters {
		if err := engine.AddFilter(name, fn); err != nil {
			return err
		}
	}
	return nil
}

// --- String filters ---

func Slugify(input interface{}, args ...interface{}) interface{} {
	s := toString(input)
	if s == "" {
		return ""
	}
	s = strings.ToLower(s)
	// Replace non-alphanumeric characters with hyphens
	reg := regexp.MustCompile(`[^a-z0-9]+`)
	s = reg.ReplaceAllString(s, "-")
	s = strings.Trim(s, "-")
	return s
}

func Upcase(input interface{}, args ...interface{}) interface{} {
	if input == nil {
		return ""
	}
	return strings.ToUpper(toString(input))
}

func Downcase(input interface{}, args ...interface{}) interface{} {
	return strings.ToLower(toString(input))
}

func Capitalize(input interface{}, args ...interface{}) interface{} {
	s := toString(input)
	if len(s) == 0 {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

func Truncate(input interface{}, args ...interface{}) interface{} {
	s := toString(input)
	length := 50
	if len(args) > 0 {
		length = toInt(args[0])
	}
	if len(s) <= length {
		return s
	}
	ellipsis := "..."
	if length <= len(ellipsis) {
		return ellipsis[:length]
	}
	return s[:length-len(ellipsis)] + ellipsis
}

func TruncateWords(input interface{}, args ...interface{}) interface{} {
	s := toString(input)
	count := 15
	if len(args) > 0 {
		count = toInt(args[0])
	}
	words := strings.Fields(s)
	if len(words) <= count {
		return s
	}
	return strings.Join(words[:count], " ") + "..."
}

func StripHTML(input interface{}, args ...interface{}) interface{} {
	s := toString(input)
	reg := regexp.MustCompile(`<[^>]*>`)
	return reg.ReplaceAllString(s, "")
}

func Escape(input interface{}, args ...interface{}) interface{} {
	return html.EscapeString(toString(input))
}

func Replace(input interface{}, args ...interface{}) interface{} {
	s := toString(input)
	if len(args) < 2 {
		return s
	}
	old := toString(args[0])
	new := toString(args[1])
	return strings.ReplaceAll(s, old, new)
}

func ReplaceFirst(input interface{}, args ...interface{}) interface{} {
	s := toString(input)
	if len(args) < 2 {
		return s
	}
	old := toString(args[0])
	new := toString(args[1])
	return strings.Replace(s, old, new, 1)
}

func Split(input interface{}, args ...interface{}) interface{} {
	s := toString(input)
	sep := " "
	if len(args) > 0 {
		sep = toString(args[0])
	}
	return strings.Split(s, sep)
}

func Join(input interface{}, args ...interface{}) interface{} {
	sep := " "
	if len(args) > 0 {
		sep = toString(args[0])
	}
	arr := toSlice(input)
	strs := make([]string, len(arr))
	for i, v := range arr {
		strs[i] = toString(v)
	}
	return strings.Join(strs, sep)
}

func Strip(input interface{}, args ...interface{}) interface{} {
	return strings.TrimSpace(toString(input))
}

func Append(input interface{}, args ...interface{}) interface{} {
	s := toString(input)
	if len(args) > 0 {
		s += toString(args[0])
	}
	return s
}

func Prepend(input interface{}, args ...interface{}) interface{} {
	s := toString(input)
	if len(args) > 0 {
		s = toString(args[0]) + s
	}
	return s
}

func NewlineToBr(input interface{}, args ...interface{}) interface{} {
	return strings.ReplaceAll(toString(input), "\n", "<br>\n")
}

func Contains(input interface{}, args ...interface{}) interface{} {
	s := toString(input)
	if len(args) == 0 {
		return false
	}
	return strings.Contains(s, toString(args[0]))
}

// --- Date filters ---

func DateFormat(input interface{}, args ...interface{}) interface{} {
	if len(args) == 0 {
		return toString(input)
	}
	format := toString(args[0])

	var t time.Time
	switch v := input.(type) {
	case time.Time:
		t = v
	case string:
		var err error
		layouts := []string{
			"2006-01-02T15:04:05Z07:00",
			"2006-01-02 15:04:05",
			"2006-01-02",
			time.RFC3339,
		}
		for _, layout := range layouts {
			t, err = time.Parse(layout, v)
			if err == nil {
				break
			}
		}
		if err != nil {
			return v
		}
	default:
		return toString(input)
	}

	p, err := getStrftimePattern(format)
	if err != nil {
		return format
	}
	return p.FormatString(t)
}

// --- Array filters ---

func Sort(input interface{}, args ...interface{}) interface{} {
	arr := toSlice(input)
	if len(arr) == 0 {
		return arr
	}
	result := make([]interface{}, len(arr))
	copy(result, arr)

	if len(args) > 0 {
		key := toString(args[0])
		sort.SliceStable(result, func(i, j int) bool {
			a := getMapValue(result[i], key)
			b := getMapValue(result[j], key)
			return compareValues(a, b)
		})
	} else {
		sort.SliceStable(result, func(i, j int) bool {
			return compareValues(result[i], result[j])
		})
	}
	return result
}

func Reverse(input interface{}, args ...interface{}) interface{} {
	arr := toSlice(input)
	result := make([]interface{}, len(arr))
	for i, v := range arr {
		result[len(arr)-1-i] = v
	}
	return result
}

func First(input interface{}, args ...interface{}) interface{} {
	arr := toSlice(input)
	if len(arr) == 0 {
		return nil
	}
	return arr[0]
}

func Last(input interface{}, args ...interface{}) interface{} {
	arr := toSlice(input)
	if len(arr) == 0 {
		return nil
	}
	return arr[len(arr)-1]
}

func Where(input interface{}, args ...interface{}) interface{} {
	arr := toSlice(input)
	if len(args) < 2 {
		return arr
	}
	key := toString(args[0])
	val := args[1]

	var result []interface{}
	for _, item := range arr {
		v := getMapValue(item, key)
		if v == val {
			result = append(result, item)
		}
	}
	return result
}

func GroupBy(input interface{}, args ...interface{}) interface{} {
	arr := toSlice(input)
	if len(args) == 0 {
		return nil
	}
	key := toString(args[0])

	groups := make(map[string]interface{})
	for _, item := range arr {
		val := toString(getMapValue(item, key))
		existing, ok := groups[val]
		if ok {
			groups[val] = append(existing.([]interface{}), item)
		} else {
			groups[val] = []interface{}{item}
		}
	}
	return groups
}

func Size(input interface{}, args ...interface{}) interface{} {
	arr := toSlice(input)
	return len(arr)
}

func Map(input interface{}, args ...interface{}) interface{} {
	arr := toSlice(input)
	if len(args) == 0 {
		return arr
	}
	key := toString(args[0])
	result := make([]interface{}, len(arr))
	for i, item := range arr {
		result[i] = getMapValue(item, key)
	}
	return result
}

func Flatten(input interface{}, args ...interface{}) interface{} {
	if input == nil {
		return nil
	}
	arr := toSlice(input)
	var result []interface{}
	for _, item := range arr {
		if sub, ok := item.([]interface{}); ok {
			result = append(result, sub...)
		} else {
			result = append(result, item)
		}
	}
	return result
}

func Uniq(input interface{}, args ...interface{}) interface{} {
	arr := toSlice(input)
	seen := make(map[interface{}]bool)
	var result []interface{}
	for _, v := range arr {
		if !seen[v] {
			seen[v] = true
			result = append(result, v)
		}
	}
	return result
}

func Compact(input interface{}, args ...interface{}) interface{} {
	arr := toSlice(input)
	var result []interface{}
	for _, v := range arr {
		if v != nil {
			result = append(result, v)
		}
	}
	return result
}

func Concat(input interface{}, args ...interface{}) interface{} {
	arr := toSlice(input)
	if len(args) > 0 {
		other := toSlice(args[0])
		result := make([]interface{}, 0, len(arr)+len(other))
		result = append(result, arr...)
		result = append(result, other...)
		return result
	}
	return arr
}

// --- Set operation filters ---

func Intersect(input interface{}, args ...interface{}) interface{} {
	a := toSlice(input)
	if len(args) == 0 {
		return a
	}
	b := toSlice(args[0])
	bSet := make(map[interface{}]bool)
	for _, v := range b {
		bSet[v] = true
	}
	var result []interface{}
	for _, v := range a {
		if bSet[v] {
			result = append(result, v)
		}
	}
	return result
}

func Union(input interface{}, args ...interface{}) interface{} {
	a := toSlice(input)
	if len(args) == 0 {
		return a
	}
	b := toSlice(args[0])
	seen := make(map[interface{}]bool)
	var result []interface{}
	for _, v := range a {
		if !seen[v] {
			seen[v] = true
			result = append(result, v)
		}
	}
	for _, v := range b {
		if !seen[v] {
			seen[v] = true
			result = append(result, v)
		}
	}
	return result
}

func Complement(input interface{}, args ...interface{}) interface{} {
	a := toSlice(input)
	if len(args) == 0 {
		return a
	}
	b := toSlice(args[0])
	bSet := make(map[interface{}]bool)
	for _, v := range b {
		bSet[v] = true
	}
	var result []interface{}
	for _, v := range a {
		if !bSet[v] {
			result = append(result, v)
		}
	}
	return result
}

// --- URL filters ---

func URLFilter(input interface{}, args ...interface{}) interface{} {
	s := toString(input)
	if !strings.HasPrefix(s, "/") {
		s = "/" + s
	}
	return s
}

func AbsoluteURL(input interface{}, args ...interface{}) interface{} {
	s := toString(input)
	if strings.HasPrefix(s, "http://") || strings.HasPrefix(s, "https://") {
		return s
	}
	if len(args) > 0 {
		base := strings.TrimSuffix(toString(args[0]), "/")
		return base + s
	}
	return s
}

func URLEncode(input interface{}, args ...interface{}) interface{} {
	return url.PathEscape(toString(input))
}

func URLDecode(input interface{}, args ...interface{}) interface{} {
	s, err := url.PathUnescape(toString(input))
	if err != nil {
		return toString(input)
	}
	return s
}

// --- Math filters ---

func Plus(input interface{}, args ...interface{}) interface{} {
	if len(args) == 0 {
		return toInt(input)
	}
	return toInt(input) + toInt(args[0])
}

func Minus(input interface{}, args ...interface{}) interface{} {
	if len(args) == 0 {
		return toInt(input)
	}
	return toInt(input) - toInt(args[0])
}

func Times(input interface{}, args ...interface{}) interface{} {
	if len(args) == 0 {
		return toInt(input)
	}
	return toInt(input) * toInt(args[0])
}

func DividedBy(input interface{}, args ...interface{}) interface{} {
	if len(args) == 0 {
		return toInt(input)
	}
	b := toInt(args[0])
	if b == 0 {
		return 0
	}
	return toInt(input) / b
}

func Modulo(input interface{}, args ...interface{}) interface{} {
	if len(args) == 0 {
		return toInt(input)
	}
	b := toInt(args[0])
	if b == 0 {
		return 0
	}
	return toInt(input) % b
}

func Ceil(input interface{}, args ...interface{}) interface{} {
	return int(math.Ceil(toFloat(input)))
}

func Floor(input interface{}, args ...interface{}) interface{} {
	return int(math.Floor(toFloat(input)))
}

func Round(input interface{}, args ...interface{}) interface{} {
	return int(math.Round(toFloat(input)))
}

func Abs(input interface{}, args ...interface{}) interface{} {
	v := toInt(input)
	if v < 0 {
		return -v
	}
	return v
}

// --- Content filters ---

var markdownifyMD goldmark.Markdown
var markdownifyOnce sync.Once

// InitMarkdownify creates the shared goldmark instance for the markdownify
// filter using config-driven options. Called from createEngine after config
// is loaded. TemplateTags is always false: markdownify processes values that
// have already been through template rendering, so tag protection is not needed.
func InitMarkdownify(opts content.MarkdownOptions) {
	opts.TemplateTags = false
	opts.Hooks = nil
	opts.HookRenderer = nil
	markdownifyMD = content.CreateGoldmark(opts)
}

func Markdownify(input interface{}, args ...interface{}) interface{} {
	if markdownifyMD == nil {
		markdownifyOnce.Do(func() {
			InitMarkdownify(content.MarkdownOptions{
				Unsafe:        true,
				Typographer:   true,
				AutoHeadingID: true,
			})
		})
	}
	s := toString(input)
	var buf strings.Builder
	if err := markdownifyMD.Convert([]byte(s), &buf); err != nil {
		return s
	}
	return buf.String()
}

// --- Regex filters ---

func FindRE(input interface{}, args ...interface{}) interface{} {
	pattern := toString(input)
	if len(args) == 0 {
		return []string{}
	}
	text := toString(args[0])
	re, err := regexp.Compile(pattern)
	if err != nil {
		return []string{}
	}
	return re.FindAllString(text, -1)
}

func ReplaceRE(input interface{}, args ...interface{}) interface{} {
	pattern := toString(input)
	if len(args) < 2 {
		return toString(input)
	}
	text := toString(args[0])
	replacement := toString(args[1])
	re, err := regexp.Compile(pattern)
	if err != nil {
		return text
	}
	return re.ReplaceAllString(text, replacement)
}

// --- Data filters ---

func JSONFilter(input interface{}, args ...interface{}) interface{} {
	b, err := jsonCodec.Marshal(input)
	if err != nil {
		return "{}"
	}
	return string(b)
}

func Default(input interface{}, args ...interface{}) interface{} {
	if input == nil || input == "" {
		if len(args) > 0 {
			return args[0]
		}
		return nil
	}
	return input
}

// --- Asset filters ---

func Fingerprint(input interface{}, args ...interface{}) interface{} {
	s := toString(input)
	h := sha256.Sum256([]byte(s))
	hash := hex.EncodeToString(h[:8])
	// Insert hash before the extension
	lastDot := strings.LastIndex(s, ".")
	if lastDot >= 0 {
		return s[:lastDot] + "." + hash + s[lastDot:]
	}
	return s + "." + hash
}

// --- Output safety filters ---

func SafeHTML(input interface{}, args ...interface{}) interface{} {
	return toString(input)
}

// --- Asset fingerprinting filters ---

func CacheBust(input interface{}, args ...interface{}) interface{} {
	path := strings.TrimLeft(toString(input), "/")
	data, err := resolveAssetFile(path)
	if err != nil {
		return "/" + path
	}
	h := sha256.Sum256(data)
	return "/" + path + "?h=" + hex.EncodeToString(h[:])[:12]
}

func GetHash(input interface{}, args ...interface{}) interface{} {
	path := strings.TrimLeft(toString(input), "/")
	data, err := resolveAssetFile(path)
	if err != nil {
		return ""
	}
	shaType := 256
	useBase64 := true
	if len(args) >= 1 {
		shaType = toInt(args[0])
	}
	if len(args) >= 2 {
		switch v := args[1].(type) {
		case bool:
			useBase64 = v
		default:
			useBase64 = toString(args[1]) != "false"
		}
	}
	var digest []byte
	switch shaType {
	case 384:
		h := sha512.Sum384(data)
		digest = h[:]
	case 512:
		h := sha512.Sum512(data)
		digest = h[:]
	default:
		h := sha256.Sum256(data)
		digest = h[:]
	}
	if useBase64 {
		return base64.StdEncoding.EncodeToString(digest)
	}
	return hex.EncodeToString(digest)
}

var builtinFilters = map[string]FilterFunc{
	"slugify":       Slugify,
	"upcase":        Upcase,
	"downcase":      Downcase,
	"capitalize":    Capitalize,
	"truncate":      Truncate,
	"truncatewords": TruncateWords,
	"strip_html":    StripHTML,
	"escape":        Escape,
	"replace":       Replace,
	"replace_first": ReplaceFirst,
	"split":         Split,
	"join":          Join,
	"strip":         Strip,
	"append":        Append,
	"prepend":       Prepend,
	"newline_to_br": NewlineToBr,
	"contains":      Contains,
	"date":          DateFormat,
	"sort":          Sort,
	"reverse":       Reverse,
	"first":         First,
	"last":          Last,
	"where":         Where,
	"group_by":      GroupBy,
	"size":          Size,
	"map":           Map,
	"flatten":       Flatten,
	"uniq":          Uniq,
	"compact":       Compact,
	"concat":        Concat,
	"intersect":     Intersect,
	"union":         Union,
	"complement":    Complement,
	"url":           URLFilter,
	"absolute_url":  AbsoluteURL,
	"url_encode":    URLEncode,
	"url_decode":    URLDecode,
	"plus":          Plus,
	"minus":         Minus,
	"times":         Times,
	"divided_by":    DividedBy,
	"modulo":        Modulo,
	"ceil":          Ceil,
	"floor":         Floor,
	"round":         Round,
	"abs":           Abs,
	"markdownify":   Markdownify,
	"findRE":        FindRE,
	"replaceRE":     ReplaceRE,
	"json":          JSONFilter,
	"default":       Default,
	"fingerprint":   Fingerprint,
	"safeHTML":      SafeHTML,
	"cachebust":     CacheBust,
	"get_hash":      GetHash,
}

// ApplyFilter dispatches a filter by name with the given input and optional arguments.
func ApplyFilter(name string, input interface{}, args ...interface{}) interface{} {
	fn, ok := builtinFilters[name]
	if !ok {
		return nil
	}
	return fn(input, args...)
}

// IsBuiltinFilter reports whether name is a known built-in filter.
func IsBuiltinFilter(name string) bool {
	_, ok := builtinFilters[name]
	return ok
}

// --- Helper functions ---

func compareValues(a, b interface{}) bool {
	if a == nil && b == nil {
		return false
	}
	if a == nil {
		return false
	}
	if b == nil {
		return true
	}
	aNum, aOk := toFloat64(a)
	bNum, bOk := toFloat64(b)
	if aOk && bOk {
		return aNum < bNum
	}
	return toString(a) < toString(b)
}

func toFloat64(v interface{}) (float64, bool) {
	switch n := v.(type) {
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	case float64:
		return n, true
	case float32:
		return float64(n), true
	case string:
		if len(n) == 0 {
			return 0, false
		}
		for _, c := range n {
			if c < '0' || c > '9' {
				return 0, false
			}
		}
		i, err := strconv.Atoi(n)
		return float64(i), err == nil
	default:
		return 0, false
	}
}

func toString(v interface{}) string {
	if v == nil {
		return ""
	}
	return fmt.Sprintf("%v", v)
}

func toInt(v interface{}) int {
	switch n := v.(type) {
	case int:
		return n
	case int64:
		return int(n)
	case float64:
		return int(n)
	case float32:
		return int(n)
	case string:
		return 0
	default:
		return 0
	}
}

func toFloat(v interface{}) float64 {
	switch n := v.(type) {
	case float64:
		return n
	case float32:
		return float64(n)
	case int:
		return float64(n)
	case int64:
		return float64(n)
	default:
		return 0
	}
}

func toSlice(v interface{}) []interface{} {
	if v == nil {
		return nil
	}
	if arr, ok := v.([]interface{}); ok {
		return arr
	}
	return nil
}

func getMapValue(item interface{}, key string) interface{} {
	if m, ok := item.(map[string]interface{}); ok {
		return m[key]
	}
	if m, ok := item.(*ordered.Map); ok {
		return m.Get(key)
	}
	return nil
}
