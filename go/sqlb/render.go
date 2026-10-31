package sqlb

import "fmt"

// NumberPlaceholders rewrites every code-position `?` in a finished statement
// into this dialect's bind marker — `$1, $2, …` for [Postgres], `@p1, …` for
// [MSSQL] — and reports how many it rewrote. On a `?` dialect the statement is
// returned as it is, with its count.
//
// # Why this exists
//
// [Q] composes only `?` statements, because a numbered marker carries its
// position in its own text and two independently numbered pieces cannot be
// spliced without renumbering (see [Clause]). A caller that builds with Q and
// executes against a numbered database therefore needs exactly one step at the
// end: number the finished statement once. Doing that with a scanner of its
// own is how two programs come to disagree about which `?` is a placeholder
// and which is inside a string; this is sqlb's own scan ([scanSQL]), so the
// `?` that Q counted and the `?` that gets numbered are the same characters.
//
// What counts as code is what Q's integrity check uses: `--` and block
// comments, '…', "…" and `…` with a doubled quote as the escape, and […] are
// skipped. Postgres's dollar-quoted strings and E'…' escapes are NOT
// recognised, so a statement that uses them must not also carry `?`.
//
// A statement that already carries this dialect's numbered markers in code
// position AND a `?` is refused: mixing the two numberings is never what a
// caller meant, and renumbering it would bind values to the wrong slots. A
// statement with no `?` at all is returned unchanged, so a hand-numbered one
// passes through.
func (d Dialect) NumberPlaceholders(sql string) (string, int, error) {
	d = d.resolve()
	if !d.numbered() {
		n, _ := scanSQL(sql)
		return sql, n, nil
	}
	if !containsByte(sql, '?') {
		return sql, 0, nil
	}
	n, _, at, mixed := scanSQLAt(sql, true)
	if n == 0 {
		return sql, 0, nil
	}
	if mixed {
		return "", 0, fmt.Errorf("sqlb: the statement mixes `?` and numbered placeholders")
	}
	out := make([]byte, 0, len(sql)+2*n)
	last := 0
	for i, pos := range at {
		out = append(out, sql[last:pos]...)
		out = d.appendPlaceholder(out, i+1)
		last = pos + 1
	}
	out = append(out, sql[last:]...)
	return string(out), n, nil
}

// Render is [Dialect.NumberPlaceholders] for a [Fragment] built with [Q]: the
// statement in this dialect's markers and its values in placeholder order.
//
// The count is checked against the values, so a fragment whose text and
// values disagree — which Q itself never builds — is an error here rather than
// a value bound to the wrong column.
func (d Dialect) Render(f Fragment) (string, []any, error) {
	if f.err != nil {
		return "", nil, f.err
	}
	text, n, err := d.NumberPlaceholders(f.text)
	if err != nil {
		return "", nil, err
	}
	if n != len(f.values) {
		return "", nil, fmt.Errorf("sqlb: the statement has %d placeholders and %d values", n, len(f.values))
	}
	return text, f.values, nil
}

// CompileFilter is [CompileFilter] in this dialect's spelling — its quote,
// its `ilike`, its JSON paths, its options — but with `?` markers, so the
// result is a [Fragment] that composes inside [Q] like any other. Number the
// finished statement once, with [Dialect.Render] or
// [Dialect.NumberPlaceholders].
//
// On [SQLite] (and the zero Dialect) it is exactly the package-level
// [CompileFilter], including the reference's JSON-path misalignment (S-01).
// On a numbered dialect a JSON path is bound once per use, because that is
// what the numbered [Clause] already means: `$1` repeated in three clauses is
// one value used three times, and the `?` spelling of the same statement needs
// the value three times.
func (d Dialect) CompileFilter(f *Filter) (Fragment, error) {
	d = d.resolve()
	if d.name == SQLite.name && !d.numbered() {
		return CompileFilter(f)
	}
	q := d
	q.placeholders = placeholderQuestion
	clause, err := q.compileWhere(f, d.numbered())
	if err != nil {
		return Fragment{}, err
	}
	return Fragment{text: clause.text, values: clause.values}, nil
}

func containsByte(s string, b byte) bool {
	for i := 0; i < len(s); i++ {
		if s[i] == b {
			return true
		}
	}
	return false
}
