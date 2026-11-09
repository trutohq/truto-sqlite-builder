// Port of src/security.test.ts.
//
// Each block corresponds to a finding from the reference's security audit. This
// file is the point of the library, so it is ported case for case and the Go
// subtest names match the reference's `it` names verbatim — a reviewer can diff
// the two lists.
//
// Where a case is unreachable in Go because the type system already forbids it,
// the test says so and asserts the Go-equivalent guarantee instead of being
// dropped. Those are marked TYPE-ENFORCED and counted separately in TRACKER.md.
package sqlb

import (
	"encoding/json"
	"strings"
	"testing"
)

// lookAlike is what a forged fragment can look like from outside this package:
// the same shape, none of the identity.
type lookAlike struct {
	Text   string
	Values []any
}

func TestSecurity_VULN0109_FragmentsAreUnforgeable(t *testing.T) {
	t.Run("rejects a forged { text, values } object in a sql interpolation", func(t *testing.T) {
		forged := lookAlike{Text: "1 OR 1=1", Values: []any{}}
		_, err := Q("SELECT * FROM users WHERE id = ?", forged)
		wantErr(t, err, "Unsupported value type")
	})

	t.Run("rejects a JSON payload that mimics a fragment", func(t *testing.T) {
		var payload any
		wantNoErr(t, json.Unmarshal([]byte(`{"text":"1 UNION SELECT secret FROM users","values":[]}`), &payload))
		_, err := Q("SELECT name FROM users WHERE id = ?", payload)
		wantErr(t, err, "Unsupported value type")
	})

	t.Run("still composes genuine, library-minted fragments safely", func(t *testing.T) {
		condition, err := Q("active = ? AND name LIKE ?", true, "a%")
		wantNoErr(t, err)
		query, err := Q("SELECT * FROM users WHERE ?", condition)
		wantNoErr(t, err)
		wantText(t, query.Text(), "SELECT * FROM users WHERE active = ? AND name LIKE ?")
		wantValues(t, query.Values(), true, "a%")
	})

	t.Run("treats a look-alike object as a value, not SQL", func(t *testing.T) {
		forged := &lookAlike{Text: "DROP TABLE users", Values: nil}
		_, err := Q("?", forged)
		wantErr(t, err, "Unsupported value type")
	})

	// TYPE-ENFORCED. The reference needs a WeakSet because a JS object can be
	// shaped like a fragment. Here Fragment's fields are unexported, so the only
	// Fragment a caller outside this package can build is the zero one — which
	// contributes no text and no values. Forgery is not blocked at runtime, it is
	// impossible at compile time.
	t.Run("an externally constructed Fragment carries no text (TYPE-ENFORCED)", func(t *testing.T) {
		var forged Fragment // all a caller outside the package can make
		query, err := Q("SELECT 1 ?", forged)
		wantNoErr(t, err)
		wantText(t, query.Text(), "SELECT 1 ")
		wantValues(t, query.Values())
	})
}

func TestSecurity_VULN02_JoinSeparators(t *testing.T) {
	frags := func(t *testing.T) []Fragment {
		t.Helper()
		a, err := Q("a = ?", 1)
		wantNoErr(t, err)
		b, err := Q("b = ?", 2)
		wantNoErr(t, err)
		c, err := Q("c = ?", 3)
		wantNoErr(t, err)
		return []Fragment{a, b, c}
	}

	t.Run("supports any structural connector", func(t *testing.T) {
		fs := frags(t)
		for _, sep := range []string{", ", " AND ", " OR ", " UNION ALL ", " AND (1=1) AND "} {
			joined := wantOK(t, Join(fs, sep))
			wantValues(t, joined.Values(), 1, 2, 3)
			if n := len(strings.Split(joined.Text(), sep)); n != 3 {
				t.Errorf("sep %q: split into %d parts, want 3 (text %q)", sep, n, joined.Text())
			}
		}
	})

	t.Run("rejects separators containing string/identifier literal delimiters", func(t *testing.T) {
		fs := frags(t)[:2]
		for _, sep := range []string{" OR '", ` OR "x"`, " OR `x`", " OR [x]"} {
			wantFragErr(t, Join(fs, sep), "Unsafe sql.join() separator")
		}
	})

	t.Run("rejects separators containing comments, terminators, NUL or backslash", func(t *testing.T) {
		fs := frags(t)[:2]
		for _, sep := range []string{" -- ", " /* x */ ", "; ", ` \ `, " \x00 ", " */ "} {
			wantFragErr(t, Join(fs, sep), "Unsafe sql.join() separator")
		}
	})

	t.Run("rejects separators with unbalanced parentheses", func(t *testing.T) {
		fs := frags(t)[:2]
		wantFragErr(t, Join(fs, ") OR (1=1) -- "), "Unsafe sql.join() separator")
		wantFragErr(t, Join(fs, " OR ("), "unbalanced parentheses")
		wantFragErr(t, Join(fs, " )"), "unbalanced parentheses")
	})

	t.Run("supports a Fragment separator with parameterized values", func(t *testing.T) {
		fs := frags(t)[:2]
		sep, err := Q(" OR flag = ? OR ", true)
		wantNoErr(t, err)
		joined := wantOK(t, JoinFragment(fs, sep))
		wantText(t, joined.Text(), "a = ? OR flag = ? OR b = ?")
		wantValues(t, joined.Values(), 1, true, 2)
	})

	// TYPE-ENFORCED. Join takes []Fragment and a string; JoinFragment takes a
	// Fragment. The reference's "rejects non-fragment array items" and "rejects a
	// separator that is neither string nor fragment" cannot be written here.
	t.Run("non-fragment items and separators are rejected by the compiler (TYPE-ENFORCED)", func(t *testing.T) {
		// A Fragment carrying an error poisons the join rather than being spliced.
		poisoned := Ident("not a valid identifier!")
		wantFragErr(t, Join([]Fragment{poisoned}, ", "), "Invalid identifier")
	})
}

func TestSecurity_VULN0304_PlaceholderIntegrity(t *testing.T) {
	t.Run("throws when a raw fragment smuggles a stray placeholder", func(t *testing.T) {
		_, err := Q("SELECT * FROM users WHERE name = ?", Raw("? OR 1=1 --"))
		wantErr(t, err, "Placeholder count (1) does not match bound value count (0)")
	})

	t.Run("throws when raw SQL contains placeholders without supplying values", func(t *testing.T) {
		_, err := Q("WHERE a = ? AND b = ?", Raw("?"), Raw("?"))
		wantErr(t, err, "Placeholder count")
	})

	t.Run("still allows legitimate raw SQL without placeholders", func(t *testing.T) {
		query, err := Q("SELECT * FROM users WHERE created_at > ?", Raw("CURRENT_TIMESTAMP"))
		wantNoErr(t, err)
		wantText(t, query.Text(), "SELECT * FROM users WHERE created_at > CURRENT_TIMESTAMP")
		wantValues(t, query.Values())
	})
}

func TestSecurity_VULN05_IdentCannotBeTricked(t *testing.T) {
	t.Run("rejects a forged fragment inside an identifier array", func(t *testing.T) {
		forged := lookAlike{Text: "secret AS name", Values: []any{}}
		wantFragErr(t, Idents("id", forged), "Array items must be strings or SQL fragments")
	})

	t.Run("still allows explicit, developer-authored Raw expressions", func(t *testing.T) {
		fragment := wantOK(t, Idents("id", Raw("COUNT(*) as total")))
		wantText(t, fragment.Text(), `"id", COUNT(*) as total`)
		wantValues(t, fragment.Values())
	})
}

func TestSecurity_VULN06_DoSLimits(t *testing.T) {
	t.Run("rejects an oversized identifier in Ident()", func(t *testing.T) {
		wantFragErr(t, Ident(strings.Repeat("a", 256)), "Identifier part too long")
	})

	t.Run("rejects an oversized field name in CompileFilter()", func(t *testing.T) {
		_, err := CompileFilter(NewFilter().Eq(strings.Repeat("a", 256), 1))
		wantErr(t, err, "Identifier part too long")
	})

	t.Run("rejects an oversized alias in CompileFilter()", func(t *testing.T) {
		_, err := CompileFilter(NewFilter().Alias(strings.Repeat("a", 256), NewFilter().Eq("x", 1)))
		wantErr(t, err, "Invalid alias identifier")
	})

	t.Run("enforces a maximum compiled-filter length (backstop)", func(t *testing.T) {
		inValues := make([]any, 999)
		for i := range inValues {
			inValues[i] = strconvItoa(i)
		}
		subs := make([]*Filter, 60)
		for i := range subs {
			subs[i] = NewFilter().Where("field_"+strconvItoa(i), NewOps().In(inValues...))
		}
		_, err := CompileFilter(NewFilter().And(subs...))
		wantErr(t, err, "Compiled filter too long")
	})
}

func TestSecurity_VULN07_CompileFilterInterpolatesSafely(t *testing.T) {
	t.Run("returns a fragment usable directly in a query", func(t *testing.T) {
		where, err := CompileFilter(NewFilter().Eq("name", "alice").Where("age", NewOps().Gte(18)))
		wantNoErr(t, err)
		query, err := Q("SELECT * FROM users WHERE ?", where)
		wantNoErr(t, err)
		wantText(t, query.Text(), `SELECT * FROM users WHERE ((("name" = ?) AND ("age" >= ?)))`)
		wantValues(t, query.Values(), "alice", 18)
		if got, want := countByte(query.Text(), '?'), len(query.Values()); got != want {
			t.Errorf("placeholders = %d, values = %d", got, want)
		}
	})

	t.Run("the old footgun (Raw of the filter text) now fails closed", func(t *testing.T) {
		where, err := CompileFilter(NewFilter().Eq("name", "alice"))
		wantNoErr(t, err)
		_, err = Q("SELECT * FROM users WHERE ?", Raw(where.Text()))
		wantErr(t, err, "Placeholder count")
	})

	t.Run("composes with additional interpolated values in the right order", func(t *testing.T) {
		where, err := CompileFilter(NewFilter().Eq("status", "active"))
		wantNoErr(t, err)
		query, err := Q("SELECT * FROM users WHERE ? LIMIT ?", where, 10)
		wantNoErr(t, err)
		wantText(t, query.Text(), `SELECT * FROM users WHERE (("status" = ?)) LIMIT ?`)
		wantValues(t, query.Values(), "active", 10)
	})
}

func TestSecurity_VULN08_StackedQueryDetection(t *testing.T) {
	t.Run("still rejects genuine multi-statement queries", func(t *testing.T) {
		_, err := Q("SELECT 1; DROP TABLE users")
		wantErr(t, err, "Stacked queries are not allowed")
	})

	t.Run("does not flag a semicolon inside a string literal", func(t *testing.T) {
		query, err := Q("SELECT ? AS s", Raw("';'"))
		wantNoErr(t, err)
		wantText(t, query.Text(), "SELECT ';' AS s")
		wantValues(t, query.Values())
	})

	t.Run("does not flag a semicolon hidden inside a comment", func(t *testing.T) {
		query, err := Q("SELECT 1 ?", Raw("/* ; DROP TABLE x */"))
		wantNoErr(t, err)
		wantText(t, query.Text(), "SELECT 1 /* ; DROP TABLE x */")
		wantValues(t, query.Values())
	})

	t.Run("allows a single trailing semicolon (single statement)", func(t *testing.T) {
		query, err := Q("SELECT 1;")
		wantNoErr(t, err)
		wantText(t, query.Text(), "SELECT 1;")
	})
}

func TestSecurity_VULN10_PatternLengthBounded(t *testing.T) {
	t.Run("rejects oversized patterns for each operator", func(t *testing.T) {
		huge := strings.Repeat("a", 1025)
		_, err := CompileFilter(NewFilter().Where("name", NewOps().Like(huge)))
		wantErr(t, err, "LIKE pattern too long")
		_, err = CompileFilter(NewFilter().Where("name", NewOps().ILike(huge)))
		wantErr(t, err, "ILIKE pattern too long")
		_, err = CompileFilter(NewFilter().Where("name", NewOps().Regex(huge)))
		wantErr(t, err, "REGEX pattern too long")
	})

	t.Run("allows patterns within the limit", func(t *testing.T) {
		ok := strings.Repeat("a", 1024)
		_, err := CompileFilter(NewFilter().Where("name", NewOps().Like(ok)))
		wantNoErr(t, err)
	})
}

func TestSecurity_ScanSQLLiteralAndCommentHandling(t *testing.T) {
	t.Run("skips placeholders and structure inside all literal kinds", func(t *testing.T) {
		query, err := Q("SELECT ?, ?, ? ?",
			Raw("'a''b?'"), Raw("`c?`"), Raw("[d?]"), Raw("-- ? trailing"))
		wantNoErr(t, err)
		wantValues(t, query.Values())
		if !strings.Contains(query.Text(), "'a''b?'") {
			t.Errorf("text %q should contain the quoted literal", query.Text())
		}
	})
}

// strconvItoa keeps this file free of a strconv import for one call site.
func strconvItoa(i int) string {
	if i == 0 {
		return "0"
	}
	var buf [20]byte
	pos := len(buf)
	for i > 0 {
		pos--
		buf[pos] = byte(0x30 + i%10)
		i /= 10
	}
	return string(buf[pos:])
}
