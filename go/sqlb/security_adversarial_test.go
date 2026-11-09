// Adversarial tests beyond the reference suite.
//
// The reference's security.test.ts is the floor, not the proof. These probe the
// places where Go's string, escaping, whitespace and regexp rules differ from
// JavaScript's, because any input Go ACCEPTS and the TypeScript REJECTS is a
// security finding, not a test failure.
//
// Every expectation here was confirmed by executing the reference on
// 2026-09-06; see docs/go-port/wiki/GOTCHAS.md.
package sqlb

import (
	"strings"
	"testing"
)

// TestAdversarial_WhitespaceAfterSemicolon pins the stacked-query whitespace
// class.
//
// STACKED_QUERY_REGEX is /;[\s\S]*\S/ (src/constants.ts:23), so what counts as
// whitespace after a semicolon decides whether a second statement is caught.
// ECMAScript's \s and Go's unicode.IsSpace disagree in BOTH directions, and one
// direction is a vulnerability:
//
//   - U+0085 NEXT LINE: unicode.IsSpace says true, ECMAScript says false. A Go
//     implementation using unicode.IsSpace would ACCEPT `SELECT 1;<U+0085>DROP …`
//     that the reference rejects.
//   - U+FEFF ZWNBSP: the reverse. Go stricter, which is safe but divergent.
func TestAdversarial_WhitespaceAfterSemicolon(t *testing.T) {
	rejected := []struct{ name, suffix string }{
		{"U+0085 NEXT LINE", "\u0085"},
		{"U+180E MONGOLIAN VOWEL SEPARATOR", "\u180e"},
		{"U+2060 WORD JOINER", "\u2060"},
		{"U+200B ZERO WIDTH SPACE", "\u200b"},
		{"U+2800 BRAILLE PATTERN BLANK", "\u2800"},
		{"U+3164 HANGUL FILLER", "\u3164"},
		{"ordinary text", "x"},
		{"a second statement", " DROP TABLE users"},
	}
	for _, tc := range rejected {
		t.Run("rejects stacked query followed by "+tc.name, func(t *testing.T) {
			_, err := Q("?", Raw("SELECT 1;"+tc.suffix))
			wantErr(t, err, "Stacked queries are not allowed")
		})
	}

	accepted := []struct{ name, suffix string }{
		{"nothing", ""},
		{"space", " "},
		{"tab", "\t"},
		{"newline", "\n"},
		{"carriage return", "\r"},
		{"vertical tab", "\v"},
		{"form feed", "\f"},
		{"U+00A0 NO-BREAK SPACE", "\u00a0"},
		{"U+1680 OGHAM SPACE MARK", "\u1680"},
		{"U+2000 EN QUAD", "\u2000"},
		{"U+200A HAIR SPACE", "\u200a"},
		{"U+2028 LINE SEPARATOR", "\u2028"},
		{"U+2029 PARAGRAPH SEPARATOR", "\u2029"},
		{"U+202F NARROW NO-BREAK SPACE", "\u202f"},
		{"U+205F MEDIUM MATHEMATICAL SPACE", "\u205f"},
		{"U+3000 IDEOGRAPHIC SPACE", "\u3000"},
		{"U+FEFF ZERO WIDTH NO-BREAK SPACE", "\ufeff"},
	}
	for _, tc := range accepted {
		t.Run("accepts a trailing semicolon followed by "+tc.name, func(t *testing.T) {
			_, err := Q("?", Raw("SELECT 1;"+tc.suffix))
			wantNoErr(t, err)
		})
	}
}

// TestAdversarial_IdentifierCharset probes characters a Unicode-aware \w would
// admit. SIMPLE_IDENTIFIER_REGEX is explicitly ASCII (src/constants.ts:30), and
// Go's regexp \w is ASCII too — but a reimplementation reaching for
// unicode.IsLetter would let all of these through, straight into a quoted
// identifier.
func TestAdversarial_IdentifierCharset(t *testing.T) {
	for _, name := range []string{
		"na\u00efve",         // naive with i-diaeresis
		"\u0438\u043c\u044f", // Cyrillic
		"\u540d\u524d",       // CJK
		"a\u200bb",           // zero width space
		"a\ufeffb",           // ZWNBSP
		"\uff41",             // fullwidth a
		"\U0001d41a",         // mathematical bold small a
		"\u00e1",             // a-acute
		"\u0661\u0662",       // Arabic-Indic digits
		"a\u00a0b",           // NBSP
		"\u01c5",             // titlecase digraph
		"\u02b0",             // modifier letter small h
		"\u1d00",             // latin letter small capital a
		"\ufb00",             // ff ligature
		"\u2170",             // small roman numeral one
		"\U0001d7d9",         // mathematical double-struck one
		"\u2460",             // circled digit one
		"a\u0000b",           // embedded NUL
	} {
		t.Run(name, func(t *testing.T) {
			wantFragErr(t, Ident(name), "Invalid identifier")
		})
	}
}

// TestAdversarial_LengthLimitsAreUTF16 pins the unit the limits count in.
//
// The reference compares against JavaScript's .length, which is UTF-16 code
// units. Counting bytes would reject input the reference accepts (divergent but
// safe); counting runes would accept input the reference rejects (not safe).
// Verified against the reference: 51199 astral characters are 102398 units and
// pass, 51201 are 102402 and do not.
func TestAdversarial_LengthLimitsAreUTF16(t *testing.T) {
	t.Run("astral text just under the query limit is accepted", func(t *testing.T) {
		_, err := Q("?", Raw(strings.Repeat("\U0001F600", 51199)))
		wantNoErr(t, err)
	})
	t.Run("astral text just over the query limit is rejected", func(t *testing.T) {
		_, err := Q("?", Raw(strings.Repeat("\U0001F600", 51201)))
		wantErr(t, err, "Query too long")
	})
	t.Run("a LIKE pattern of exactly 1024 UTF-16 units is accepted", func(t *testing.T) {
		_, err := CompileFilter(NewFilter().Where("n", NewOps().Like(strings.Repeat("\U0001F600", 512))))
		wantNoErr(t, err)
	})
	t.Run("a LIKE pattern of 1026 UTF-16 units is rejected", func(t *testing.T) {
		_, err := CompileFilter(NewFilter().Where("n", NewOps().Like(strings.Repeat("\U0001F600", 513))))
		wantErr(t, err, "LIKE pattern too long")
	})
}

// TestAdversarial_PoisonedFragmentContributesNothing checks the sticky-error
// design fails closed. A Fragment that failed to build must never contribute
// partial text to anything that consumes it.
func TestAdversarial_PoisonedFragmentContributesNothing(t *testing.T) {
	bad := Ident("users; DROP TABLE users")
	wantFragErr(t, bad, "Invalid identifier")
	wantText(t, bad.Text(), "")

	list := Idents("id", bad, "name")
	wantFragErr(t, list, "Invalid identifier")
	wantText(t, list.Text(), "")

	joined := Join([]Fragment{Ident("id"), bad}, ", ")
	wantFragErr(t, joined, "Invalid identifier")
	wantText(t, joined.Text(), "")

	_, err := Q("SELECT ? FROM t", list)
	wantErr(t, err, "Invalid identifier")

	// And as a Join separator.
	sep := Ident("bad-sep")
	wantFragErr(t, JoinFragment([]Fragment{Ident("a"), Ident("b")}, sep), "Invalid identifier")
}

// TestAdversarial_FilterFieldsAreAlwaysQuoted checks that no COLUMN name
// reaches the SQL text unvalidated.
func TestAdversarial_FilterFieldsAreAlwaysQuoted(t *testing.T) {
	for _, field := range []string{
		"users; DROP TABLE users; --",
		`name" OR "1"="1`,
		"name'",
		"name)",
		"name--",
		"a..b",
		".b",
		"a.",
		"json_extract(x,'$.a')",
		"a b",
		"a\u0000b",
		"1.b",
	} {
		t.Run(field, func(t *testing.T) {
			if _, err := CompileFilter(NewFilter().Eq(field, 1)); err == nil {
				t.Fatalf("field %q was accepted", field)
			}
			if _, err := CompileFilter(NewFilter().Where(field, NewOps().Gt(1))); err == nil {
				t.Fatalf("field %q was accepted as an operator target", field)
			}
		})
	}
}

// TestAdversarial_JSONPathTailIsBoundNotSpliced pins the one place a field name
// is only half-validated.
//
// For `a.b`, only the column half goes through identifier validation; the tail
// is used verbatim (src/filter.ts:70). That is safe HERE because the tail is
// BOUND as a parameter rather than rendered into the SQL. It is the same shape
// that is NOT safe in src/common/AbstractService.ts:177-182, where the
// equivalent path is concatenated into the query text -- see
// gotruto/internal/query/doc.go. Confirmed against the reference 2026-09-06:
// every one of these compiles, with the hostile text landing only in the value
// list.
func TestAdversarial_JSONPathTailIsBoundNotSpliced(t *testing.T) {
	for _, field := range []string{
		"a.b--",
		"a.'b",
		"a. b",
		"a.b.'c",
		"a.b ",
		"a.b\u0000",
		"a.b;DROP TABLE users",
		`a.b" OR "1"="1`,
		"a.*/x",
	} {
		t.Run(field, func(t *testing.T) {
			r, err := CompileFilter(NewFilter().Eq(field, 1))
			wantNoErr(t, err)
			wantText(t, r.Text(), `((json_extract("a", ?) = ?))`)
			tail := field[strings.IndexByte(field, '.')+1:]
			wantValues(t, r.Values(), "$."+tail, 1)
		})
	}
}

// TestAdversarial_AliasIsNeverQuotedButAlwaysValidated is the sharpest edge in
// the filter compiler: an alias is emitted UNQUOTED as `t2."col"`
// (src/filter.ts:166-168), so its validation is the only thing between a caller
// and raw SQL.
func TestAdversarial_AliasIsNeverQuotedButAlwaysValidated(t *testing.T) {
	for _, alias := range []string{
		"t2 JOIN secrets s ON 1=1 --",
		`t2"`,
		"t2.t3",
		"t2-",
		"",
		"1t",
		"t2;",
		"t2 ",
		"t 2",
		"t2'",
		"t2)",
		"t2\u0000",
		"na\u00efve",
	} {
		t.Run(alias, func(t *testing.T) {
			_, err := CompileFilter(NewFilter().Alias(alias, NewFilter().Eq("c", 1)))
			wantErr(t, err, "Invalid alias identifier")
		})
	}
}

// TestAdversarial_ValuesNeverReachTheTextChannel is a blanket property: for a
// large corpus of hostile strings, the assembled text must be byte-identical to
// the parameterised form and the hostile string must appear only in Values.
func TestAdversarial_ValuesNeverReachTheTextChannel(t *testing.T) {
	hostile := []string{
		"'; DROP TABLE users; --",
		"' OR 1=1 --",
		"\" OR 1=1 --",
		"` OR 1=1 --",
		"] OR 1=1 --",
		"\x00",
		"\x00' OR 1=1--",
		"/* */; SELECT 1",
		strings.Repeat("?", 64),
		"?",
		"$1",
		":name",
		"@p1",
		"\u2028; DROP TABLE users",
		"\\'",
		"\\\\';DROP TABLE users;--",
	}
	for _, s := range hostile {
		t.Run(s, func(t *testing.T) {
			q, err := Q("SELECT * FROM users WHERE name = ? AND note = ?", s, s)
			wantNoErr(t, err)
			wantText(t, q.Text(), "SELECT * FROM users WHERE name = ? AND note = ?")
			wantValues(t, q.Values(), s, s)

			f, err := CompileFilter(NewFilter().Eq("name", s).Where("note", NewOps().Like(s).Ne(s)))
			wantNoErr(t, err)
			if strings.Contains(f.Text(), s) && s != "?" {
				t.Fatalf("hostile string leaked into filter text: %q", f.Text())
			}
		})
	}
}
