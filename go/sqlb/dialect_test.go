package sqlb

import (
	"strings"
	"testing"
)

// Dialects exist so goenvoy/internal/sqlfilter can be deleted (D-0024). What is
// under test here is ONLY what a dialect changes — the bind marker, the
// identifier quote, and the three constructs with no shared spelling. Operator
// SEMANTICS are the reference's and are covered by the ported suites; if a
// dialect ever changed which rows match, that would be the bug.

func TestDialectSpelling(t *testing.T) {
	cases := []struct {
		name    string
		dialect Dialect
		filter  string
		text    string
		values  []any
		err     string
	}{
		// --- the default is SQLite, unchanged -------------------------------
		{
			name: "the zero Dialect is SQLite", dialect: Dialect{},
			filter: `{"name":"ada","age":{"gte":18}}`,
			text:   `((("name" = ?) AND ("age" >= ?)))`, values: []any{"ada", int64(18)},
		},

		// --- quoting --------------------------------------------------------
		{
			name: "postgres double-quotes", dialect: Postgres,
			filter: `{"name":"ada"}`,
			text:   `(("name" = $1))`, values: []any{"ada"},
		}, {
			name: "mysql backticks", dialect: MySQL,
			filter: `{"name":"ada"}`,
			text:   "((`name` = ?))", values: []any{"ada"},
		}, {
			name: "mssql brackets", dialect: MSSQL,
			filter: `{"name":"ada"}`,
			text:   `(([name] = @p1))`, values: []any{"ada"},
		},

		// --- placeholder numbering -----------------------------------------
		{
			// Ported from sqlfilter/mssql_test.go:31-50. The IN list consumes
			// @p1..@p3, so the sibling condition MUST be @p4. An off-by-one binds
			// the wrong value into the wrong column and the query still runs.
			name: "mssql numbers every placeholder across an IN list", dialect: MSSQL,
			filter: `{"status":{"in":["a","b","c"]},"active":1}`,
			text:   `((([status] IN (@p1,@p2,@p3)) AND ([active] = @p4)))`,
			values: []any{"a", "b", "c", int64(1)},
		}, {
			name: "postgres numbers every placeholder across an IN list", dialect: Postgres,
			filter: `{"status":{"in":["a","b","c"]},"active":1}`,
			text:   `((("status" IN ($1,$2,$3)) AND ("active" = $4)))`,
			values: []any{"a", "b", "c", int64(1)},
		},

		// --- ilike ----------------------------------------------------------
		{
			name: "sqlite ilike is LIKE COLLATE NOCASE", dialect: SQLite,
			filter: `{"name":{"ilike":"%ada%"}}`,
			text:   `(("name" LIKE ? COLLATE NOCASE))`, values: []any{"%ada%"},
		}, {
			name: "postgres ilike is native", dialect: Postgres,
			filter: `{"name":{"ilike":"%ada%"}}`,
			text:   `(("name" ILIKE $1))`, values: []any{"%ada%"},
		}, {
			// sqlfilter.go:169-170 maps ilike to a plain LIKE for MySQL, whose
			// default collations are already case-insensitive.
			name: "mysql ilike is a plain LIKE", dialect: MySQL,
			filter: `{"name":{"ilike":"%ada%"}}`,
			text:   "((`name` LIKE ?))", values: []any{"%ada%"},
		}, {
			// mssqlconn/filter.go:210-212 spells it this way already.
			name: "mssql ilike lowercases both sides", dialect: MSSQL,
			filter: `{"name":{"ilike":"%ada%"}}`,
			text:   `((LOWER([name]) LIKE LOWER(@p1)))`, values: []any{"%ada%"},
		},

		// --- regex ----------------------------------------------------------
		{
			name: "postgres regex is ~", dialect: Postgres,
			filter: `{"name":{"regex":"^a"}}`,
			text:   `(("name" ~ $1))`, values: []any{"^a"},
		}, {
			name: "mysql regex is REGEXP", dialect: MySQL,
			filter: `{"name":{"regex":"^a"}}`,
			text:   "((`name` REGEXP ?))", values: []any{"^a"},
		}, {
			// T-SQL has no regex operator. Erroring is the only safe answer:
			// downgrading it to LIKE would return a different set of rows with
			// nothing to show that it had.
			name: "mssql cannot spell regex and says so", dialect: MSSQL,
			filter: `{"name":{"regex":"^a"}}`,
			err:    "REGEX operator is not supported by the mssql dialect",
		},

		// --- JSON paths -----------------------------------------------------
		{
			name: "mssql spells json_extract JSON_VALUE", dialect: MSSQL,
			filter: `{"config.host":"example.com"}`,
			text:   `((JSON_VALUE([config], @p1) = @p2))`,
			values: []any{"$.host", "example.com"},
		}, {
			// sqlfilter.go:115-117 — Postgres.JSONExtract is nil there and the
			// message is this one, verbatim, so a task already handling it keeps
			// matching after the swap.
			name: "postgres has no JSON path support", dialect: Postgres,
			filter: `{"config.host":"example.com"}`,
			err:    "JSON path filters are not supported by this database: config.host",
		}, {
			name: "mysql has no JSON path support", dialect: MySQL,
			filter: `{"config.host":"example.com"}`,
			err:    "JSON path filters are not supported by this database: config.host",
		},

		// --- the alias is unquoted in every dialect -------------------------
		{
			// src/filter.ts:166-168 emits `t2."col"`, not `"t2"."col"`, and
			// isValidAlias is the only thing making that safe (S-H5). Dialects do
			// not change it: a bare [A-Za-z_][A-Za-z0-9_]* is legal unquoted
			// everywhere.
			name: "mysql qualifies with a bare alias", dialect: MySQL,
			filter: `{"$t2":{"col":{"gte":100}}}`,
			text:   "((t2.`col` >= ?))", values: []any{int64(100)},
		}, {
			name: "mssql qualifies with a bare alias", dialect: MSSQL,
			filter: `{"$t2":{"col":{"gte":100}}}`,
			text:   `((t2.[col] >= @p1))`, values: []any{int64(100)},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f, err := DecodeFilter([]byte(tc.filter))
			if err != nil {
				t.Fatal(err)
			}
			clause, err := tc.dialect.CompileWhere(f)
			if tc.err != "" {
				if err == nil {
					t.Fatalf("want error %q, got %q", tc.err, clause.Text())
				}
				if !strings.Contains(err.Error(), tc.err) {
					t.Fatalf("error\n  got:  %s\n  want: %s", err, tc.err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if clause.Text() != tc.text {
				t.Errorf("text\n  got:  %s\n  want: %s", clause.Text(), tc.text)
			}
			assertValues(t, clause.Values(), tc.values)
		})
	}
}

// SQLite.CompileWhere and the package-level CompileFilter must be the same
// compiler, or the dialect path becomes a second implementation of the thing
// this package exists to have one of.
func TestSQLiteDialectMatchesTheDefaultPath(t *testing.T) {
	for _, raw := range []string{
		`{"status":"ACTIVE"}`,
		`{"and":[{"a":1},{"or":[{"b":2},{"c":null}]}]}`,
		`{"a.b":{"gt":1,"lt":2}}`,
		`{"$t2":{"col":{"gte":100}},"x":{"ne":null}}`,
		`{"n":{"in":[1,2,3],"nin":[4]},"p":{"like":"%x%"},"q":{"ilike":"%y%"},"r":{"regex":"^z"}}`,
		`{"e":{"exists":0}}`,
	} {
		f, err := DecodeFilter([]byte(raw))
		if err != nil {
			t.Fatal(err)
		}
		frag, ferr := CompileFilter(f)
		clause, cerr := SQLite.CompileWhere(f)
		if (ferr == nil) != (cerr == nil) {
			t.Fatalf("%s: CompileFilter err=%v, SQLite.CompileWhere err=%v", raw, ferr, cerr)
		}
		if ferr != nil {
			continue
		}
		if frag.Text() != clause.Text() {
			t.Errorf("%s\n  CompileFilter:      %s\n  SQLite.CompileWhere: %s",
				raw, frag.Text(), clause.Text())
		}
		assertValues(t, clause.Values(), frag.Values())
	}
}

// The reference binds a JSON path ONCE while rendering json_extract(col, ?) into
// every clause, so a multi-operator path filter emits more placeholders than it
// has values (S-01, D-0108). That is a positional-placeholder defect and the
// numbered dialects do not have it — $1 repeated three times refers to the same
// bound value.
//
// This is the one place a dialect changes something other than spelling, so it
// is pinned in both directions.
func TestJSONPathMisalignmentIsPositionalOnly(t *testing.T) {
	f, err := DecodeFilter([]byte(`{"a.b":{"gt":1,"lt":2}}`))
	if err != nil {
		t.Fatal(err)
	}

	sqlite, err := SQLite.CompileWhere(f)
	if err != nil {
		t.Fatal(err)
	}
	if want := `((json_extract("a", ?) > ? AND json_extract("a", ?) < ?))`; sqlite.Text() != want {
		t.Fatalf("sqlite text\n  got:  %s\n  want: %s", sqlite.Text(), want)
	}
	if markers, values := strings.Count(sqlite.Text(), "?"), len(sqlite.Values()); markers == values {
		t.Fatalf("the reference's misalignment was silently corrected: %d markers, %d values",
			markers, values)
	}

	mssql, err := MSSQL.CompileWhere(f)
	if err != nil {
		t.Fatal(err)
	}
	want := `((JSON_VALUE([a], @p1) > @p2 AND JSON_VALUE([a], @p1) < @p3))`
	if mssql.Text() != want {
		t.Fatalf("mssql text\n  got:  %s\n  want: %s", mssql.Text(), want)
	}
	// Three DISTINCT markers, three values — correct by construction, not by a
	// fix. @p1 is reused because the path is bound once and referred to twice.
	for _, marker := range []string{"@p1", "@p2", "@p3"} {
		if !strings.Contains(mssql.Text(), marker) {
			t.Errorf("mssql clause is missing %s", marker)
		}
	}
	if len(mssql.Values()) != 3 {
		t.Errorf("mssql values %#v, want three", mssql.Values())
	}
}

func TestDialectIdentQuoting(t *testing.T) {
	cases := []struct {
		dialect Dialect
		name    string
		want    string
	}{
		{SQLite, "users", `"users"`},
		{SQLite, "u.name", `"u"."name"`},
		{Postgres, "u.name", `"u"."name"`},
		{MySQL, "users", "`users`"},
		{MySQL, "u.name", "`u`.`name`"},
		{MSSQL, "users", `[users]`},
		{MSSQL, "u.name", `[u].[name]`},
		{Dialect{}, "users", `"users"`},
	}
	for _, tc := range cases {
		got := tc.dialect.Ident(tc.name)
		if got.Err() != nil {
			t.Fatalf("%s.Ident(%q): %v", tc.dialect.Name(), tc.name, got.Err())
		}
		if got.Text() != tc.want {
			t.Errorf("%s.Ident(%q) = %s, want %s", tc.dialect.Name(), tc.name, got.Text(), tc.want)
		}
	}

	idents := MSSQL.Idents("id", "name")
	if idents.Err() != nil {
		t.Fatal(idents.Err())
	}
	if want := "[id], [name]"; idents.Text() != want {
		t.Errorf("MSSQL.Idents = %s, want %s", idents.Text(), want)
	}
}

// Every dialect rejects the same identifiers. The charset is a security posture,
// not a syntax, and sqlfilter's choice to ESCAPE rather than reject
// (sqlfilter.go:33) is deliberately not offered as a dialect option — see the
// Dialect doc comment.
func TestDialectIdentifierPolicyIsNotADialectSetting(t *testing.T) {
	hostile := []string{
		`a"; DROP TABLE users; --`,
		"a`; DROP TABLE users; --",
		`a]; DROP TABLE users; --`,
		`we][rd`,
		`naïve`, `имя`, `ａ`,
		`a b`, `a-b`, `a;b`, `1abc`, ``,
	}
	for _, d := range []Dialect{SQLite, Postgres, MySQL, MSSQL} {
		for _, name := range hostile {
			if got := d.Ident(name); got.Err() == nil {
				t.Errorf("%s.Ident(%q) succeeded with %s — every dialect must REJECT, not escape",
					d.Name(), name, got.Text())
			}
			f := NewFilter().Eq(name, 1)
			if _, err := d.CompileWhere(f); err == nil {
				t.Errorf("%s.CompileWhere on field %q succeeded", d.Name(), name)
			}
		}
	}
}

// If validateIdent ever let a delimiter through, appendQuotedIdent would become
// an injection in whichever dialect uses it — it does no escaping because it has
// never needed to. This is the tripwire for that.
func TestAdversarial_QuotingNeedsNoEscapeInAnyDialect(t *testing.T) {
	for b := 0; b < 256; b++ {
		name := "a" + string(rune(b)) + "b"
		if err := validateIdent(name); err == nil {
			switch byte(b) {
			case '"', '`', '[', ']':
				t.Fatalf("validateIdent accepted %q, which contains a dialect delimiter", name)
			}
		}
	}
	// And the positive half: an accepted identifier contains only bytes that no
	// dialect treats specially.
	for _, name := range []string{"users", "u.name", "_x9", "A1"} {
		if err := validateIdent(name); err != nil {
			t.Fatalf("validateIdent(%q): %v", name, err)
		}
		if strings.ContainsAny(name, "\"`[]") {
			t.Fatalf("%q would need escaping", name)
		}
	}
}

func TestDialectCapabilityReporting(t *testing.T) {
	if !SQLite.SupportsJSONPath() || !MSSQL.SupportsJSONPath() {
		t.Error("sqlite and mssql have a JSON accessor")
	}
	if Postgres.SupportsJSONPath() || MySQL.SupportsJSONPath() {
		t.Error("postgres and mysql are wired without one, matching sqlfilter")
	}
	if !SQLite.SupportsOperator("regex") || !Postgres.SupportsOperator("regex") {
		t.Error("sqlite and postgres can spell regex")
	}
	if MSSQL.SupportsOperator("regex") {
		t.Error("T-SQL has no regex operator")
	}
	for _, d := range []Dialect{SQLite, Postgres, MySQL, MSSQL} {
		for _, op := range []string{"gt", "gte", "lt", "lte", "ne", "in", "nin", "like", "ilike", "exists", "and", "or"} {
			if !d.SupportsOperator(op) {
				t.Errorf("%s should support %s: every dialect shares the row semantics", d.Name(), op)
			}
		}
		if d.SupportsOperator("nope") {
			t.Errorf("%s claims to support a made-up operator", d.Name())
		}
	}
}

// An empty filter compiles to no WHERE clause in every dialect (D-0032), which
// is what sqlfilter already did and what the reference does NOT do — it throws.
// This is the divergence that changes rows rather than syntax, so it is pinned
// per dialect rather than once: a dialect must not be able to opt out of it in
// either direction.
func TestEmptyFilterCompilesToNoWhereClauseInEveryDialect(t *testing.T) {
	for _, d := range []Dialect{SQLite, Postgres, MySQL, MSSQL} {
		clause, err := d.CompileWhere(NewFilter())
		if err != nil {
			t.Fatalf("%s: an empty filter errored: %v", d.Name(), err)
		}
		if !clause.IsEmpty() || clause.Text() != "" {
			t.Errorf("%s: text = %q, want empty", d.Name(), clause.Text())
		}
		if len(clause.Values()) != 0 {
			t.Errorf("%s: values = %#v, want none", d.Name(), clause.Values())
		}
		if clause.Dialect().Name() != d.resolve().Name() {
			t.Errorf("%s: empty clause lost its dialect: %s", d.Name(), clause.Dialect().Name())
		}

		// A NESTED empty filter is still an error, and still the exported
		// sentinel. Emitting nothing there would produce `()` or silently drop a
		// conjunct, and the reference's error is the useful answer.
		if _, err := d.CompileWhere(NewFilter().And(NewFilter())); err != ErrEmptyFilter {
			t.Errorf("%s: nested empty filter err = %v, want ErrEmptyFilter", d.Name(), err)
		}
		if _, err := d.CompileWhere(NewFilter().Eq("a", 1).Alias("x", NewFilter())); err != ErrEmptyFilter {
			t.Errorf("%s: empty alias block err = %v, want ErrEmptyFilter", d.Name(), err)
		}
	}
}

// The hazard D-0032 creates, pinned so it cannot be discovered in production.
// Nothing between the caller and the database objects to either of these.
func TestEmptyFilterHazards(t *testing.T) {
	t.Run("an empty filter spliced after WHERE leaves a bare WHERE", func(t *testing.T) {
		where, err := CompileFilter(NewFilter())
		wantNoErr(t, err)
		q, err := Q("SELECT * FROM t WHERE ?", where)
		wantNoErr(t, err)
		wantText(t, q.Text(), "SELECT * FROM t WHERE ")
	})

	t.Run("JSON that decodes to no conditions means the whole table", func(t *testing.T) {
		// Every one of these is attacker-reachable as a request body.
		for _, body := range []string{`{}`, `[]`, `{"$notanobject":"x"}`} {
			f, err := DecodeFilter([]byte(body))
			wantNoErr(t, err)
			where, err := CompileFilter(f)
			wantNoErr(t, err)
			if !where.IsEmpty() {
				t.Errorf("%s compiled to %q, want an empty clause", body, where.Text())
			}
		}
	})
}
