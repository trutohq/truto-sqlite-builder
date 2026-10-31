package sqlb

import (
	"strings"
	"testing"
)

func TestNumberPlaceholders(t *testing.T) {
	cases := []struct {
		d       Dialect
		in, out string
		n       int
		err     string
	}{
		{Postgres, `select * from t where a = ? and b in (?,?)`, `select * from t where a = $1 and b in ($2,$3)`, 3, ""},
		{MSSQL, `a = ? or b = ?`, `a = @p1 or b = @p2`, 2, ""},
		{SQLite, `a = ? or b = ?`, `a = ? or b = ?`, 2, ""},
		{Dialect{}, `a = ?`, `a = ?`, 1, ""},
		// Everything Q's integrity check treats as not-code stays as written.
		{Postgres, `x = '?' and "?" = ? -- ?` + "\n" + `and /* ? */ [?] = ?`,
			`x = '?' and "?" = $1 -- ?` + "\n" + `and /* ? */ [?] = $2`, 2, ""},
		{Postgres, `a = 'it''s ?' and b = ?`, `a = 'it''s ?' and b = $1`, 1, ""},
		{Postgres, "`?` = ?", "`?` = $1", 1, ""},
		// A statement numbered by hand passes through untouched.
		{Postgres, `a = $1 and b = $2`, `a = $1 and b = $2`, 0, ""},
		{Postgres, `a = 'x'`, `a = 'x'`, 0, ""},
		// Mixing the two numberings is refused, not renumbered.
		{Postgres, `a = $1 and b = ?`, "", 0, "mixes"},
		{MSSQL, `a = @p1 and b = ?`, "", 0, "mixes"},
		// A `$1` inside a literal is not a marker.
		{Postgres, `a = '$1' and b = ?`, `a = '$1' and b = $1`, 1, ""},
		// The regex literal the SQLite-semantics rendering writes.
		{Postgres, `x ~ '^-?[0-9]+$' and y = ?`, `x ~ '^-?[0-9]+$' and y = $1`, 1, ""},
	}
	for _, c := range cases {
		out, n, err := c.d.NumberPlaceholders(c.in)
		if c.err != "" {
			if err == nil || !strings.Contains(err.Error(), c.err) {
				t.Errorf("%s %q: err %v, want %q", c.d.Name(), c.in, err, c.err)
			}
			continue
		}
		if err != nil || out != c.out || n != c.n {
			t.Errorf("%s %q = %q, %d, %v; want %q, %d", c.d.Name(), c.in, out, n, err, c.out, c.n)
		}
	}
}

// The numbered statement's `?` count is Q's count, by construction: both are
// scanSQL. Reversing the numbering gives back the input.
func FuzzNumberPlaceholders(f *testing.F) {
	for _, s := range []string{`a = ? and b = '?'`, `"x?" = ? -- ?`, `[?]?`, `/* ? */?`, `'unterminated ?`, `$1 ?`} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		out, n, err := Postgres.NumberPlaceholders(s)
		want, _ := scanSQL(s)
		if err != nil {
			if !strings.Contains(err.Error(), "mixes") {
				t.Fatalf("%q: %v", s, err)
			}
			return
		}
		if n != want {
			t.Fatalf("%q: numbered %d, scanSQL counts %d", s, n, want)
		}
		if n == 0 {
			if out != s {
				t.Fatalf("%q: nothing to number, but the statement changed to %q", s, out)
			}
			return
		}
		// Every code `?` is gone, every other `?` is still there, and the
		// markers are 1…n in order.
		if left, _ := scanSQL(out); left != 0 {
			t.Fatalf("%q: %q still has %d code placeholders", s, out, left)
		}
		if strings.Count(out, "?") != strings.Count(s, "?")-n {
			t.Fatalf("%q: %q changed a `?` that was not code", s, out)
		}
		last := -1
		for i := 1; i <= n; i++ {
			j := strings.Index(out[last+1:], "$"+itoa(i))
			if j < 0 {
				t.Fatalf("%q: %q has no $%d after offset %d", s, out, i, last)
			}
			last += 1 + j
		}
	})
}

func TestRender(t *testing.T) {
	frag, err := Q("select * from t where a = ? and b = ?", "x", 2)
	if err != nil {
		t.Fatal(err)
	}
	text, values, err := Postgres.Render(frag)
	if err != nil || text != "select * from t where a = $1 and b = $2" || len(values) != 2 {
		t.Fatalf("Render = %q, %v, %v", text, values, err)
	}
	if _, _, err := Postgres.Render(failed(errStackedQuery)); err == nil {
		t.Fatal("a failed fragment rendered")
	}
	// A fragment Q never builds: text and values disagree.
	bad := Fragment{text: "a = ? and b = ?", values: []any{1}}
	if _, _, err := Postgres.Render(bad); err == nil {
		t.Fatal("a misaligned fragment rendered")
	}
}

// Dialect.CompileFilter is the dialect's clause with `?` markers: numbering
// it gives back CompileWhere's text and values exactly, for every dialect.
func TestDialectCompileFilterIsTheClauseUnnumbered(t *testing.T) {
	pgSQLite, err := Postgres.With(SQLiteLike, SQLiteJSONPath)
	if err != nil {
		t.Fatal(err)
	}
	filters := []string{
		`{"status":"ACTIVE"}`,
		`{"and":[{"a":1},{"or":[{"b":2},{"c":null}]}]}`,
		`{"$t2":{"col":{"gte":100}},"x":{"ne":null}}`,
		`{"n":{"in":[1,2,3],"nin":[4]},"p":{"like":"%x%"},"q":{"ilike":"%y%"}}`,
		`{"e":{"exists":0}}`,
		`{"cfg.a":"x","cfg.b.c":{"gt":1,"lt":5,"ne":null},"cfg.d":{"in":["a",1,null]}}`,
		`{"cfg.a":{"like":"%x%"},"cfg.b":{"exists":true}}`,
	}
	for _, d := range []Dialect{Postgres, MSSQL, MySQL, pgSQLite} {
		for _, raw := range filters {
			f, err := DecodeFilter([]byte(raw))
			if err != nil {
				t.Fatal(err)
			}
			clause, cerr := d.CompileWhere(f)
			frag, ferr := d.CompileFilter(f)
			if (cerr == nil) != (ferr == nil) {
				t.Fatalf("%s %s: CompileWhere %v, CompileFilter %v", d.Name(), raw, cerr, ferr)
			}
			if cerr != nil {
				continue
			}
			if strings.Contains(frag.Text(), "$1") || strings.Contains(frag.Text(), "@p1") {
				t.Errorf("%s %s: the fragment is numbered: %s", d.Name(), raw, frag.Text())
			}
			text, values, err := d.Render(frag)
			if err != nil {
				t.Fatalf("%s %s: %v", d.Name(), raw, err)
			}
			// MSSQL binds a JSON path once in the clause and once per use in
			// the fragment, so its text differs there; it must still compose.
			// The SQLite-semantics rendering binds per use in both.
			if strings.Contains(raw, "cfg.") && d.jsonExtract != "" {
				if _, err := Q("select ?", frag); err != nil {
					t.Errorf("%s %s: the fragment does not compose: %v", d.Name(), raw, err)
				}
				continue
			}
			if text != clause.Text() {
				t.Errorf("%s %s\n  clause:   %s\n  fragment: %s", d.Name(), raw, clause.Text(), text)
			}
			assertValues(t, clause.Values(), values)
		}
	}
}

// On SQLite the method is the package function, misalignment included (S-01).
func TestSQLiteCompileFilterKeepsTheReferenceMisalignment(t *testing.T) {
	f, err := DecodeFilter([]byte(`{"a.b":{"gt":1,"lt":2}}`))
	if err != nil {
		t.Fatal(err)
	}
	frag, err := SQLite.CompileFilter(f)
	if err != nil {
		t.Fatal(err)
	}
	if countByte(frag.Text(), '?') == len(frag.Values()) {
		t.Fatal("SQLite.CompileFilter corrected the reference's misalignment")
	}
	// MSSQL's fragment binds the path at each use, so it composes.
	m, err := MSSQL.CompileFilter(f)
	if err != nil {
		t.Fatal(err)
	}
	wantText(t, m.Text(), `((JSON_VALUE([a], ?) > ? AND JSON_VALUE([a], ?) < ?))`)
	wantValues(t, m.Values(), "$.b", int64(1), "$.b", int64(2))
}
