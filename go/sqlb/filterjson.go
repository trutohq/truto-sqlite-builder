package sqlb

import (
	"bytes"
	"encoding/json/jsontext"
	"errors"
	"fmt"
	"io"
)

// UnmarshalJSON decodes a filter while preserving key order.
//
// Order is the whole reason this exists. Decoding into map[string]any would
// lose it, and key order decides clause order, which decides the order of the
// bound values.
//
// It reads through encoding/json/jsontext rather than encoding/json's
// Decoder.Token. Both are standard library and both preserve order; the
// difference is that Token returns `any`, so every string, number and delimiter
// is boxed. On a realistic 645-byte filter that accounted for 47% of the
// allocations on this path and roughly three quarters of the wall time of a
// whole compile. jsontext.Token is a struct.
//
// Numbers: an integral value inside int64's range decodes to int64, everything
// else to float64. JavaScript cannot tell 1 from 1.0, so D1 binds an integral
// JSON number as an INTEGER; decoding to float64 unconditionally would bind
// REAL instead and change what a comparison means. Values beyond 2^53 are
// already lossy on the JavaScript side and stay that way here — see
// DIVERGENCES.md D-JSON-BIGINT.
//
// JSON has no undefined, so [Undefined] never comes out of this.
func (f *Filter) UnmarshalJSON(data []byte) error {
	// AllowDuplicateNames and AllowInvalidUTF8 are both required for parity, and
	// both are off by default because jsontext implements RFC 7493. JSON.parse
	// does not:
	//
	//   - Duplicate keys: JavaScript keeps the LAST value at the FIRST key's
	//     position. jsontext errors. Filter.Set already replaces in place, which
	//     is the same rule, so allowing them is all that is needed.
	//   - Invalid UTF-8: JavaScript replaces malformed sequences rather than
	//     failing the parse.
	//
	// The differential harness found both within a minute of the decoder being
	// switched over; see GOTCHAS.md.
	dec := jsontext.NewDecoder(bytes.NewReader(data),
		jsontext.AllowDuplicateNames(true),
		jsontext.AllowInvalidUTF8(true))

	tok, err := dec.ReadToken()
	if err != nil {
		return err
	}
	if tok.Kind() != '{' {
		return errFilterNotObject
	}

	st := &decodeState{dec: dec}
	decoded, err := st.object()
	if err != nil {
		return err
	}
	*f = *decoded

	if _, err := dec.ReadToken(); !errors.Is(err, io.EOF) {
		return fmt.Errorf("sqlb: trailing data after filter object")
	}
	return nil
}

// decodeState carries the block allocators.
//
// A filter tree of N objects is N Filter headers and N entry slices, which as
// individual allocations dominated this path once the token reader stopped
// boxing. They are carved from blocks instead: each object still owns its own
// storage exclusively, so an entries slice that outgrows its four-element
// window simply reallocates the normal way.
type decodeState struct {
	dec      *jsontext.Decoder
	filters  []Filter
	entries  []filterEntry
	elements []any
}

const (
	filterBlock  = 16
	entryWindow  = 4
	entryBlock   = 64
	elementBlock = 32
)

func (s *decodeState) newFilter() *Filter {
	if len(s.filters) == 0 {
		s.filters = make([]Filter, filterBlock)
	}
	f := &s.filters[0]
	s.filters = s.filters[1:]
	f.entries = s.newEntries()
	return f
}

func (s *decodeState) newEntries() []filterEntry {
	if len(s.entries) < entryWindow {
		s.entries = make([]filterEntry, entryBlock)
	}
	window := s.entries[:0:entryWindow]
	s.entries = s.entries[entryWindow:]
	return window
}

func (s *decodeState) newElements() []any {
	if len(s.elements) < entryWindow {
		s.elements = make([]any, elementBlock)
	}
	window := s.elements[:0:entryWindow]
	s.elements = s.elements[entryWindow:]
	return window
}

// object reads members until the closing brace, whose opening the caller has
// already consumed.
func (s *decodeState) object() (*Filter, error) {
	f := s.newFilter()
	for {
		tok, err := s.dec.ReadToken()
		if err != nil {
			return nil, err
		}
		if tok.Kind() == '}' {
			return f, nil
		}
		if tok.Kind() != '"' {
			return nil, fmt.Errorf("sqlb: object key is not a string")
		}
		key := tok.String()

		value, err := s.value()
		if err != nil {
			return nil, err
		}
		f.Set(key, value)
	}
}

func (s *decodeState) array() ([]any, error) {
	items := s.newElements()
	for {
		if s.dec.PeekKind() == ']' {
			if _, err := s.dec.ReadToken(); err != nil {
				return nil, err
			}
			return items, nil
		}
		item, err := s.value()
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
}

func (s *decodeState) value() (any, error) {
	tok, err := s.dec.ReadToken()
	if err != nil {
		return nil, err
	}
	switch tok.Kind() {
	case '{':
		return s.object()
	case '[':
		return s.array()
	case '"':
		return tok.String(), nil
	case 't':
		return true, nil
	case 'f':
		return false, nil
	case 'n':
		return nil, nil
	case '0':
		return decodeNumber(tok)
	}
	return nil, fmt.Errorf("sqlb: unexpected token %q", tok.Kind())
}

func decodeNumber(tok jsontext.Token) (any, error) {
	// Int() succeeds only for a value that is exactly an int64, which is also
	// the only case where SQLite should see an INTEGER.
	if i, err := tok.Int(); err == nil {
		return i, nil
	}
	fl, err := tok.Float()
	if err != nil {
		return nil, err
	}
	// A float that is exactly integral is a JavaScript integer as far as SQLite
	// binding is concerned.
	if fl == float64(int64(fl)) && fl >= -(1<<53) && fl <= 1<<53 {
		return int64(fl), nil
	}
	return fl, nil
}

// CompileFilterJSON decodes a JSON filter and compiles it in one step. It is
// the entry point a service handling a customer-supplied filter body wants.
func CompileFilterJSON(data []byte) (Fragment, error) {
	f := NewFilter()
	if err := f.UnmarshalJSON(data); err != nil {
		return Fragment{}, err
	}
	return CompileFilter(f)
}
