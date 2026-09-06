package sqlb

// Limits ported verbatim from src/constants.ts.
//
// Lengths are counted in UTF-16 code units, not bytes, because the reference
// compares against JavaScript's String.length. See lengthUTF16.
const (
	// MaxQueryLength bounds the assembled query. src/constants.ts:6.
	MaxQueryLength = 102400
	// MaxIdentifierLength bounds one identifier part. src/constants.ts:11.
	MaxIdentifierLength = 255
	// MaxPatternLength bounds a LIKE/ILIKE/REGEXP pattern. src/constants.ts:17.
	MaxPatternLength = 1024
	// MaxNestingDepth bounds compileFilter recursion. src/filter.ts:17.
	MaxNestingDepth = 10
	// MaxOperators bounds the operator count in one filter. src/filter.ts:18.
	MaxOperators = 100
)

// sqlText is the type of [Q]'s first argument.
//
// It is unexported on purpose. Go assigns an untyped string constant to it
// happily, but a value of type string needs an explicit conversion — and the
// type cannot be named from outside this package, so that conversion cannot be
// written. The practical effect is that only a literal, developer-authored
// constant can reach the code channel of a query. The TypeScript gets the same
// property from tagged templates; here the compiler enforces it.
type sqlText string

// Fragment is a piece of SQL text together with the values bound to its
// placeholders. It is the Go equivalent of the branded, frozen SqlFragment in
// src/fragment.ts, and of SqlQuery — [Q] returns a Fragment too.
//
// A Fragment carrying text can only be produced by this package: every field is
// unexported, so a caller cannot forge one carrying attacker-chosen SQL. That
// replaces the WeakSet registry at src/fragment.ts:17 with a compile-time
// guarantee.
//
// The zero Fragment is empty and valid: it contributes no text and no values.
type Fragment struct {
	text   string
	values []any
	err    error
}

// Text returns the fragment's SQL text. A fragment that failed to build has no
// text; check [Fragment.Err].
func (f Fragment) Text() string { return f.text }

// Values returns the values bound to the fragment's placeholders, in order.
//
// The returned slice aliases the fragment's own storage and must not be
// modified. The reference freezes it (src/fragment.ts:29-32); copying on every
// call would allocate on the hot path to protect a property that cannot affect
// SQL safety, since values are always bound and never spliced into text.
func (f Fragment) Values() []any { return f.values }

// IsEmpty reports that the fragment carries no SQL text.
//
// It is how a caller tells "this filter had no conditions" from "this filter
// compiled to something", since [CompileFilter] on a filter with no conditions
// returns an empty fragment and no error (D-0032). Splicing an empty fragment
// into `... WHERE ?` produces a statement ending in a bare WHERE, which the
// database will reject; on a DELETE or an UPDATE, dropping the WHERE instead
// affects every row. Decide which you want, at the call site:
//
//	where, err := sqlb.CompileFilter(filter)
//	if err != nil {
//	    return err
//	}
//	if where.IsEmpty() {
//	    // no conditions: return the whole table, or refuse — your call
//	}
//
// A fragment that FAILED to build is also empty, because a failed fragment
// carries no text. Check [Fragment.Err] first: `if !f.IsEmpty()` on an unchecked
// failure silently drops the WHERE clause, which is the same outage from the
// other direction.
func (f Fragment) IsEmpty() bool { return f.text == "" }

// Err reports whether the fragment failed to build.
//
// Fragment constructors return a Fragment rather than (Fragment, error) so that
// they compose inside a [Q] call. A failed fragment carries no text and no
// values, and [Q], [Join], [JoinFragment] and [Idents] all propagate the first
// error they meet — so a failure can never contribute partial SQL. The
// reference throws from these helpers immediately; the difference is visible
// only if a caller builds a fragment and never uses it.
func (f Fragment) Err() error { return f.err }

// undefinedT exists so a filter can carry JavaScript's undefined, which Go's
// nil cannot express: src/filter.ts:353-359 drops a field whose value is
// undefined, while src/filter.ts:126 compiles a null one to IS NULL.
type undefinedT uint8

// Undefined is the value that means JavaScript's undefined. A field set to
// Undefined is skipped; a field set to nil compiles to IS NULL.
const Undefined undefinedT = 0

// Filter is an ordered set of filter entries — the Go stand-in for the plain
// object compileFilter() walks with Object.entries (src/filter.ts:353).
//
// Order is part of the contract, not a detail: it decides the order of the
// emitted clauses and therefore of the bound values. A Go map cannot express
// it, because map iteration is randomised.
//
// Build one with [NewFilter]. The zero Filter is a valid empty filter, but a
// nil *Filter compiles to "Filter must be an object", matching the reference's
// null check.
type Filter struct {
	entries []filterEntry
}

type filterEntry struct {
	key   string
	value any
}

// NewFilter returns an empty filter.
func NewFilter() *Filter { return &Filter{} }

// Eq records a field compared against a single value. A nil value compiles to
// IS NULL; [Undefined] makes the field disappear.
func (f *Filter) Eq(field string, value any) *Filter { return f.Set(field, value) }

// Where records a field with an operator object.
func (f *Filter) Where(field string, ops *Ops) *Filter { return f.Set(field, ops) }

// And records a conjunction of sub-filters. It is always emitted before Or and
// before the plain fields, whatever order the calls were made in — that is the
// reference's clause order (src/filter.ts:314-350) and it decides value order.
func (f *Filter) And(subs ...*Filter) *Filter { return f.Set("and", subs) }

// Or records a disjunction of sub-filters.
func (f *Filter) Or(subs ...*Filter) *Filter { return f.Set("or", subs) }

// Alias records a block whose fields are qualified by a table alias. The
// reference spells these as keys prefixed with "$" (src/filter.ts:375-398).
//
// The alias is emitted UNQUOTED, so it is validated as a bare identifier.
func (f *Filter) Alias(name string, sub *Filter) *Filter { return f.Set("$"+name, sub) }

// Len reports how many entries the filter carries.
func (f *Filter) Len() int { return len(f.entries) }

// Ops is an ordered operator object — the value side of
// `{age: {gte: 18, lt: 65}}`. Order decides clause and value order, as in
// [Filter]. Build one with [NewOps].
type Ops struct {
	entries []opEntry
}

type opEntry struct {
	name  string
	value any
}

// NewOps returns an empty operator object.
func NewOps() *Ops { return &Ops{} }

// Gt records `> value`.
func (o *Ops) Gt(value any) *Ops { return o.Set("gt", value) }

// Gte records `>= value`.
func (o *Ops) Gte(value any) *Ops { return o.Set("gte", value) }

// Lt records `< value`.
func (o *Ops) Lt(value any) *Ops { return o.Set("lt", value) }

// Lte records `<= value`.
func (o *Ops) Lte(value any) *Ops { return o.Set("lte", value) }

// Ne records `<> value`, or IS NOT NULL when value is nil or [Undefined].
func (o *Ops) Ne(value any) *Ops { return o.Set("ne", value) }

// In records `IN (...)`.
func (o *Ops) In(values ...any) *Ops { return o.Set("in", values) }

// Nin records `NOT IN (...)`.
func (o *Ops) Nin(values ...any) *Ops { return o.Set("nin", values) }

// Like records `LIKE pattern`.
func (o *Ops) Like(pattern string) *Ops { return o.Set("like", pattern) }

// ILike records `LIKE pattern COLLATE NOCASE`.
func (o *Ops) ILike(pattern string) *Ops { return o.Set("ilike", pattern) }

// Regex records `REGEXP pattern`.
func (o *Ops) Regex(pattern string) *Ops { return o.Set("regex", pattern) }

// Exists records IS NOT NULL (true) or IS NULL (false). It overrides every
// other operator in the same object regardless of position, matching
// src/filter.ts:179-188.
func (o *Ops) Exists(present bool) *Ops { return o.Set("exists", present) }

// Len reports how many operators the object carries.
func (o *Ops) Len() int { return len(o.entries) }
