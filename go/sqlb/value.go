package sqlb

import (
	"errors"
	"fmt"
	"reflect"
	"strconv"
	"time"
)

// errBlobValue is the reference's message verbatim (src/sql.ts:46).
var errBlobValue = errors.New(
	"Buffer/Uint8Array values must be used with sql.blob() for safe BLOB handling")

// Value converts a Go value to its SQLite representation. Port of sqlValue
// (src/sql.ts:28).
func Value(v any) (any, error) {
	switch x := v.(type) {
	case nil:
		return nil, nil
	case undefinedT:
		// JavaScript's undefined and null both become null (src/sql.ts:29-31).
		return nil, nil
	case string:
		return x, nil
	case bool:
		return x, nil
	case int:
		return x, nil
	case int8:
		return x, nil
	case int16:
		return x, nil
	case int32:
		return x, nil
	case int64:
		return x, nil
	case uint:
		return x, nil
	case uint8:
		return x, nil
	case uint16:
		return x, nil
	case uint32:
		return x, nil
	case uint64:
		return x, nil
	case float32:
		return x, nil
	case float64:
		return x, nil
	case time.Time:
		return formatDate(x), nil
	case []byte:
		return nil, errBlobValue
	case Fragment:
		// A fragment reaching Value means a caller passed one where a value was
		// expected — outside Q, which splices them. Refusing is the safe answer.
		return nil, fmt.Errorf("Unsupported value type: %T", v)
	}
	return valueSlow(v)
}

// valueSlow handles named types and pointers. It is deliberately off the hot
// path: every type the reference can actually see is in the switch above, and
// this branch exists so `type Status string` and `*int` — both ordinary in Go
// and inexpressible in JavaScript — do not become an error a caller cannot act
// on.
func valueSlow(v any) (any, error) {
	rv := reflect.ValueOf(v)
	switch rv.Kind() {
	case reflect.Pointer:
		if rv.IsNil() {
			return nil, nil
		}
		return Value(rv.Elem().Interface())
	case reflect.String:
		return rv.String(), nil
	case reflect.Bool:
		return rv.Bool(), nil
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return rv.Int(), nil
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return rv.Uint(), nil
	case reflect.Float32, reflect.Float64:
		return rv.Float(), nil
	case reflect.Slice:
		if rv.Type().Elem().Kind() == reflect.Uint8 {
			return nil, errBlobValue
		}
	}
	return nil, fmt.Errorf("Unsupported value type: %T", v)
}

// formatDate renders a time as SQLite's `YYYY-MM-DD HH:MM:SS`. Port of
// formatDate (src/sql.ts:14).
//
// Two details are load-bearing:
//
//   - The year is NOT zero-padded. The reference builds it with
//     String(date.getFullYear()), so year 999 formats as "999-01-02", where Go's
//     "2006" layout would give "0999". Confirmed against the reference
//     2026-09-06.
//   - Every other field is padded to two digits with padStart.
//
// The reference reads the date through getFullYear() and friends, which are
// local-time getters. Go times carry their own location and this honours it;
// see DIVERGENCES.md D-DATE-ZONE. Cloudflare Workers run in UTC, so passing a
// UTC time reproduces what production stores.
func formatDate(t time.Time) string {
	year, month, day := t.Date()
	hour, minute, second := t.Clock()

	buf := make([]byte, 0, 24)
	buf = strconv.AppendInt(buf, int64(year), 10)
	buf = append(buf, '-')
	buf = appendPad2(buf, int(month))
	buf = append(buf, '-')
	buf = appendPad2(buf, day)
	buf = append(buf, ' ')
	buf = appendPad2(buf, hour)
	buf = append(buf, ':')
	buf = appendPad2(buf, minute)
	buf = append(buf, ':')
	buf = appendPad2(buf, second)
	return string(buf)
}

func appendPad2(buf []byte, v int) []byte {
	if v < 0 || v > 99 {
		return strconv.AppendInt(buf, int64(v), 10)
	}
	return append(buf, byte('0'+v/10), byte('0'+v%10))
}
