package sqlb

import (
	"math"
	"reflect"
	"strings"
	"testing"
)

// Envoy's Postgres is the dialect WITHOUT options, and the SQLite-semantics
// options must not leak into it: its like is Postgres's case-sensitive LIKE
// with `\` as the escape, its ilike Unicode ILIKE, its regex POSIX `~`, and
// it has no JSON path. Every byte pinned, so a change to the default is a
// deliberate edit to this test, not a side effect.
func TestPostgresWithoutOptionsIsUnchanged(t *testing.T) {
	for _, c := range []struct{ filter, text, err string }{
		{`{"name":{"like":"%ada%"}}`, `(("name" LIKE $1))`, ""},
		{`{"name":{"ilike":"%ada%"}}`, `(("name" ILIKE $1))`, ""},
		{`{"name":{"regex":"^a"}}`, `(("name" ~ $1))`, ""},
		{`{"config.host":"x"}`, "", "JSON path filters are not supported by this database: config.host"},
		{`{"a":1,"b":{"gte":2}}`, `((("a" = $1) AND ("b" >= $2)))`, ""},
	} {
		f, err := DecodeFilter([]byte(c.filter))
		if err != nil {
			t.Fatal(err)
		}
		clause, err := Postgres.CompileWhere(f)
		if c.err != "" {
			wantErr(t, err, c.err)
			continue
		}
		if err != nil {
			t.Fatal(err)
		}
		wantText(t, clause.Text(), c.text)
	}
	if Postgres.SupportsJSONPath() || !Postgres.SupportsOperator("regex") {
		t.Fatal("Postgres's capabilities changed")
	}
	// With returns a copy; the package value is untouched.
	if _, err := Postgres.With(SQLiteLike, SQLiteJSONPath); err != nil {
		t.Fatal(err)
	}
	if Postgres.sqliteLike || Postgres.sqliteJSON || Postgres.SupportsJSONPath() {
		t.Fatal("With mutated the package's Postgres value")
	}
}

func TestWith(t *testing.T) {
	s, err := SQLite.With(SQLiteLike, SQLiteJSONPath)
	if err != nil || !reflect.DeepEqual(s, SQLite) {
		t.Fatalf("SQLite.With = %+v, %v; want SQLite unchanged", s, err)
	}
	for _, d := range []Dialect{MySQL, MSSQL} {
		if _, err := d.With(SQLiteLike); err == nil {
			t.Errorf("%s.With accepted an option nothing renders there", d.Name())
		}
	}
	if _, err := Postgres.With(flagOption(64)); err == nil {
		t.Error("an unknown option was accepted")
	}
	if _, err := Postgres.With(nil); err == nil {
		t.Error("a nil option was accepted")
	}
	pg, err := Postgres.With(SQLiteLike, SQLiteJSONPath)
	if err != nil {
		t.Fatal(err)
	}
	if !pg.SupportsJSONPath() || pg.SupportsOperator("regex") {
		t.Fatal("the options did not change the capabilities")
	}
	// The exported building blocks refuse a dialect without their option.
	if _, err := Postgres.CompareJSONPath(Ident("c"), "$.a", "=", "x"); err == nil {
		t.Error("CompareJSONPath ran without SQLiteJSONPath")
	}
	if _, err := Postgres.LikeSQLite(Ident("c"), "x", false); err == nil {
		t.Error("LikeSQLite ran without SQLiteLike")
	}
}

// The rendering, byte for byte. What these MEAN is proven against both
// engines by the consumer that has both (gotruto's conformance corpus,
// internal/store/sqlitesemantics_test.go); here the text is pinned so a change
// to it is deliberate.
func TestSQLiteSemanticsRendering(t *testing.T) {
	pg, err := Postgres.With(SQLiteLike, SQLiteJSONPath)
	if err != nil {
		t.Fatal(err)
	}
	f, err := DecodeFilter([]byte(`{"name":{"like":"%Ada%"}}`))
	if err != nil {
		t.Fatal(err)
	}
	clause, err := pg.CompileWhere(f)
	if err != nil {
		t.Fatal(err)
	}
	// The value is scoped once (sqlb_j), then read by SQLite's text rules.
	j := `sqlb_j.j`
	text := `(CASE COALESCE(json_typeof(` + j + `), 'null') WHEN 'null' THEN NULL WHEN 'boolean' THEN ` +
		`(CASE WHEN CAST(` + j + ` AS text) = 'true' THEN '1' ELSE '0' END) WHEN 'number' THEN ` +
		`(CASE WHEN (` + j + ` #>> '{}') ~ '^-?[0-9]+$' AND CAST((` + j + ` #>> '{}') AS numeric) ` +
		`BETWEEN -9223372036854775808 AND 9223372036854775807 THEN CAST(CAST((` + j + ` #>> '{}') AS numeric) AS text) ` +
		`ELSE (` + j + ` #>> '{}') END) ELSE (` + j + ` #>> '{}') END)`
	wantText(t, clause.Text(), `(((SELECT (`+text+`) COLLATE "C" ILIKE CAST($1 AS text) ESCAPE '' `+
		`FROM (SELECT to_json("name") AS j) AS sqlb_j)))`)
	wantValues(t, clause.Values(), "%Ada%")

	// ilike is the same thing: SQLite's ilike is LIKE COLLATE NOCASE.
	f, _ = DecodeFilter([]byte(`{"name":{"ilike":"%Ada%"}}`))
	ilike, err := pg.CompileWhere(f)
	if err != nil {
		t.Fatal(err)
	}
	wantText(t, ilike.Text(), clause.Text())

	// regex is refused, as on D1.
	f, _ = DecodeFilter([]byte(`{"name":{"regex":"^a"}}`))
	_, err = pg.CompileWhere(f)
	wantErr(t, err, "REGEX operator is not supported by the postgres dialect")

	// A JSON path binds each step per use; the comparison reads the class.
	cmp, err := pg.CompareJSONPath(Ident("t.config"), `$.a[0]`, ">", "m")
	if err != nil {
		t.Fatal(err)
	}
	wantText(t, cmp.Text(), `(SELECT (CASE WHEN COALESCE(json_typeof(sqlb_j.j), 'null') = 'null' THEN NULL `+
		`WHEN json_typeof(sqlb_j.j) IN ('number', 'boolean') THEN false `+
		`ELSE (sqlb_j.j #>> '{}') COLLATE "C" > CAST(? AS text) END) `+
		`FROM (SELECT (CAST("t"."config" AS json) -> CAST(? AS text) -> CAST(? AS integer)) AS j) AS sqlb_j)`)
	// The comparison's value first, then the path's steps: text order.
	wantValues(t, cmp.Values(), "m", "a", int64(0))
	if _, err := Q("select 1 where ?", cmp); err != nil {
		t.Fatalf("the fragment does not compose: %v", err)
	}
}

func TestCompareJSONPathAnswersByClass(t *testing.T) {
	pg, _ := Postgres.With(SQLiteJSONPath)
	col := Ident("c")
	for _, c := range []struct {
		op    string
		value any
		has   []string
	}{
		// NULL, NaN and ±Inf bind as NULL (D1): the comparison is NULL.
		{"=", nil, []string{"CAST(NULL AS boolean)"}},
		{"=", math.NaN(), []string{"CAST(NULL AS boolean)"}},
		{"<", math.Inf(1), []string{"CAST(NULL AS boolean)"}},
		// A blob sorts above everything.
		{"<", []byte{1}, []string{"WHEN 'null' THEN NULL ELSE true END"}},
		// A number: text rows answer as text > number.
		{">", 5, []string{"ELSE true END)", "AS double precision"}},
		{"=", true, []string{"ELSE false END)"}},
	} {
		frag, err := pg.CompareJSONPath(col, "$.a", c.op, c.value)
		if err != nil {
			t.Fatalf("%s %v: %v", c.op, c.value, err)
		}
		for _, h := range c.has {
			if !strings.Contains(frag.Text(), h) {
				t.Errorf("%s %v: %s does not contain %q", c.op, c.value, frag.Text(), h)
			}
		}
	}
	// A bound number is compared exactly with an INTEGER literal: the exact
	// decimal of the double, not its shortest spelling.
	frag, err := pg.CompareJSONPath(col, "$.a", "=", float64(1<<60))
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, v := range frag.Values() {
		if v == "1152921504606846976" {
			found = true
		}
	}
	if !found {
		t.Errorf("2^60 is not bound exactly: %v", frag.Values())
	}
	if _, err := pg.CompareJSONPath(col, "$.a", "~", "x"); err == nil {
		t.Error("an unknown operator rendered")
	}
	if _, err := pg.CompareJSONPath(Raw("x = ?"), "$.a", "=", "x"); err == nil {
		t.Error("a column fragment with a placeholder rendered")
	}
}

func TestExactDecimal(t *testing.T) {
	for f, want := range map[float64]string{
		0:                           "0",
		5:                           "5",
		-7.5:                        "-7.5",
		0.1:                         "0.1000000000000000055511151231257827021181583404541015625",
		9007199254740993:            "9007199254740992",
		1e21:                        "1000000000000000000000",
		math.SmallestNonzeroFloat64: "0." + strings.Repeat("0", 323) + "4940656458412465441765687928682213723650598026143247644255856825006755072702087518652998363616359923797965646954457177309266567103559397963987747960107818781263007131903114045278458171678489821036887186360569987307230500063874091535649843873124733972731696151400317153853980741262385655911710266585566867681870395603106249319452715914924553293054565444011274801297099995419319894090804165633245247571478690147267801593552386115501348035264934720193790268107107491703332226844753335720832431936092382893458368060106011506169809753078342277318329247904982524730776375927247874656084778203734469699533647017972677717585125660551199131504891101451037862738167250955837389733598993664809941164205702637090279242767544565229087538682506419718265533447265625",
	} {
		if got := exactDecimal(f); got != want {
			t.Errorf("exactDecimal(%v) = %s, want %s", f, got, want)
		}
	}
}

// The path grammar, against SQLite 3.45's answers (measured with the sqlite3
// shell, 2026-10-31): `.0` on an array is a KEY and finds nothing, `[0]` on an
// object is an INDEX and finds nothing, which is why steps keep their kind.
func TestParseSQLitePath(t *testing.T) {
	for _, c := range []struct {
		path  string
		steps []pathStep
		err   bool
	}{
		{"$", nil, false},
		{"$.a", []pathStep{{kind: stepKey, key: "a"}}, false},
		{"$.a.b", []pathStep{{kind: stepKey, key: "a"}, {kind: stepKey, key: "b"}}, false},
		{"$.a.0", []pathStep{{kind: stepKey, key: "a"}, {kind: stepKey, key: "0"}}, false},
		{"$.a[0]", []pathStep{{kind: stepKey, key: "a"}, {kind: stepIndex, index: 0}}, false},
		{"$.a[12]", []pathStep{{kind: stepKey, key: "a"}, {kind: stepIndex, index: 12}}, false},
		{"$.a[#-1]", []pathStep{{kind: stepKey, key: "a"}, {kind: stepFromEnd, index: 1}}, false},
		{"$.a[#]", []pathStep{{kind: stepKey, key: "a"}, {kind: stepMiss}}, false},
		{"$.a[#-0]", []pathStep{{kind: stepKey, key: "a"}, {kind: stepMiss}}, false},
		{"$.a b", []pathStep{{kind: stepKey, key: "a b"}}, false},
		{"$.a]b", []pathStep{{kind: stepKey, key: "a]b"}}, false},
		{`$."x.y"`, []pathStep{{kind: stepKey, key: "x.y"}}, false},
		{`$."x\"y".z`, []pathStep{{kind: stepKey, key: `x"y`}, {kind: stepKey, key: "z"}}, false},
		{`$."é"`, []pathStep{{kind: stepKey, key: "é"}}, false},
		{"$.a[99999999999]", []pathStep{{kind: stepKey, key: "a"}, {kind: stepMiss}}, false},
		{"$.", nil, true},
		{"$.a.", nil, true},
		{"$..a", nil, true},
		{"$.a[", nil, true},
		{"$.a[x]", nil, true},
		{"$.a[-1]", nil, true},
		{"$.a[0]x", nil, true},
		{`$."unterminated`, nil, true},
		{"a", nil, true},
	} {
		steps, err := parseSQLitePath(c.path)
		if (err != nil) != c.err {
			t.Errorf("%q: err %v, want error %v", c.path, err, c.err)
			continue
		}
		if err == nil && !reflect.DeepEqual(steps, c.steps) {
			t.Errorf("%q = %+v, want %+v", c.path, steps, c.steps)
		}
		if err != nil && !strings.HasPrefix(err.Error(), "bad JSON path: '") {
			t.Errorf("%q: %v is not SQLite's message", c.path, err)
		}
	}
}

func TestSQLiteAffinityRendering(t *testing.T) {
	kinds := map[string]ColumnKind{"name": ColumnText, "count": ColumnInteger, "ratio": ColumnReal,
		"on": ColumnBoolean, "at": ColumnOther}
	pg, err := Postgres.With(SQLiteLike, SQLiteJSONPath,
		SQLiteAffinity(func(c string) ColumnKind { return kinds[c] }))
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		filter, text string
		values       []any
	}{
		// Text against text: as written, so an index still serves it.
		{`{"name":"ada"}`, `(("name" = CAST($1 AS text)))`, []any{"ada"}},
		// Numeric text against a number column is the number (affinity).
		{`{"count":"5"}`, `(("count" = CAST($1 AS bigint)))`, []any{int64(5)}},
		{`{"count":" 5.0 "}`, `(("count" = CAST($1 AS bigint)))`, []any{int64(5)}},
		{`{"count":{"gt":"1.5"}}`, `((CAST("count" AS numeric) > CAST(CAST($1 AS text) AS numeric)))`, []any{"1.5"}},
		{`{"ratio":{"lt":"1"}}`, `(("ratio" < CAST($1 AS double precision)))`, []any{float64(1)}},
		// Other text is above every number: SQLite's answer, not a refusal.
		{`{"count":"abc"}`, `(((CASE WHEN "count" IS NULL THEN NULL ELSE false END)))`, nil},
		{`{"count":{"lt":""}}`, `(((CASE WHEN "count" IS NULL THEN NULL ELSE true END)))`, nil},
		{`{"count":{"gt":"1e400"}}`, `(((CASE WHEN "count" IS NULL THEN NULL ELSE false END)))`, nil},
		// A boolean holds 1 and 0.
		{`{"on":"1"}`, `(("on" = CAST($1 AS boolean)))`, []any{true}},
		{`{"on":{"gte":"2"}}`, `((CAST("on" AS integer) >= CAST(CAST($1 AS text) AS numeric)))`, []any{"2"}},
		{`{"on":"true"}`, `(((CASE WHEN "on" IS NULL THEN NULL ELSE false END)))`, nil},
		// A NUL cannot be in Postgres text; compared byte-wise.
		{`{"name":"a\u0000b"}`, `((convert_to("name", 'UTF8') = CAST($1 AS bytea)))`, []any{[]byte("a\x00b")}},
		// An unknown name is SQLite's string literal, answered in Go.
		{`{"nosuchcol":"x"}`, `((false))`, nil},
		{`{"nosuchcol":"nosuchcol"}`, `((true))`, nil},
		{`{"nosuchcol":{"gt":5}}`, `((true))`, nil},
		{`{"nosuchcol":null}`, `((false))`, nil},
		{`{"nosuchcol":{"ne":null}}`, `((true))`, nil},
		// IN is the OR of the equalities; NOT IN its negation.
		{`{"count":{"in":["1","x",null]}}`,
			`((("count" = CAST($1 AS bigint) OR (CASE WHEN "count" IS NULL THEN NULL ELSE false END) OR CAST(NULL AS boolean))))`,
			[]any{int64(1)}},
		{`{"name":{"nin":["a"]}}`, `((NOT ("name" = CAST($1 AS text))))`, []any{"a"}},
		// The date-time and date columns are left to the driver.
		{`{"at":{"gt":"2026-01-01T00:00:00.000Z"}}`, `(("at" > $1))`, []any{"2026-01-01T00:00:00.000Z"}},
	} {
		f, err := DecodeFilter([]byte(c.filter))
		if err != nil {
			t.Fatal(err)
		}
		clause, err := pg.CompileWhere(f)
		if err != nil {
			t.Fatalf("%s: %v", c.filter, err)
		}
		if clause.Text() != c.text {
			t.Errorf("%s\n  got:  %s\n  want: %s", c.filter, clause.Text(), c.text)
		}
		if !reflect.DeepEqual(clause.Values(), c.values) && !(len(c.values) == 0 && len(clause.Values()) == 0) {
			t.Errorf("%s: values %#v, want %#v", c.filter, clause.Values(), c.values)
		}
	}
	// An unknown name's like binds the literal.
	f, _ := DecodeFilter([]byte(`{"nosuchcol":{"like":"%such%"}}`))
	clause, err := pg.CompileWhere(f)
	if err != nil {
		t.Fatal(err)
	}
	wantText(t, clause.Text(), `(((CAST($1 AS text)) COLLATE "C" ILIKE CAST($2 AS text) ESCAPE ''))`)
	wantValues(t, clause.Values(), "nosuchcol", "%such%")
}

func TestSQLiteCompareValues(t *testing.T) {
	for _, c := range []struct {
		a, b  any
		op    string
		want  bool
		known bool
	}{
		{"b", "a", ">", true, true},
		{"a", 5.0, ">", true, true}, // text above numbers
		{5.0, "a", "<", true, true}, // numbers below text
		{"a", []byte{0}, "<", true, true},
		{"a", nil, "=", false, false},
		{1.0, 1.0, "=", true, true},
		{"é", "z", ">", true, true}, // byte order: 0xC3 > 'z'
	} {
		ac, an, at, _ := classify(c.a)
		bc, bn, bt, _ := classify(c.b)
		got, known := sqliteCompareValues(ac, an, at, bc, bn, bt, c.op)
		if got != c.want || known != c.known {
			t.Errorf("%v %s %v = %v (known %v), want %v (%v)", c.a, c.op, c.b, got, known, c.want, c.known)
		}
	}
}
