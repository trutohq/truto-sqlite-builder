package sqlb

import (
	"fmt"
	"strings"
)

// isIdentStart and isIdentPart implement SIMPLE_IDENTIFIER_REGEX
// (src/constants.ts:30) by hand.
//
// Hand-written rather than compiled from a regexp because the class is
// explicitly ASCII and this runs once per column of every query. Any byte
// >= 0x80 fails, which is what keeps a Unicode look-alike out of a quoted
// identifier.
func isIdentStart(c byte) bool {
	return c == '_' || (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z')
}

func isIdentPart(c byte) bool {
	return isIdentStart(c) || (c >= '0' && c <= '9')
}

func isSimpleIdent(s string) bool {
	if s == "" || !isIdentStart(s[0]) {
		return false
	}
	for i := 1; i < len(s); i++ {
		if !isIdentPart(s[i]) {
			return false
		}
	}
	return true
}

// isQualifiedIdent implements QUALIFIED_IDENTIFIER_REGEX (src/constants.ts:26):
// one or more simple identifiers joined by dots.
func isQualifiedIdent(s string) bool {
	if s == "" {
		return false
	}
	start := 0
	for i := 0; i <= len(s); i++ {
		if i == len(s) || s[i] == '.' {
			if !isSimpleIdent(s[start:i]) {
				return false
			}
			start = i + 1
		}
	}
	return true
}

// invalidIdentifier is the reference's message verbatim (src/sql.ts:113).
func invalidIdentifier(name string) error {
	return fmt.Errorf(
		"Invalid identifier: %s. Must be a valid identifier or qualified identifier (e.g., table.column)",
		name)
}

// validateIdent is the whole of the identifier trust boundary: charset, then
// per-part length. Everything that quotes an identifier calls this first, so
// there is exactly one place to read when asking what can get through.
//
// Identifiers that pass are ASCII by construction, so a byte length is also a
// UTF-16 length and the limit needs no scan.
func validateIdent(name string) error {
	if name == "" {
		return errEmptyIdentifier
	}
	if !isQualifiedIdent(name) {
		return invalidIdentifier(name)
	}
	start := 0
	for i := 0; i <= len(name); i++ {
		if i != len(name) && name[i] != '.' {
			continue
		}
		if part := i - start; part > MaxIdentifierLength {
			return fmt.Errorf("Identifier part too long: %d characters (max: %d)",
				part, MaxIdentifierLength)
		}
		start = i + 1
	}
	return nil
}

// quotedLen is how many bytes the quoted form of a validated identifier takes.
func quotedLen(name string) int {
	return len(name) + 2 + 2*strings.Count(name, ".")
}

// appendQuotedIdent appends the quoted form of an already-validated identifier.
// This is the emitter the filter compiler uses, so that quoting a column costs
// no allocation of its own.
func appendQuotedIdent(dst []byte, name string) []byte {
	start := 0
	for i := 0; i <= len(name); i++ {
		if i != len(name) && name[i] != '.' {
			continue
		}
		if start > 0 {
			dst = append(dst, '.')
		}
		dst = append(dst, '"')
		dst = append(dst, name[start:i]...)
		dst = append(dst, '"')
		start = i + 1
	}
	return dst
}

func writeQuotedIdent(b *strings.Builder, name string) {
	start := 0
	for i := 0; i <= len(name); i++ {
		if i != len(name) && name[i] != '.' {
			continue
		}
		if start > 0 {
			b.WriteByte('.')
		}
		b.WriteByte('"')
		b.WriteString(name[start:i])
		b.WriteByte('"')
		start = i + 1
	}
}

// Ident validates and quotes one identifier, which may be qualified with dots.
// Port of the scalar half of sqlIdent (src/sql.ts:131-141).
func Ident(name string) Fragment {
	if err := validateIdent(name); err != nil {
		return failed(err)
	}
	var b strings.Builder
	b.Grow(quotedLen(name))
	writeQuotedIdent(&b, name)
	return Fragment{text: b.String()}
}

// Idents quotes a list of identifiers and fragments, joined with ", ". Port of
// the array half of sqlIdent (src/sql.ts:92-128).
//
// Only a Fragment may pass through unquoted, and only this package can mint
// one. That is what stops a value shaped like a fragment being injected as raw
// SQL, which is what the reference's WeakSet is for.
//
// Everything is validated before anything is emitted, so a list containing one
// bad identifier yields a fragment with no text at all rather than a partial
// one.
func Idents(items ...any) Fragment {
	if len(items) == 0 {
		return failed(errEmptyIdentifierList)
	}

	total := 2 * (len(items) - 1)
	values := 0
	for _, item := range items {
		switch v := item.(type) {
		case Fragment:
			if v.err != nil {
				return failed(v.err)
			}
			total += len(v.text)
			values += len(v.values)
		case string:
			if v == "" {
				return failed(errEmptyIdentifierInList)
			}
			if err := validateIdent(v); err != nil {
				return failed(err)
			}
			total += quotedLen(v)
		default:
			return failed(errIdentItemType)
		}
	}

	var b strings.Builder
	b.Grow(total)
	var collected []any
	if values > 0 {
		collected = make([]any, 0, values)
	}
	for i, item := range items {
		if i > 0 {
			b.WriteString(", ")
		}
		switch v := item.(type) {
		case Fragment:
			b.WriteString(v.text)
			collected = append(collected, v.values...)
		case string:
			writeQuotedIdent(&b, v)
		}
	}
	return Fragment{text: b.String(), values: collected}
}
