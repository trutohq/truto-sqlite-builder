package sqlb

import (
	"math"

	"github.com/trutohq/jsorder"
)

// JSON decoding lives in github.com/trutohq/jsorder, not here.
//
// sqlb used to carry its own order-preserving decoder. It was deleted (D-0023)
// even though it cost this package its zero-dependency property, because two
// order-preserving JSON decoders in one organisation is one duplicated
// DECISION, and duplicated decisions drift silently — they surface as a
// signature that will not verify rather than as an error.
//
// What is left is the conversion below: jsorder's value model into sqlb's. It
// is deliberately small, because that is the whole argument for the trade.

// DecodeFilter parses a JSON filter into a [Filter], preserving key order.
//
// Order is the reason this is not json.Unmarshal into a map: key order decides
// clause order, which decides the order of the bound values, which decides
// which column each placeholder binds to.
//
// A JSON object becomes a Filter; a JSON array becomes a Filter keyed "0",
// "1", … because JavaScript's Object.entries walks an array that way and the
// reference then rejects it with `Invalid identifier: 0`; anything else is
// "Filter must be an object", as compileFilter's null/typeof check is
// (src/filter.ts:417, confirmed by executing the reference 2026-09-06).
func DecodeFilter(data []byte) (*Filter, error) {
	// DecodeFloat64, not Decode. jsorder.Decode keeps a number as its literal
	// json.Number, which is LOSSLESS and therefore wrong here: the reference
	// reaches compileFilter through JSON.parse, so a filter on an id past 2^53
	// has already been rounded by the time any SQL is emitted. Executing the
	// reference gives `{"id": 9007199254740993}` → the bound value
	// 9007199254740992, and `{"id": 1234567890123456789}` →
	// 1234567890123456800. Decode would bind the exact digits and send a
	// different query than production does. See DIVERGENCES J-01.
	v, err := jsorder.DecodeFloat64(data)
	if err != nil {
		return nil, err
	}
	return FilterFromJS(v)
}

// FilterFromJS converts an already-decoded jsorder value into a [Filter]. Use
// it when the JSON has been parsed for other reasons too, so it is not parsed
// twice.
func FilterFromJS(v any) (*Filter, error) {
	f, ok := asFilterLike(fromJS(v))
	if !ok {
		return nil, errFilterNotObject
	}
	return f, nil
}

// fromJS rewrites jsorder's value model into sqlb's. Three things change and
// nothing else does:
//
//   - *jsorder.Map becomes *Filter, keeping key order.
//   - jsorder's Null sentinel becomes Go nil. The two packages spend Go's nil
//     differently: jsorder reserves it for JavaScript's `undefined` and marks
//     JSON null with a sentinel, while sqlb's nil IS null (compiling to
//     IS NULL) and [Undefined] is its sentinel. Getting this backwards turns
//     `{"deleted_at": null}` from `IS NULL` into a dropped condition, which
//     widens a DELETE.
//   - an integral number narrows to int64; see jsNumber.
func fromJS(v any) any {
	switch x := v.(type) {
	case *jsorder.Map:
		f := &Filter{entries: make([]filterEntry, 0, x.Len())}
		x.Range(func(key string, value any) bool {
			// Appended rather than passed through Filter.Set: a jsorder.Map has
			// already applied JavaScript's "first position, last value" rule for a
			// repeated key, so Set's linear duplicate scan could never fire and
			// would only make a wide object quadratic. The old decoder did call
			// Set here, so a hostile 100k-key filter was O(n²) — this removes that.
			f.entries = append(f.entries, filterEntry{key: key, value: fromJS(value)})
			return true
		})
		return f
	case []any:
		out := make([]any, len(x))
		for i, item := range x {
			out[i] = fromJS(item)
		}
		return out
	case float64:
		return jsNumber(x)
	}
	if jsorder.IsNull(v) {
		return nil
	}
	return v
}

// jsNumber narrows an integral JSON number to int64.
//
// JavaScript cannot tell 1 from 1.0, so the reference's driver binds an
// integral number as an INTEGER; leaving every number a float64 would bind REAL
// instead and change what a comparison against an INTEGER column means.
//
// The range is JavaScript's SAFE-INTEGER range, not int64's, and the
// differential harness is what settled that. Past 2^53 a JSON literal is
// already inexact by the time the reference sees it — 1234567890123456789
// arrives as 1234567890123456768 — and JavaScript renders that value as
// 1234567890123456800, because the shortest decimal that round-trips the float
// is all it can honestly claim. Narrowing it to an int64 would print
// eighteen exact digits the reference never had, and bind an INTEGER where
// JavaScript hands its driver a float. Outside the safe range, REAL is the
// truthful answer; inside it, INTEGER is.
func jsNumber(f float64) any {
	const maxExactInt = 1 << 53
	if f == math.Trunc(f) && f >= -maxExactInt && f <= maxExactInt {
		return int64(f)
	}
	return f
}
