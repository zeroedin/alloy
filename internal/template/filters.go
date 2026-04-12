package template

// RegisterBuiltinFilters registers all Tier 1 built-in filters on the given engine.
func RegisterBuiltinFilters(engine TemplateEngine) error {
	return ErrNotImplemented
}

// --- String filters ---

func Slugify(input interface{}, args ...interface{}) interface{}      { return nil }
func Upcase(input interface{}, args ...interface{}) interface{}       { return nil }
func Downcase(input interface{}, args ...interface{}) interface{}     { return nil }
func Capitalize(input interface{}, args ...interface{}) interface{}   { return nil }
func Truncate(input interface{}, args ...interface{}) interface{}     { return nil }
func TruncateWords(input interface{}, args ...interface{}) interface{} { return nil }
func StripHTML(input interface{}, args ...interface{}) interface{}    { return nil }
func Escape(input interface{}, args ...interface{}) interface{}      { return nil }
func Replace(input interface{}, args ...interface{}) interface{}     { return nil }
func ReplaceFirst(input interface{}, args ...interface{}) interface{} { return nil }
func Split(input interface{}, args ...interface{}) interface{}       { return nil }
func Join(input interface{}, args ...interface{}) interface{}        { return nil }
func Strip(input interface{}, args ...interface{}) interface{}       { return nil }
func Append(input interface{}, args ...interface{}) interface{}      { return nil }
func Prepend(input interface{}, args ...interface{}) interface{}     { return nil }
func NewlineToBr(input interface{}, args ...interface{}) interface{} { return nil }
func Contains(input interface{}, args ...interface{}) interface{}    { return nil }

// --- Date filters ---

func DateFormat(input interface{}, args ...interface{}) interface{} { return nil }

// --- Array filters ---

func Sort(input interface{}, args ...interface{}) interface{}    { return nil }
func Reverse(input interface{}, args ...interface{}) interface{} { return nil }
func First(input interface{}, args ...interface{}) interface{}   { return nil }
func Last(input interface{}, args ...interface{}) interface{}    { return nil }
func Where(input interface{}, args ...interface{}) interface{}   { return nil }
func GroupBy(input interface{}, args ...interface{}) interface{} { return nil }
func Size(input interface{}, args ...interface{}) interface{}    { return nil }
func Map(input interface{}, args ...interface{}) interface{}     { return nil }
func Uniq(input interface{}, args ...interface{}) interface{}    { return nil }
func Compact(input interface{}, args ...interface{}) interface{} { return nil }
func Concat(input interface{}, args ...interface{}) interface{}  { return nil }

// --- Set operation filters ---

func Intersect(input interface{}, args ...interface{}) interface{}  { return nil }
func Union(input interface{}, args ...interface{}) interface{}      { return nil }
func Complement(input interface{}, args ...interface{}) interface{} { return nil }

// --- URL filters ---

func URLFilter(input interface{}, args ...interface{}) interface{}    { return nil }
func AbsoluteURL(input interface{}, args ...interface{}) interface{}  { return nil }
func URLEncode(input interface{}, args ...interface{}) interface{}    { return nil }
func URLDecode(input interface{}, args ...interface{}) interface{}    { return nil }

// --- Math filters ---

func Plus(input interface{}, args ...interface{}) interface{}      { return nil }
func Minus(input interface{}, args ...interface{}) interface{}     { return nil }
func Times(input interface{}, args ...interface{}) interface{}     { return nil }
func DividedBy(input interface{}, args ...interface{}) interface{} { return nil }
func Modulo(input interface{}, args ...interface{}) interface{}    { return nil }
func Ceil(input interface{}, args ...interface{}) interface{}      { return nil }
func Floor(input interface{}, args ...interface{}) interface{}     { return nil }
func Round(input interface{}, args ...interface{}) interface{}     { return nil }
func Abs(input interface{}, args ...interface{}) interface{}       { return nil }

// --- Content filters ---

func Markdownify(input interface{}, args ...interface{}) interface{} { return nil }

// --- Regex filters ---

func FindRE(input interface{}, args ...interface{}) interface{}    { return nil }
func ReplaceRE(input interface{}, args ...interface{}) interface{} { return nil }

// --- Data filters ---

func JSONFilter(input interface{}, args ...interface{}) interface{} { return nil }
func Default(input interface{}, args ...interface{}) interface{}    { return nil }

// --- Asset filters ---

func Fingerprint(input interface{}, args ...interface{}) interface{} { return nil }

// --- Output safety filters ---

func SafeHTML(input interface{}, args ...interface{}) interface{} { return nil }

// ApplyFilter dispatches a filter by name with the given input and optional arguments.
// This allows table-driven tests to exercise filters generically.
func ApplyFilter(name string, input interface{}, args ...interface{}) interface{} {
	return nil
}
