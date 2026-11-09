package sqlb

import "strconv"

// A Dialect is how one SQL flavour spells the three things that differ between
// databases without changing which rows match: the bind marker, the identifier
// quote, and the two operators (`ilike`, `regex`) that have no single spelling.
//
// The default is [SQLite] and it is the zero value, so a Dialect nobody set is
// the behaviour this package had before dialects existed — byte for byte.
//
// # Why this is an opaque value and not a struct of functions
//
// goenvoy/internal/sqlfilter/sqlfilter.go:20-28 models a dialect as three
// exported func fields. That is the natural Go design and it is the wrong one
// HERE, because it reopens the boundary this package exists to close: a caller
// can write `Dialect{QuoteIdent: func(s string) string { return s }}` and put
// arbitrary attacker-controlled text into the query, with no `Raw` call to
// grep for. Dialect's fields are unexported and the only values are the four
// below, so quoting and placeholder rendering stay this package's.
//
// # What a Dialect deliberately does NOT decide
//
//   - Whether an unvalidated identifier is escaped or rejected. sqlb rejects
//     anything outside [A-Za-z_][A-Za-z0-9_]* (see validateIdent); sqlfilter
//     escapes by doubling. That is a security posture, not a syntax, and making
//     it a dialect flag would let a caller pick the weaker one per database.
//   - What an empty filter means. Every dialect compiles one to no WHERE clause
//     — "match everything" — and none of them refuses (D-0032). That changes
//     ROWS, not syntax, so it is decided once for the package rather than per
//     database. See [Clause.IsEmpty] and [ErrEmptyFilter].
type Dialect struct {
	name         string
	placeholders placeholderStyle
	quote        quoteStyle

	// jsonExtract is the function that reads a JSON path, e.g. "json_extract".
	// Empty means the database has no equivalent and a dotted field is an error.
	jsonExtract string

	// ilike and regex are the operators whose spelling is not shared. A zero
	// opClause means the dialect cannot express the operator at all.
	ilike opClause
	regex opClause
}

type placeholderStyle uint8

const (
	placeholderQuestion placeholderStyle = iota // ?      sqlite, mysql
	placeholderDollar                           // $1     postgres
	placeholderAtP                              // @p1    mssql
)

type quoteStyle uint8

const (
	quoteDouble   quoteStyle = iota // "name"  sqlite, postgres
	quoteBacktick                   // `name`  mysql
	quoteBracket                    // [name]  mssql
)

// opClause renders one operator as pre + fieldExpr + mid + placeholder + post.
// Three pieces are enough for every spelling that exists here, including
// MSSQL's LOWER(x) LIKE LOWER(?).
type opClause struct {
	pre, mid, post string
}

func (c opClause) supported() bool { return c.mid != "" }

// The dialects. Adding one is adding a value here; nothing else in the package
// knows a database name.
var (
	// SQLite is the default and the reference's own flavour. Its ilike is
	// `LIKE ? COLLATE NOCASE` and its regex is `REGEXP ?`, which needs the
	// regexp extension registered on the connection — the reference emits it
	// regardless (src/filter.ts:271).
	SQLite = Dialect{
		name:         "sqlite",
		placeholders: placeholderQuestion,
		quote:        quoteDouble,
		jsonExtract:  "json_extract",
		ilike:        opClause{mid: " LIKE ", post: " COLLATE NOCASE"},
		regex:        opClause{mid: " REGEXP "},
	}

	// Postgres numbers its placeholders and has native ILIKE and ~.
	//
	// No JSON path support, matching sqlfilter (Postgres.JSONExtract is nil,
	// sqlfilter.go:40-43). Postgres CAN do it with #>> but the path syntax is
	// '{a,b}', not '$.a.b', so it is a translation rather than a spelling — see
	// ENVOY-MIGRATION.md.
	Postgres = Dialect{
		name:         "postgres",
		placeholders: placeholderDollar,
		quote:        quoteDouble,
		ilike:        opClause{mid: " ILIKE "},
		regex:        opClause{mid: " ~ "},
	}

	// MySQL quotes with backticks. Its LIKE is already case-insensitive under
	// the usual collations, so ilike is a plain LIKE — which is also what
	// sqlfilter does (sqlfilter.go:169-170 maps both to LIKE).
	//
	// No JSON path support, matching sqlfilter (MySQL.JSONExtract is nil).
	// MySQL's JSON_EXTRACT takes the same '$.a.b' syntax SQLite does, so this
	// is the cheapest gap in the set to close; it is left open only because
	// closing it would change what Envoy does today.
	MySQL = Dialect{
		name:         "mysql",
		placeholders: placeholderQuestion,
		quote:        quoteBacktick,
		ilike:        opClause{mid: " LIKE "},
		regex:        opClause{mid: " REGEXP "},
	}

	// MSSQL brackets its identifiers and numbers its placeholders @p1, @p2.
	//
	// Numbering makes bind ORDER part of the query: a dialect that emitted a
	// bare `?` here would bind every value to the wrong column the moment two
	// appear (sqlfilter/mssql_test.go:31-50).
	//
	// ilike is LOWER(x) LIKE LOWER(?), which is what the MSSQL connector
	// already does (mssqlconn/filter.go:210-212). There is no regex operator in
	// T-SQL, so `regex` is an error rather than a wrong query.
	MSSQL = Dialect{
		name:         "mssql",
		placeholders: placeholderAtP,
		quote:        quoteBracket,
		jsonExtract:  "JSON_VALUE",
		ilike:        opClause{pre: "LOWER(", mid: ") LIKE LOWER(", post: ")"},
	}
)

// Name reports the dialect's name, for error messages and logs.
func (d Dialect) Name() string { return d.resolve().name }

// resolve maps the zero Dialect onto SQLite.
//
// The zero value being useful is the Go rule, and here it is also the safe one:
// a Dialect a caller forgot to set behaves exactly as this package did before
// dialects existed, rather than emitting nothing for a quote character.
func (d Dialect) resolve() Dialect {
	if d.name == "" {
		return SQLite
	}
	return d
}

// SupportsOperator reports whether the dialect can express a filter operator.
// Every dialect supports the structural ones (`and`, `or`, `exists`, the
// comparisons); only `regex` and JSON-path fields vary.
func (d Dialect) SupportsOperator(op string) bool {
	d = d.resolve()
	switch op {
	case "ilike":
		return d.ilike.supported()
	case "regex":
		return d.regex.supported()
	}
	return validOperators[op]
}

// SupportsJSONPath reports whether a dotted field name compiles to a JSON
// lookup rather than an error.
func (d Dialect) SupportsJSONPath() bool { return d.resolve().jsonExtract != "" }

// appendPlaceholder writes the bind marker for the n-th bound value, 1-based.
func (d Dialect) appendPlaceholder(dst []byte, n int) []byte {
	switch d.placeholders {
	case placeholderDollar:
		dst = append(dst, '$')
		return strconv.AppendInt(dst, int64(n), 10)
	case placeholderAtP:
		dst = append(dst, '@', 'p')
		return strconv.AppendInt(dst, int64(n), 10)
	}
	return append(dst, '?')
}

// numbered reports whether a placeholder carries its position in its text,
// which is what makes a fragment un-composable — see [Q].
func (d Dialect) numbered() bool { return d.placeholders != placeholderQuestion }

func (q quoteStyle) chars() (open, close byte) {
	switch q {
	case quoteBacktick:
		return '`', '`'
	case quoteBracket:
		return '[', ']'
	}
	return '"', '"'
}
