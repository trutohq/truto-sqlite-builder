package sqlb

import (
	"errors"
	"math"
	"reflect"
	"time"
)

// validOperators mirrors VALID_OPERATORS (src/filter.ts:21).
//
// Note that "and" and "or" are members. Inside an operator object they pass
// validation, are counted against the operator budget, and then match no case
// in the reference's switch — so an object containing only them fails with
// "Operator object must contain at least one valid operator". That is the
// reference's behaviour and is reproduced rather than tidied.
var validOperators = map[string]bool{
	"gt": true, "gte": true, "lt": true, "lte": true, "ne": true,
	"in": true, "nin": true, "like": true, "ilike": true,
	"regex": true, "exists": true, "and": true, "or": true,
}

var (
	errFilterNotObject = errors.New("Filter must be an object")
	errFilterEmpty     = errors.New("Filter must contain at least one condition")
	errNoValidOperator = errors.New("Operator object must contain at least one valid operator")
	errConditionType   = errors.New("Condition must be a value or operator object")
)

// ErrEmptyFilter is returned when a NESTED filter has no conditions —
// `{"and":[{}]}`, or an empty alias block. A top-level filter with no
// conditions is not an error: since D-0032 it compiles to no WHERE clause at
// all. See [Dialect.CompileWhere].
//
// It stays exported for two reasons.
//
// The first is that a caller may still want to refuse. The library will not do
// it any more, so the refusal has to be written down where it is visible:
//
//	if filter == nil || filter.Len() == 0 {
//		return sqlb.ErrEmptyFilter   // or compile it and return every row
//	}
//
// ⚠ On a DELETE or an UPDATE that check is not optional. An empty filter now
// means "affect every row in the table", and nothing between the caller and the
// database will object. That is the difference between a failed run and an
// outage, and it is the reason ENVOY-MIGRATION.md says so in those words.
//
// The second is that errors.Is(err, ErrEmptyFilter) still identifies the nested
// case, which is a malformed filter rather than an absent one. Do NOT use it to
// detect "the request had no filter": it cannot fire for that any more.
var ErrEmptyFilter = errFilterEmpty

// Set records key in order, replacing an existing entry in place.
//
// Replacing in place is what a JavaScript object does: assigning to an existing
// key keeps its original position in [[OwnPropertyKeys]]. Appending instead
// would reorder the emitted clauses and therefore the bound values.
func (f *Filter) Set(key string, value any) *Filter {
	for i := range f.entries {
		if f.entries[i].key == key {
			f.entries[i].value = value
			return f
		}
	}
	f.entries = append(f.entries, filterEntry{key: key, value: value})
	return f
}

func (f *Filter) get(key string) (any, bool) {
	for i := range f.entries {
		if f.entries[i].key == key {
			return f.entries[i].value, true
		}
	}
	return nil, false
}

// Set records an operator in order, replacing an existing one in place, for the
// same reason [Filter.Set] does.
func (o *Ops) Set(name string, value any) *Ops {
	for i := range o.entries {
		if o.entries[i].name == name {
			o.entries[i].value = value
			return o
		}
	}
	o.entries = append(o.entries, opEntry{name: name, value: value})
	return o
}

func (o *Ops) get(name string) (any, bool) {
	for i := range o.entries {
		if o.entries[i].name == name {
			return o.entries[i].value, true
		}
	}
	return nil, false
}

// --- JavaScript value semantics --------------------------------------------
//
// The reference asks JavaScript questions about its input — is this truthy, is
// this an Array, is this typeof 'string' — and the answers decide which SQL is
// emitted. Go has no equivalent of any of them, so they are spelled out here
// rather than approximated by whatever the type switch happens to fall through
// to.

// jsTruthy answers what `if (value)` answers in JavaScript. The reference asks
// it in two places that matter: whether a logical key is present
// (src/filter.ts:314) and what `exists` means (src/filter.ts:185).
func jsTruthy(v any) bool {
	switch x := v.(type) {
	case nil:
		return false
	case undefinedT:
		return false
	case bool:
		return x
	case string:
		return x != ""
	case int:
		return x != 0
	case int8:
		return x != 0
	case int16:
		return x != 0
	case int32:
		return x != 0
	case int64:
		return x != 0
	case uint:
		return x != 0
	case uint8:
		return x != 0
	case uint16:
		return x != 0
	case uint32:
		return x != 0
	case uint64:
		return x != 0
	case float32:
		return x != 0 && !math.IsNaN(float64(x))
	case float64:
		return x != 0 && !math.IsNaN(x)
	case *Filter:
		return x != nil
	case *Ops:
		return x != nil
	case []*Filter:
		// A Go slice stands for a JavaScript array, and an array is truthy even
		// when empty. Without this case a nil slice — which is what And() with no
		// arguments produces — would look falsy and the key would be skipped
		// instead of reporting "AND operator cannot be used with empty arrays".
		return true
	case []any:
		return true
	}
	rv := reflect.ValueOf(v)
	switch rv.Kind() {
	case reflect.Pointer, reflect.Map, reflect.Interface:
		return !rv.IsNil()
	case reflect.Slice:
		return true
	}
	// Everything else is an object reference in JavaScript terms, and objects
	// are truthy.
	return true
}

// isNullish is `value === null || value === undefined` (src/filter.ts:215).
func isNullish(v any) bool {
	if v == nil {
		return true
	}
	_, undef := v.(undefinedT)
	return undef
}

// asScalar reports whether a condition takes the reference's direct-value
// branch: null, undefined, string, number, boolean or Date (src/filter.ts:107).
// A nil first result with a true second means "null".
func asScalar(v any) (any, bool) {
	switch v.(type) {
	case nil:
		return nil, true
	case undefinedT:
		return nil, true
	case string, bool,
		int, int8, int16, int32, int64,
		uint, uint16, uint32, uint64,
		float32, float64,
		time.Time:
		return v, true
	}
	rv := reflect.ValueOf(v)
	switch rv.Kind() {
	case reflect.String, reflect.Bool,
		reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64,
		reflect.Float32, reflect.Float64:
		return v, true
	case reflect.Pointer:
		if rv.IsNil() {
			return nil, true
		}
	}
	return nil, false
}

// asOps treats a condition as an operator object. A *Filter is accepted because
// in JavaScript any plain object reaching this point is walked as one — which is
// how `{a: {b: 1}}` becomes "Unknown operator: b" rather than a nested filter.
func asOps(v any) (*Ops, bool) {
	switch x := v.(type) {
	case *Ops:
		if x == nil {
			return nil, false
		}
		return x, true
	case *Filter:
		if x == nil {
			return nil, false
		}
		ops := &Ops{entries: make([]opEntry, len(x.entries))}
		for i, e := range x.entries {
			ops.entries[i] = opEntry{name: e.key, value: e.value}
		}
		return ops, true
	}
	return nil, false
}

// asFilterLike treats an alias-block or sub-filter value as a nested filter.
// Arrays are objects in JavaScript, so `{$x: [1, 2]}` is walked with the keys
// "0" and "1" — which then fail identifier validation. Reproduced rather than
// special-cased, because that error is what a caller sees today.
func asFilterLike(v any) (*Filter, bool) {
	switch x := v.(type) {
	case *Filter:
		if x == nil {
			return nil, false
		}
		return x, true
	case *Ops:
		if x == nil {
			return nil, false
		}
		f := &Filter{entries: make([]filterEntry, len(x.entries))}
		for i, e := range x.entries {
			f.entries[i] = filterEntry{key: e.name, value: e.value}
		}
		return f, true
	}
	if items, ok := asJSArray(v); ok {
		f := &Filter{entries: make([]filterEntry, len(items))}
		for i, item := range items {
			f.entries[i] = filterEntry{key: itoa(i), value: item}
		}
		return f, true
	}
	return nil, false
}

// isFilterLike is [asFilterLike] without the conversion, for the counting pass
// that decides whether a node needs wrapping parentheses. It must agree with
// asFilterLike exactly.
func isFilterLike(v any) bool {
	switch x := v.(type) {
	case *Filter:
		return x != nil
	case *Ops:
		return x != nil
	case []*Filter, []any:
		return true
	case []byte, nil:
		return false
	}
	rv := reflect.ValueOf(v)
	k := rv.Kind()
	return (k == reflect.Slice || k == reflect.Array) && rv.Type().Elem().Kind() != reflect.Uint8
}

// asFilterSlice reads the operand of `and`/`or`. The []*Filter fast path is
// what the builder API produces and costs nothing; the general path exists for
// a decoded JSON filter, whose sub-filters arrive as []any.
func asFilterSlice(v any) ([]*Filter, bool) {
	if subs, ok := v.([]*Filter); ok {
		return subs, true
	}
	items, ok := asJSArray(v)
	if !ok {
		return nil, false
	}
	subs := make([]*Filter, len(items))
	for i, item := range items {
		// A non-filter element stays nil so the recursion reports "Filter must be
		// an object" — after its depth check, as the reference does.
		subs[i], _ = asFilterLike(item)
	}
	return subs, true
}

// isJSArray is Array.isArray. A []byte stands for a Uint8Array, which
// Array.isArray rejects, so it is excluded here too.
func isJSArray(v any) bool {
	switch v.(type) {
	case []any, []*Filter:
		return true
	case []byte, nil:
		return false
	}
	rv := reflect.ValueOf(v)
	k := rv.Kind()
	return (k == reflect.Slice || k == reflect.Array) && rv.Type().Elem().Kind() != reflect.Uint8
}

func asJSArray(v any) ([]any, bool) {
	switch x := v.(type) {
	case []any:
		return x, true
	case []byte:
		return nil, false
	case []*Filter:
		items := make([]any, len(x))
		for i, f := range x {
			items[i] = f
		}
		return items, true
	case nil:
		return nil, false
	}
	rv := reflect.ValueOf(v)
	if rv.Kind() != reflect.Slice && rv.Kind() != reflect.Array {
		return nil, false
	}
	if rv.Type().Elem().Kind() == reflect.Uint8 {
		return nil, false
	}
	items := make([]any, rv.Len())
	for i := range items {
		items[i] = rv.Index(i).Interface()
	}
	return items, true
}

// asString is `typeof value === 'string'`, widened to named string types.
func asString(v any) (string, bool) {
	if s, ok := v.(string); ok {
		return s, true
	}
	rv := reflect.ValueOf(v)
	if rv.IsValid() && rv.Kind() == reflect.String {
		return rv.String(), true
	}
	return "", false
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	var buf [20]byte
	pos := len(buf)
	for i > 0 {
		pos--
		buf[pos] = byte('0' + i%10)
		i /= 10
	}
	return string(buf[pos:])
}
