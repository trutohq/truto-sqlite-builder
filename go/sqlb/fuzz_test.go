package sqlb

import (
	"strings"
	"testing"
	"unicode/utf8"
)

// safeGrammar is the complete set of characters the compiler is allowed to emit
// into a WHERE clause.
//
// Every value is bound, every identifier is ASCII-validated before quoting, and
// the only literal text is a fixed vocabulary of operators and keywords. So the
// output alphabet is closed, and anything outside it means input reached the
// code channel. This is a stronger statement than "no injection in these
// examples" and it is what the fuzzers below assert.
const safeGrammar = `ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789_ ()"?,.<>=$`

// assertClauseIsSafe fails if a compiled clause escaped the closed alphabet or
// looks like more than one statement.
func assertClauseIsSafe(t *testing.T, text string) {
	t.Helper()
	for i := range len(text) {
		if !strings.ContainsRune(safeGrammar, rune(text[i])) {
			t.Fatalf("compiled clause contains %q, outside the closed alphabet: %q",
				text[i], text)
		}
	}
	if _, stacked := scanSQL(text); stacked {
		t.Fatalf("compiled clause looks like a stacked query: %q", text)
	}
	if strings.Contains(text, "--") || strings.Contains(text, "/*") {
		t.Fatalf("compiled clause contains a comment marker: %q", text)
	}
}

// FuzzCompileFilterJSON is the main security fuzzer: arbitrary JSON in, and
// either an error or a clause drawn from the closed alphabet out.
func FuzzCompileFilterJSON(f *testing.F) {
	seeds := []string{
		`{"status":"ACTIVE"}`,
		`{"a":{"gt":1,"lt":2}}`,
		`{"a.b":{"in":[1,2,3]}}`,
		`{"and":[{"a":1},{"or":[{"b":2},{"c":null}]}]}`,
		`{"$t2":{"col":{"gte":100}}}`,
		`{"$t2 JOIN x --":{"col":1}}`,
		`{"name'":"x"}`,
		`{"a":{"like":"%x%"},"b":{"exists":false}}`,
		`{"a":{"regex":"^\\+1"}}`,
		`{"users; DROP TABLE users":1}`,
		`{"a":[1,2]}`,
		`{"and":[]}`,
		`{"a":{"unknown":1}}`,
		`{"$":{"a":1}}`,
		`{"a.b":{"gt":1,"lt":2}}`,
	}
	for _, s := range seeds {
		f.Add(s)
	}

	f.Fuzz(func(t *testing.T, raw string) {
		frag, err := CompileFilterJSON([]byte(raw))
		if err != nil {
			if frag.Text() != "" {
				t.Fatalf("failed compile returned text %q", frag.Text())
			}
			return
		}
		assertClauseIsSafe(t, frag.Text())

		// The one place the reference emits a clause whose placeholders and values
		// disagree is a JSON path with several operators; composing must then fail
		// closed rather than bind the wrong columns.
		q, qerr := Q("SELECT * FROM t WHERE ?", frag)
		if qerr != nil {
			if !strings.Contains(qerr.Error(), "Placeholder count") {
				t.Fatalf("unexpected composition error %v for clause %q", qerr, frag.Text())
			}
			return
		}
		if got, want := countByte(q.Text(), '?'), len(q.Values()); got != want {
			t.Fatalf("composed query has %d placeholders and %d values", got, want)
		}
	})
}

// FuzzIdent asserts that a successful Ident yields nothing but quoted ASCII
// identifier parts, and a failed one yields no text at all.
func FuzzIdent(f *testing.F) {
	for _, s := range []string{
		"users", "u.name", "schema.table.column", "_x", "SELECT",
		"", "1a", "a-b", "a b", `a"b`, "a'b", "a;b", "a.", ".a", "a..b",
		"é", "\U0001F600", "a\x00b", strings.Repeat("a", 256),
	} {
		f.Add(s)
	}

	f.Fuzz(func(t *testing.T, name string) {
		frag := Ident(name)
		if frag.Err() != nil {
			if frag.Text() != "" {
				t.Fatalf("failed Ident(%q) returned text %q", name, frag.Text())
			}
			return
		}
		text := frag.Text()
		if len(frag.Values()) != 0 {
			t.Fatalf("Ident(%q) bound values", name)
		}
		// The output must be "part" or "part"."part"…, with parts drawn from the
		// ASCII identifier class. Anything else means an unquoted byte escaped.
		for _, part := range strings.Split(text, ".") {
			if len(part) < 2 || part[0] != '"' || part[len(part)-1] != '"' {
				t.Fatalf("Ident(%q) = %q: part %q is not quoted", name, text, part)
			}
			if !isSimpleIdent(part[1 : len(part)-1]) {
				t.Fatalf("Ident(%q) = %q: part %q is not an ASCII identifier", name, text, part)
			}
		}
		// Round-trip: re-quoting the unquoted form is stable.
		if again := Ident(name); again.Text() != text {
			t.Fatalf("Ident(%q) is not deterministic", name)
		}
	})
}

// FuzzScanSQL checks the scanner is total and agrees with a straightforward
// reimplementation of the reference's two-step version (build a code-only
// string, then run the regex over it).
func FuzzScanSQL(f *testing.F) {
	for _, s := range []string{
		"SELECT 1", "SELECT ?", "SELECT 1; DROP TABLE t", "SELECT 1;",
		"SELECT 'a''b?'", "-- ?\n?", "/* ? */?", "[a?]", "`c?`",
		"SELECT 1;", "SELECT 1;\ufeff", "'unterminated", "/*unterminated",
		"[unterminated", "??;?", "", ";", ";;", "; ",
	} {
		f.Add(s)
	}

	f.Fuzz(func(t *testing.T, s string) {
		gotPlaceholders, gotStacked := scanSQL(s)
		wantPlaceholders, wantCode := scanSQLReference(s)
		wantStacked := stackedRegexReference(wantCode)

		if gotPlaceholders != wantPlaceholders {
			t.Fatalf("placeholders = %d, reference says %d for %q",
				gotPlaceholders, wantPlaceholders, s)
		}
		if gotStacked != wantStacked {
			t.Fatalf("stacked = %v, reference says %v for %q (code %q)",
				gotStacked, wantStacked, s, wantCode)
		}
	})
}

// scanSQLReference is a direct, unfused transliteration of src/sql.ts:319 —
// slow and allocating, kept only so the fused production version has something
// independent to be checked against.
func scanSQLReference(text string) (placeholderCount int, code string) {
	var b strings.Builder
	i, n := 0, len(text)
	for i < n {
		c := text[i]
		if c == '-' && i+1 < n && text[i+1] == '-' {
			i += 2
			for i < n && text[i] != '\n' {
				i++
			}
			continue
		}
		if c == '/' && i+1 < n && text[i+1] == '*' {
			i += 2
			for i < n && !(text[i] == '*' && i+1 < n && text[i+1] == '/') {
				i++
			}
			i += 2
			continue
		}
		if c == '\'' || c == '"' || c == '`' {
			quote := c
			i++
			for i < n {
				if text[i] == quote {
					if i+1 < n && text[i+1] == quote {
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
			for i < n && text[i] != ']' {
				i++
			}
			i++
			continue
		}
		if c == '?' {
			placeholderCount++
		}
		if c < utf8.RuneSelf {
			b.WriteByte(c)
			i++
		} else {
			r, size := utf8.DecodeRuneInString(text[i:])
			b.WriteRune(r)
			i += size
		}
	}
	return placeholderCount, b.String()
}

// stackedRegexReference is /;[\s\S]*\S/ spelled out: a semicolon with any
// non-whitespace character after it.
func stackedRegexReference(code string) bool {
	semi := strings.IndexByte(code, ';')
	if semi < 0 {
		return false
	}
	for _, r := range code[semi+1:] {
		if !isJSSpaceRune(r) {
			return true
		}
	}
	return false
}

// FuzzQValuesNeverReachText pins the central property of the whole library: for
// a fixed constant format, no value a caller supplies can change the SQL.
func FuzzQValuesNeverReachText(f *testing.F) {
	for _, s := range []string{
		"John", "'; DROP TABLE users; --", "?", "", "\x00", "\\", "';",
		strings.Repeat("?", 100), "\U0001F600",
	} {
		f.Add(s, int64(0))
	}

	const format = "SELECT * FROM users WHERE name = ? AND id = ?"
	f.Fuzz(func(t *testing.T, s string, n int64) {
		q, err := Q(format, s, n)
		if err != nil {
			t.Fatalf("Q rejected a plain value pair: %v", err)
		}
		if q.Text() != format {
			t.Fatalf("value changed the SQL: %q", q.Text())
		}
		if len(q.Values()) != 2 || q.Values()[0] != s || q.Values()[1] != n {
			t.Fatalf("values = %#v", q.Values())
		}
	})
}

// FuzzJoinSeparator asserts a separator that passes validation cannot introduce
// a literal, a comment or a second statement into the joined text.
func FuzzJoinSeparator(f *testing.F) {
	for _, s := range []string{", ", " AND ", " OR ", " UNION ALL ", "", " ) OR ( ",
		"'", "--", "/*", ";", "\\", "(", ")", " AND (1=1) AND "} {
		f.Add(s)
	}

	f.Fuzz(func(t *testing.T, sep string) {
		frags := []Fragment{Ident("a"), Ident("b")}
		joined := Join(frags, sep)
		if joined.Err() != nil {
			if joined.Text() != "" {
				t.Fatalf("failed Join returned text %q", joined.Text())
			}
			return
		}
		text := joined.Text()
		// Nothing that could open a literal, start a comment or end a statement
		// may have survived validation. The double quotes in the output come from
		// the two Ident fragments, never from the separator.
		for _, forbidden := range []string{"'", `"`, "`", "[", "]", ";", `\`, "\x00", "--", "/*", "*/"} {
			if strings.Contains(sep, forbidden) {
				t.Fatalf("separator %q containing %q was accepted", sep, forbidden)
			}
		}
		if _, stacked := scanSQL(text); stacked {
			t.Fatalf("join produced a stacked query: %q", text)
		}
		depth := 0
		for i := range len(text) {
			switch text[i] {
			case '(':
				depth++
			case ')':
				depth--
			}
			if depth < 0 {
				t.Fatalf("join produced unbalanced parentheses: %q", text)
			}
		}
		if depth != 0 {
			t.Fatalf("join produced unbalanced parentheses: %q", text)
		}
	})
}
