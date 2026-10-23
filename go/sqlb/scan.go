package sqlb

import "unicode/utf8"

// isJSSpaceASCII and isJSSpaceRune implement ECMAScript's \s, which is
// WhiteSpace ∪ LineTerminator.
//
// This is written out by hand rather than deferring to unicode.IsSpace because
// the two disagree, and one direction of the disagreement is a vulnerability:
// unicode.IsSpace(U+0085) is true while ECMAScript's \s does not match it, so a
// Go implementation built on unicode.IsSpace would accept `SELECT 1;<U+0085>DROP
// TABLE users` — a stacked query the reference rejects. In the other direction
// U+FEFF is ECMAScript whitespace and not Go's.
//
// Set: TAB, LF, VT, FF, CR, SP, NBSP, ZWNBSP, the Zs category, LS and PS.
// Notably absent: U+0085 NEL and U+180E, which Go and older Unicode
// respectively call space.
func isJSSpaceASCII(c byte) bool {
	switch c {
	case '\t', '\n', '\v', '\f', '\r', ' ':
		return true
	}
	return false
}

func isJSSpaceRune(r rune) bool {
	if r < utf8.RuneSelf {
		return isJSSpaceASCII(byte(r))
	}
	switch r {
	case 0x00a0, // NO-BREAK SPACE
		0x1680,         // OGHAM SPACE MARK
		0x2028, 0x2029, // LINE / PARAGRAPH SEPARATOR
		0x202f, // NARROW NO-BREAK SPACE
		0x205f, // MEDIUM MATHEMATICAL SPACE
		0x3000, // IDEOGRAPHIC SPACE
		0xfeff: // ZERO WIDTH NO-BREAK SPACE
		return true
	}
	return r >= 0x2000 && r <= 0x200a // EN QUAD … HAIR SPACE
}

// lengthUTF16 counts UTF-16 code units, which is what JavaScript's String
// .length reports and therefore the unit every limit in the reference is
// expressed in. Counting bytes would reject input the reference accepts;
// counting runes would accept input it rejects.
func lengthUTF16(s string) int {
	n := 0
	for _, r := range s {
		n++
		if r > 0xffff {
			n++
		}
	}
	return n
}

// exceedsUTF16 reports whether s is longer than max UTF-16 code units, and how
// long it actually is when it is.
//
// The byte length is an upper bound on the UTF-16 length for any valid UTF-8
// string — a 2- or 3-byte sequence is one code unit and a 4-byte sequence is
// two — so the common case needs no scan at all.
func exceedsUTF16(s string, max int) (int, bool) {
	if len(s) <= max {
		return 0, false
	}
	n := lengthUTF16(s)
	return n, n > max
}

// scanSQL walks assembled SQL once and reports how many true placeholders it
// contains and whether it holds a stacked query. Port of scanSql
// (src/sql.ts:319) fused with the STACKED_QUERY_REGEX test at src/sql.ts:431.
//
// The reference materialises a "code-only" copy of the query with literals and
// comments stripped, then runs /;[\s\S]*\S/ over it. Building that copy is the
// single largest allocation on the hot path and nothing else ever reads it, so
// the semicolon test is folded into the same pass: after the first
// code-position semicolon, any code-position non-whitespace character means a
// second statement.
//
// Placeholders and semicolons inside string literals, quoted identifiers,
// bracket identifiers and comments are invisible to both, as in the reference.
func scanSQL(s string) (placeholders int, stacked bool) {
	i, n := 0, len(s)
	seenSemicolon := false

	for i < n {
		c := s[i]

		// Line comment: -- … <newline>
		if c == '-' && i+1 < n && s[i+1] == '-' {
			i += 2
			for i < n && s[i] != '\n' {
				i++
			}
			continue
		}

		// Block comment: /* … */
		if c == '/' && i+1 < n && s[i+1] == '*' {
			i += 2
			for i < n && !(s[i] == '*' && i+1 < n && s[i+1] == '/') {
				i++
			}
			i += 2
			continue
		}

		// Quoted string or quoted identifier, with the SQL convention that the
		// quote character is escaped by doubling it.
		if c == '\'' || c == '"' || c == '`' {
			quote := c
			i++
			for i < n {
				if s[i] == quote {
					if i+1 < n && s[i+1] == quote {
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

		// Bracket-quoted identifier: [ … ]
		if c == '[' {
			i++
			for i < n && s[i] != ']' {
				i++
			}
			i++
			continue
		}

		// Code position.
		if c < utf8.RuneSelf {
			if c == '?' {
				placeholders++
			}
			if seenSemicolon {
				if !stacked && !isJSSpaceASCII(c) {
					stacked = true
				}
			} else if c == ';' {
				seenSemicolon = true
			}
			i++
			continue
		}

		r, size := utf8.DecodeRuneInString(s[i:])
		if seenSemicolon && !stacked && !isJSSpaceRune(r) {
			stacked = true
		}
		i += size
	}

	return placeholders, stacked
}
