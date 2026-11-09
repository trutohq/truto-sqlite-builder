// Package sqlb is a safe SQL builder for SQLite, Postgres, MySQL and SQL Server.
//
// It is a Go port of @truto/sqlite-builder (TypeScript). The trust model is
// unchanged: the TEXT of a query is code and must originate from the developer;
// interpolated values are data and are always bound as parameters.
//
// # The API differs from the TypeScript, deliberately
//
// The TypeScript entry point is a tagged template:
//
//	sql`SELECT * FROM ${sql.ident(table)} WHERE id = ${id}`
//
// Go has no tagged templates, so the shape had to change. The Go entry point is
// [Q], whose first argument is a compile-time constant and whose interpolation
// slots are written as `?`:
//
//	q, err := sqlb.Q("SELECT * FROM ? WHERE id = ?", sqlb.Ident(table), id)
//	// q.Text()   == `SELECT * FROM "users" WHERE id = ?`
//	// q.Values() == []any{id}
//
// A slot filled with a [Fragment] splices that fragment's text and values; a
// slot filled with anything else becomes a bound parameter. That is exactly what
// `${}` does in the TypeScript.
//
// The first argument's type is unexported, which means Go will only accept an
// untyped string constant there. A `string` variable — anything that could carry
// user input — does not compile. That is a stronger guarantee than the
// TypeScript has: it moves "the code channel is developer-authored" from a
// convention to a compiler check.
//
// [Raw] remains the single explicit escape hatch, exactly as sql.raw() is in the
// TypeScript. Whatever string reaches it becomes SQL verbatim.
//
// # Filters, JSON and dialects
//
// A filter arriving as JSON is decoded by [DecodeFilter], which is a thin layer
// over github.com/trutohq/jsorder — this package does not carry a JSON decoder
// of its own, because the organisation already has one and two of those is one
// duplicated decision.
//
// [CompileFilter] compiles for SQLite and returns a composable [Fragment].
// [Dialect.CompileWhere] compiles for any of [SQLite], [Postgres], [MySQL] and
// [MSSQL] and returns a [Clause], which is deliberately not composable: a
// numbered placeholder carries its absolute position, so splicing two of them
// would silently bind every value to the wrong column.
//
// A Dialect changes only how SQL is spelled. Everything that decides which ROWS
// match is the same in all four, and — apart from the empty filter below — is
// the reference's.
//
// # An empty filter means every row
//
// A filter with no conditions compiles to no WHERE clause and no error, so the
// statement returns the whole table (D-0032). The TypeScript throws instead;
// this is the one deliberate behavioural divergence in the package.
//
// ⚠ On a DELETE or an UPDATE that is every row in the table, and the library
// will not refuse for you. Guard destructive call sites explicitly with
// [Clause.IsEmpty], [Fragment.IsEmpty] or Filter.Len. [ErrEmptyFilter] is still
// exported so a caller can return it itself, and still fires for a NESTED empty
// filter such as `{"and":[{}]}`.
//
// See DIVERGENCES.md in docs/go-port/wiki for the full list of behavioural
// differences, and README.md in the parent directory for the rest.
package sqlb
