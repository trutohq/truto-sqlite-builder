package sqlb

import (
	"strings"
	"testing"

	"github.com/trutohq/jsorder"
)

// The glue that replaced sqlb's own JSON decoder (D-0023).
//
// Every `want` below was produced by EXECUTING the reference — bun against
// ../src — not by reading filter.ts. The whole argument for spending the
// zero-dependency property is that this layer is small; these are the cases
// that say it is also right.

func TestDecodeFilter(t *testing.T) {
	cases := []struct {
		name   string
		json   string
		text   string
		values []any
		err    string
	}{
		{
			// JSON.parse rounds at 2^53 and the reference never sees the odd digit.
			name: "a literal past 2^53 binds the value JSON.parse produced",
			json: `{"id": 9007199254740993}`,
			text: `(("id" = ?))`, values: []any{int64(9007199254740992)},
		}, {
			// Past the safe range REAL is the honest binding: 1234567890123456789
			// is 1234567890123456768 as a float, which JavaScript renders
			// 1234567890123456800.
			name: "a literal past the safe range stays a float",
			json: `{"id": 1234567890123456789}`,
			text: `(("id" = ?))`, values: []any{float64(1234567890123456789)},
		}, {
			name:   "an integral number binds as INTEGER and a fractional one as REAL",
			json:   `{"id": 1.0, "score": 0.5}`,
			text:   `((("id" = ?) AND ("score" = ?)))`,
			values: []any{int64(1), 0.5},
		}, {
			// jsorder marks JSON null with a sentinel and spends Go nil on
			// undefined; sqlb is the other way round. Getting this backwards drops
			// the condition instead of emitting IS NULL, which widens a DELETE.
			name: "JSON null is sqlb's nil and compiles to IS NULL",
			json: `{"deleted_at": null}`,
			text: `(("deleted_at" IS NULL))`, values: nil,
		}, {
			name: "a duplicate key keeps the LAST value",
			json: `{"a":1,"a":2}`,
			text: `(("a" = ?))`, values: []any{int64(2)},
		}, {
			// The first position, the last value — what a JavaScript object literal
			// and JSON.parse both do, and what decides which column each ? binds.
			name:   "a duplicate key keeps the FIRST position",
			json:   `{"b":1,"a":2,"b":3}`,
			text:   `((("b" = ?) AND ("a" = ?)))`,
			values: []any{int64(3), int64(2)},
		}, {
			// [[OwnPropertyKeys]]: array-index keys first, in ASCENDING NUMERIC
			// order. "2" is reported before "10" even though "10" is written first.
			// sqlb's own decoder did not do this and the differential corpus had to
			// exclude integer keys to hide it.
			name: "integer keys are enumerated in numeric order, not document order",
			json: `{"10":1,"2":1}`,
			err:  "Invalid identifier: 2.",
		}, {
			name: "an integer key is hoisted above a string key written before it",
			json: `{"status":"x","2":1}`,
			err:  "Invalid identifier: 2.",
		}, {
			// Array.isArray is not the test compileFilter makes: typeof [] is
			// 'object', so Object.entries walks it with the keys "0", "1".
			name: "a top-level array is walked with index keys",
			json: `[1,2]`,
			err:  "Invalid identifier: 0.",
		}, {
			name: "a top-level scalar is not a filter",
			json: `"x"`,
			err:  "Filter must be an object",
		}, {
			name: "a top-level null is not a filter",
			json: `null`,
			err:  "Filter must be an object",
		}, {
			name:   "nested objects become sub-filters",
			json:   `{"and":[{"a":1},{"b":2}]}`,
			text:   `((("a" = ?) AND ("b" = ?)))`,
			values: []any{int64(1), int64(2)},
		}, {
			name: "an alias block survives decoding",
			json: `{"$t2":{"col":{"gte":100}}}`,
			text: `((t2."col" >= ?))`, values: []any{int64(100)},
		}, {
			// exists is a TRUTHINESS test (S-06). 0 is falsy, so this is IS NULL —
			// a decoder that typed the operand as bool would get it backwards.
			name: "exists 0 is falsy",
			json: `{"a":{"exists":0}}`,
			text: `(("a" IS NULL))`, values: nil,
		}, {
			name: "exists on a non-empty string is truthy",
			json: `{"a":{"exists":"no"}}`,
			text: `(("a" IS NOT NULL))`, values: nil,
		}, {
			name:   "mixed types inside IN survive as themselves",
			json:   `{"a":{"in":[1,"2",true,null]}}`,
			text:   `(("a" IN (?,?,?,?)))`,
			values: []any{int64(1), "2", true, nil},
		}, {
			name: "an exponent literal is a float",
			json: `{"a":{"gt":1e21}}`,
			text: `(("a" > ?))`, values: []any{1e21},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			filter, err := DecodeFilter([]byte(tc.json))
			var frag Fragment
			if err == nil {
				frag, err = CompileFilter(filter)
			}
			if tc.err != "" {
				if err == nil {
					t.Fatalf("want error %q, got %q", tc.err, frag.Text())
				}
				if !strings.Contains(err.Error(), tc.err) {
					t.Fatalf("error\n  got:  %s\n  want: %s", err, tc.err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if frag.Text() != tc.text {
				t.Errorf("text\n  got:  %s\n  want: %s", frag.Text(), tc.text)
			}
			assertValues(t, frag.Values(), tc.values)
		})
	}
}

func assertValues(t *testing.T, got, want []any) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("values\n  got:  %#v\n  want: %#v", got, want)
	}
	for i := range got {
		if got[i] != want[i] {
			t.Errorf("value %d\n  got:  %#v\n  want: %#v", i, got[i], want[i])
		}
	}
}

// DecodeFilter must not be the only door: a caller that has already parsed the
// request body should not parse it twice.
func TestFilterFromJS(t *testing.T) {
	decoded, err := jsorder.DecodeFloat64([]byte(`{"status":"ACTIVE","age":{"gte":18}}`))
	if err != nil {
		t.Fatal(err)
	}
	filter, err := FilterFromJS(decoded)
	if err != nil {
		t.Fatal(err)
	}
	frag, err := CompileFilter(filter)
	if err != nil {
		t.Fatal(err)
	}
	if want := `((("status" = ?) AND ("age" >= ?)))`; frag.Text() != want {
		t.Errorf("text\n  got:  %s\n  want: %s", frag.Text(), want)
	}
	assertValues(t, frag.Values(), []any{"ACTIVE", int64(18)})
}

// A decoded filter is a plain Filter, so the builder API can extend one. This
// is what makes deleting CompileFilterJSON cheap rather than a loss.
func TestDecodedFilterIsAnOrdinaryFilter(t *testing.T) {
	filter, err := DecodeFilter([]byte(`{"status":"ACTIVE"}`))
	if err != nil {
		t.Fatal(err)
	}
	filter.Set("tenant_id", "t-1")
	frag, err := CompileFilter(filter)
	if err != nil {
		t.Fatal(err)
	}
	if want := `((("status" = ?) AND ("tenant_id" = ?)))`; frag.Text() != want {
		t.Errorf("text\n  got:  %s\n  want: %s", frag.Text(), want)
	}
}

// Malformed UTF-8 must not fail the parse: JSON.parse replaces it, and a
// decoder that rejected it would turn a request the reference accepts into a
// 400. encoding/json (jsorder's tokenizer) replaces too.
func TestDecodeFilterAcceptsInvalidUTF8(t *testing.T) {
	filter, err := DecodeFilter([]byte("{\"status\":\"a\xffb\"}"))
	if err != nil {
		t.Fatal(err)
	}
	frag, err := CompileFilter(filter)
	if err != nil {
		t.Fatal(err)
	}
	if got := frag.Values()[0].(string); got != "a�b" {
		t.Errorf("value %q, want the replacement character", got)
	}
}

func TestDecodeFilterRejectsTrailingContent(t *testing.T) {
	if _, err := DecodeFilter([]byte(`{"a":1} garbage`)); err == nil {
		t.Fatal("want an error for trailing content")
	}
}

// A wide object used to be quadratic: the old decoder pushed every key through
// Filter.Set, whose duplicate scan is linear. jsorder deduplicates already, so
// the glue appends. 20k keys would be 200M comparisons the old way.
func TestDecodeFilterWideObjectIsNotQuadratic(t *testing.T) {
	var b strings.Builder
	b.WriteByte('{')
	for i := range 20000 {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteString(`"k`)
		b.WriteString(itoa(i))
		b.WriteString(`":1`)
	}
	b.WriteByte('}')

	filter, err := DecodeFilter([]byte(b.String()))
	if err != nil {
		t.Fatal(err)
	}
	if filter.Len() != 20000 {
		t.Fatalf("Len = %d, want 20000", filter.Len())
	}
}
