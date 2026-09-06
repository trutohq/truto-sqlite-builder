// Package sqlb is a safe, zero-dependency SQL builder for SQLite.
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
// See DIVERGENCES.md in docs/go-port/wiki for the full list of behavioural
// differences, and README.md in the parent directory for the rest.
package sqlb
