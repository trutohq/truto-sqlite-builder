package sqlb

import (
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"unicode/utf8"
)

// Error messages are the reference's, verbatim, so that a message a customer or
// a log-scraping alert already matches keeps matching.
var (
	errEmptyIdentifier       = errors.New("Identifier must be a non-empty string")
	errEmptyIdentifierList   = errors.New("Identifier array cannot be empty")
	errEmptyIdentifierInList = errors.New("All identifiers must be non-empty strings")
	errIdentItemType         = errors.New("Array items must be strings or SQL fragments")
	errEmptyIn               = errors.New("sql.in() cannot be used with empty arrays")
	errBlobArgument          = errors.New("sql.blob() requires a Buffer or Uint8Array")
	errStackedQuery          = errors.New("Stacked queries are not allowed")
)

// failed builds the poisoned fragment: no text, no values, a sticky error.
// Every consumer propagates it, so a failure can never contribute partial SQL.
func failed(err error) Fragment { return Fragment{err: err} }

// Q assembles a query from a constant format and its interpolation arguments.
// Port of the sql tagged template (src/sql.ts:390).
//
// Q is `?`-placeholder only, and that is enforced by types rather than by a
// check: a numbered-placeholder dialect compiles to a [Clause], which has no
// way into a Fragment. See [Dialect.CompileWhere].
func Q(text sqlText, args ...any) (Fragment, error) {
	format := string(text)

	// The reference evaluates sql.ident(), sql.in() and friends while evaluating
	// the template's expressions — that is, before the tag body runs at all. So a
	// failing helper always wins over a failing value conversion, whatever their
	// relative positions. Reproducing that means sweeping the fragments first.
	hasFragment := false
	for _, arg := range args {
		if f, ok := arg.(Fragment); ok {
			if f.err != nil {
				return Fragment{}, f.err
			}
			hasFragment = true
		}
	}

	// When no argument splices text, every slot is a `?` replaced by a `?`: the
	// assembled query IS the format, byte for byte. Skipping the builder is the
	// difference between one allocation and none on much the most common shape a
	// caller writes.
	if !hasFragment {
		return qValuesOnly(format, args)
	}

	var b strings.Builder
	b.Grow(len(format) + 16*len(args))
	values := make([]any, 0, len(args))

	slots := 0
	last := 0
	i, n := 0, len(format)

	for i < n {
		c := format[i]

		if c == '-' && i+1 < n && format[i+1] == '-' {
			i += 2
			for i < n && format[i] != '\n' {
				i++
			}
			continue
		}
		if c == '/' && i+1 < n && format[i+1] == '*' {
			i += 2
			for i < n && !(format[i] == '*' && i+1 < n && format[i+1] == '/') {
				i++
			}
			i += 2
			continue
		}
		if c == '\'' || c == '"' || c == '`' {
			quote := c
			i++
			for i < n {
				if format[i] == quote {
					if i+1 < n && format[i+1] == quote {
						i += 2
						continue
					}
					i++
					break
				}
				i++
			}
			continue
		}
		if c == '[' {
			i++
			for i < n && format[i] != ']' {
				i++
			}
			i++
			continue
		}
		if c != '?' {
			if c < utf8.RuneSelf {
				i++
			} else {
				_, size := utf8.DecodeRuneInString(format[i:])
				i += size
			}
			continue
		}

		// A `?` in code position is an interpolation slot.
		b.WriteString(format[last:i])
		last = i + 1
		i++

		if slots < len(args) {
			arg := args[slots]
			if f, ok := arg.(Fragment); ok {
				b.WriteString(f.text)
				values = append(values, f.values...)
			} else {
				v, err := Value(arg)
				if err != nil {
					return Fragment{}, err
				}
				b.WriteByte('?')
				values = append(values, v)
			}
		}
		slots++
	}
	b.WriteString(format[last:])

	if slots != len(args) {
		return Fragment{}, fmt.Errorf(
			"sqlb: format has %s but %s supplied",
			plural(slots, "interpolation slot"), pluralWere(len(args), "argument"))
	}

	assembled := b.String()

	if length, over := exceedsUTF16(assembled, MaxQueryLength); over {
		return Fragment{}, fmt.Errorf("Query too long: %d bytes (max: %d)", length, MaxQueryLength)
	}

	placeholders, stacked := scanSQL(assembled)

	// Integrity: every placeholder must have exactly one bound value and vice
	// versa. This is what catches a raw fragment smuggling a stray `?`, and what
	// makes the reference's JSON-path misalignment fail closed instead of
	// silently binding the wrong columns.
	if placeholders != len(values) {
		return Fragment{}, fmt.Errorf(
			"Placeholder count (%d) does not match bound value count (%d). "+
				"Did a raw fragment contain a \"?\" without supplying its value?",
			placeholders, len(values))
	}

	if stacked {
		return Fragment{}, errStackedQuery
	}

	return Fragment{text: assembled, values: values}, nil
}

// qValuesOnly is Q for the case where no argument is a Fragment.
//
// The scan is the same one Q uses, minus the copying: every code-position `?`
// is both an interpolation slot and a placeholder in the finished query, so
// counting once answers the arity question and the integrity question at the
// same time — placeholders and bound values cannot disagree on this path.
func qValuesOnly(format string, args []any) (Fragment, error) {
	// Value conversion happens FIRST, because the reference converts while it
	// assembles and therefore reports an unsupported value before it reports an
	// over-long or stacked query. Getting this order wrong changes which error a
	// caller sees for input that is wrong in two ways at once.
	var values []any
	if len(args) > 0 {
		values = make([]any, len(args))
		for i, arg := range args {
			v, err := Value(arg)
			if err != nil {
				return Fragment{}, err
			}
			values[i] = v
		}
	}

	slots, stacked := scanSQL(format)
	if slots != len(args) {
		return Fragment{}, fmt.Errorf(
			"sqlb: format has %s but %s supplied",
			plural(slots, "interpolation slot"), pluralWere(len(args), "argument"))
	}
	if length, over := exceedsUTF16(format, MaxQueryLength); over {
		return Fragment{}, fmt.Errorf("Query too long: %d bytes (max: %d)", length, MaxQueryLength)
	}
	if stacked {
		return Fragment{}, errStackedQuery
	}
	return Fragment{text: format, values: values}, nil
}

func plural(n int, noun string) string {
	if n == 1 {
		return fmt.Sprintf("%d %s", n, noun)
	}
	return fmt.Sprintf("%d %ss", n, noun)
}

func pluralWere(n int, noun string) string {
	if n == 1 {
		return fmt.Sprintf("%d %s was", n, noun)
	}
	return fmt.Sprintf("%d %ss were", n, noun)
}

// In builds a parenthesised placeholder list. Port of sqlIn (src/sql.ts:147).
func In(values ...any) Fragment {
	if len(values) == 0 {
		return failed(errEmptyIn)
	}
	warnLargeIn(len(values))

	var b strings.Builder
	b.Grow(2*len(values) + 1)
	b.WriteByte('(')
	converted := make([]any, len(values))
	for i, v := range values {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteByte('?')
		c, err := Value(v)
		if err != nil {
			return failed(err)
		}
		converted[i] = c
	}
	b.WriteByte(')')
	return Fragment{text: b.String(), values: converted}
}

// InSlice is [In] for a typed slice. It exists because converting []string to
// []any at every call site is the sort of noise that makes people reach for
// string concatenation instead.
func InSlice[T any](values []T) Fragment {
	boxed := make([]any, len(values))
	for i, v := range values {
		boxed[i] = v
	}
	return In(boxed...)
}

// warnLargeIn reproduces the reference's console.warn (src/sql.ts:157-161).
//
// A library writing to the default logger is not ideal, but the warning is
// behaviour the reference has and dropping it would quietly remove the only
// signal that a caller is about to hand SQLite a thousand-term IN list.
func warnLargeIn(n int) {
	if n <= 1000 {
		return
	}
	slog.Warn(fmt.Sprintf(
		"sql.in(): Large array with %d items. Consider using temporary tables for better performance.", n))
}

// Raw creates a fragment from SQL text verbatim. Port of sqlRaw
// (src/sql.ts:177).
//
// DANGEROUS, and deliberately so: this is the library's single explicit trust
// boundary, the one place a string becomes SQL without being validated or
// bound. Only ever pass developer-authored SQL. For a dynamic WHERE clause use
// [CompileFilter]; for identifiers use [Ident] or [Idents]; for a list of
// values use [In].
//
// Note that [Q]'s format cannot be a variable at all, so Raw is also the only
// way to build genuinely dynamic query text — which is the point. It is
// greppable.
func Raw(text string) Fragment { return Fragment{text: text} }

// Blob binds binary data as one parameter. Port of sqlBlob (src/sql.ts:188).
//
// The reference rejects a string, a number and null. Go's signature rules out
// the first two; a nil slice is the null case and is rejected here, which also
// keeps an accidental `Blob(nilVar)` from silently binding NULL.
func Blob(data []byte) Fragment {
	if data == nil {
		return failed(errBlobArgument)
	}
	return Fragment{text: "?", values: []any{data}}
}

// separatorForbidden lists the tokens that would let a join separator escape
// its structural-connector role: literal delimiters, statement terminators,
// comment markers, NUL and backslash escapes. src/sql.ts:204.
//
// display holds each token as JSON.stringify would render it, because the
// reference embeds that form in the error message and error text is part of
// what a port has to preserve.
var separatorForbidden = []struct{ token, display string }{
	{"'", `"'"`},
	{`"`, `"\""`},
	{"`", "\"`\""},
	{"[", `"["`},
	{"]", `"]"`},
	{";", `";"`},
	{`\`, `"\\"`},
	{"\x00", "\"\\u0000\""},
	{"--", `"--"`},
	{"/*", `"/*"`},
	{"*/", `"*/"`},
}

// checkSeparator validates a string separator. Port of assertSafeSeparator
// (src/sql.ts:224).
func checkSeparator(sep string) error {
	for _, f := range separatorForbidden {
		if strings.Contains(sep, f.token) {
			return fmt.Errorf(
				"Unsafe sql.join() separator: contains forbidden token %s. "+
					"Pass a SqlFragment (e.g. sql.raw) if you need parameterized separators.",
				f.display)
		}
	}

	depth := 0
	for i := range len(sep) {
		switch sep[i] {
		case '(':
			depth++
		case ')':
			depth--
			if depth < 0 {
				return errors.New("Unsafe sql.join() separator: unbalanced parentheses")
			}
		}
	}
	if depth != 0 {
		return errors.New("Unsafe sql.join() separator: unbalanced parentheses")
	}
	return nil
}

// Join concatenates fragments with a validated string separator. Port of
// sqlJoin (src/sql.ts:262) for the string-separator case.
func Join(frags []Fragment, sep string) Fragment {
	if f, err := checkFragments(frags); err != nil {
		return f
	}
	if len(frags) == 0 {
		// The reference returns early, before validating the separator
		// (src/sql.ts:278-280). Keep the order: an empty join never fails.
		return Fragment{}
	}
	if err := checkSeparator(sep); err != nil {
		return failed(err)
	}
	return assemble(frags, sep, nil)
}

// JoinFragment concatenates fragments with a fragment separator, interleaving
// the separator's own values. Port of sqlJoin's fragment-separator case
// (src/sql.ts:285-287).
func JoinFragment(frags []Fragment, sep Fragment) Fragment {
	if f, err := checkFragments(frags); err != nil {
		return f
	}
	if len(frags) == 0 {
		return Fragment{}
	}
	if sep.err != nil {
		return failed(sep.err)
	}
	return assemble(frags, sep.text, sep.values)
}

func checkFragments(frags []Fragment) (Fragment, error) {
	for _, f := range frags {
		if f.err != nil {
			return failed(f.err), f.err
		}
	}
	return Fragment{}, nil
}

func assemble(frags []Fragment, sepText string, sepValues []any) Fragment {
	textLen := len(sepText) * (len(frags) - 1)
	valueLen := len(sepValues) * (len(frags) - 1)
	for _, f := range frags {
		textLen += len(f.text)
		valueLen += len(f.values)
	}

	var b strings.Builder
	b.Grow(textLen)
	values := make([]any, 0, valueLen)

	for i, f := range frags {
		if i > 0 {
			b.WriteString(sepText)
			values = append(values, sepValues...)
		}
		b.WriteString(f.text)
		values = append(values, f.values...)
	}
	return Fragment{text: b.String(), values: values}
}
