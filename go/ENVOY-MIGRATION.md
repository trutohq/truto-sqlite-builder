# Replacing `goenvoy/internal/sqlfilter` with `sqlb`

**Status: the `sqlb` side is done. The Envoy side is not started, and is yours.**

Written 2026-09-06 for someone who has read neither repository. Everything here
is cited; nothing is remembered.

---

## What this is

Envoy's SQL connectors compile the JSON `filter` argument a task writes into a
SQL `WHERE` clause. Three separate Go implementations of that do this today, all
of them described in their own comments as ports of the same TypeScript library:

| Where | Lines | Doc comment |
|---|---|---|
| `goenvoy/internal/sqlfilter/sqlfilter.go` | 246 | *"Port of @truto/sqlite-builder's compileFilter."* (`sqlfilter.go:1-10`) |
| `goenvoy/internal/connector/sqliteconn/sqlite.go:463-599` | ~136 | *"Port of compileFilter (@truto/sqlite-builder)."* (`sqlite.go:465`) |
| `goenvoy/internal/connector/mssqlconn/filter.go` | 259 | port of `src/connectors/mssql.ts`, **a different reference** (`filter.go:15-31`) |

`sqlb` is the fourth, and it is the one verified against the TypeScript by
executing both engines over a generated corpus on every test run (180,000 cases
over this session alone, zero mismatches). The decision to converge on it is `D-0024` in
`truto/docs/go-port/wiki/DECISIONS.md`.

The window for this is open because **Envoy has no customers on its Go path
yet**, so its behaviour is still free to change. That closes.

## Scope: two of the three, not all three

**In scope: `internal/sqlfilter` and `sqliteconn`'s private `compileFilter`.**
These are near-identical copies of each other — same `validOperators` map, same
`fieldExpression`, same `compileFieldCondition`, same errors — and both are
ports of `@truto/sqlite-builder`. `sqlb` replaces both.

**Out of scope: `mssqlconn/filter.go`.** It looks like a fourth copy and is not.
Its own comment (`filter.go:15-31`) lists five ways it deliberately disagrees
with `@truto/sqlite-builder`, because it is a port of `src/connectors/mssql.ts`
in the Envoy TypeScript, which is a separate hand-written implementation:

- `and`/`or` supported there, rejected in `sqlfilter`
- a dotted field is a **schema qualifier** `[meta].[kind]`, not a JSON path
- `exists` does **not** short-circuit its siblings
- an unknown operator is **silently ignored**, so a typo'd `gte` degrades the
  whole filter to `1=1`
- an empty `in` array emits `IN ()` and lets the server reject it

Folding that into `sqlb` would change what a task written against MSSQL does.
Leave it. If it should converge, that is a change to the Envoy **TypeScript**
first, then both ports — a separate piece of work with its own argument.

`sqlb.MSSQL` exists anyway, because `sqlfilter.MSSQL` exists
(`sqlfilter.go:240-246`) and something may be using it; it reproduces
`sqlfilter`'s MSSQL behaviour, not `mssqlconn`'s.

## What `sqlb` gives you

```go
import "github.com/trutohq/truto-sqlite-builder/go/sqlb"

filter, err := sqlb.FilterFromJS(args["filter"])   // already-decoded JSON
clause, err := sqlb.Postgres.CompileWhere(filter)

query := "SELECT * FROM " + sqlb.Postgres.Ident(table).Text() + " WHERE " + clause.Text()
rows, err := pool.Query(ctx, query, clause.Values()...)
```

| `sqlfilter` | `sqlb` |
|---|---|
| `sqlfilter.New(sqlfilter.Postgres)` + `.Compile(raw)` | `sqlb.Postgres.CompileWhere(*sqlb.Filter)` |
| `builder.Params()` | `clause.Values()` |
| `builder.Ident(name)` | `sqlb.Postgres.Ident(name).Text()` — **but see §"identifiers"** |
| `sqlfilter.ANSI` | `sqlb.SQLite` (same `?` + `"x"` spelling) |
| `sqlfilter.Postgres` / `MySQL` / `MSSQL` | `sqlb.Postgres` / `sqlb.MySQL` / `sqlb.MSSQL` |
| `sqlfilter.Columns(row)` | **not provided** — see §"what sqlb still cannot do" |
| `sqlfilter.OrderBy`, `LimitOffset`, `AssertReadonlySelectSQL` (`query.go`) | **not provided**, and out of scope: they are not filter compilation |

`sqlb.Dialect` is an opaque value with unexported fields, not `sqlfilter`'s
struct of three exported `func`s. That is deliberate: a caller cannot supply its
own `QuoteIdent` and put attacker-chosen text into a query with no `sql.raw` to
grep for.

---

## The five behaviour changes, worst first

### 1. An empty filter means EVERY ROW, and `sqlb` will not refuse for you

**Changed 2026-09-06 (D-0032). If you read an earlier version of this file, read
this section again — it now says the opposite.**

`sqlfilter.go:81-85` compiles an empty or non-object filter to the empty string,
and every caller turns that into a statement with **no WHERE clause**:
`pgconn/postgres.go:160-166`, `mysqlconn/mysql.go:202-208`,
`sqliteconn/sqlite.go:144-150`.

**`sqlb` now does the same.** `CompileWhere` on a filter with no conditions
returns an empty `Clause` and a **nil error**. It used to return
`ErrEmptyFilter`; it does not any more.

**Every destructive call site — `DELETE`, `UPDATE` — must guard explicitly,
because the library will no longer refuse for them.** On a delete that is the
difference between a failed run and an outage: a filter that decoded to nothing
now removes every row in the table, silently, with a successful-looking run.
There is no error to catch, no log line, and no test that fails.

All of these are empty, and all of them are reachable from a request body:

```jsonc
{}                       // an empty object
[]                       // an empty array — the reference treats it as an object with no keys
{"deleted_at": null}     // NOT empty: this is IS NULL
{"$notanobject": "x"}    // a $-prefixed key whose value is not an object is ignored
```

**Do this**, at each call site, where it is visible:

```go
clause, err := dialect.CompileWhere(filter)
if err != nil {
    return err
}
if clause.IsEmpty() && destructive {
    return fmt.Errorf("refusing to %s with no filter", method)   // or sqlb.ErrEmptyFilter
}
where := ""
if !clause.IsEmpty() {
    where = " WHERE " + clause.Text()
    params = clause.Values()
}
```

`filter == nil || filter.Len() == 0` before compiling is equivalent and equally
fine. What is **not** fine is relying on an error that no longer comes.

`sqlb.ErrEmptyFilter` is still exported, for two narrower jobs: returning it
yourself when you decide to refuse, and identifying a *nested* empty filter —
`{"and":[{}]}`, or an empty alias block — which still errors, because emitting
nothing there would drop a conjunct rather than widen the statement.

A `nil` `*Filter` is still `Filter must be an object`, not an empty clause.

### 2. Identifiers are rejected, not escaped

**`sqlfilter.go:33`** accepts any string and escapes it:

```go
QuoteIdent: func(name string) string { return `"` + strings.ReplaceAll(name, `"`, `""`) + `"` }
```

`sqlb` validates first: `[A-Za-z_][A-Za-z0-9_]*` per dot-separated part, ASCII
only, 255 bytes per part (`sqlb/ident.go:66`), and **rejects** anything else with
`Invalid identifier: <name>. Must be a valid identifier or qualified identifier
(e.g., table.column)`.

**What breaks:** any task filtering on a column whose name has a space, a hyphen,
a non-ASCII character, or a leading digit. Those work today and will start
erroring. That is the intended direction — it is the reference's rule and the
reason `sqlb`'s output alphabet is closed — but it is a behaviour change users
can hit, so it wants a release note, and possibly a grep over stored task
definitions before the swap.

It is **not** offered as a per-dialect option. Escaping and rejecting are
different security postures, and a flag would let a caller pick the weaker one.

### 3. `and` / `or` start working

**`sqlfilter.go:194-196`:**

```go
case "regex", "and", "or":
    return "", fmt.Errorf("the %s operator is not supported in goenvoy", op)
```

`sqlb` supports both, with the reference's exact parenthesisation and its
hoisting rule: `and` and `or` are emitted **before** every plain field, and `and`
before `or`, whatever order the keys appear in (`src/filter.ts:314-350`).

This is additive — nothing that works today stops working — but it changes the
**order of the bound values** for a filter that mixes logical operators with
plain fields. Any golden-SQL test in Envoy that asserts a clause string for such
a filter will need its expectation regenerated. There should not be many,
because such filters error today.

`regex` also starts working, on every dialect except MSSQL.

### 4. `exists` becomes a truthiness test

**`sqlfilter.go:209-212`:**

```go
func truthy(v any) bool { b, ok := v.(bool); return ok && b }
```

Only a literal `true` is truthy there. In JavaScript — and so in `sqlb` —
`{exists: 1}`, `{exists: "no"}` and `{exists: []}` are all truthy, while
`{exists: 0}`, `{exists: ""}` and `{exists: null}` are falsy
(`src/filter.ts:185`).

**What changes:** `{"deleted_at": {"exists": 1}}` compiles to `IS NULL` today and
`IS NOT NULL` after the swap. Same clause, opposite rows. Filters written with a
literal `true`/`false` — which is nearly all of them — are unaffected.

### 5. Clause wrapping and error text

`sqlfilter` joins field clauses with a flat `" AND "` (`sqlfilter.go:98`).
`sqlb` reproduces the reference's parenthesisation, which wraps a multi-clause
node: `((("a" = ?) AND ("b" >= ?)))` where `sqlfilter` gives
`("a" = ?) AND ("b" >= ?)`. Semantically identical, textually different — so any
test asserting an exact clause string changes.

Error messages are the **TypeScript's**, verbatim, so that a log grep on the
Truto side keeps matching. Two are already identical (`Unknown operator: %s`,
`Invalid JSON path: %s`); `JSON path filters are not supported by this database:
%s` was copied from `sqlfilter.go:116` on purpose. New ones:

| `sqlfilter` | `sqlb` |
|---|---|
| `the regex operator is not supported in goenvoy` | `REGEX operator is not supported by the mssql dialect` (MSSQL only; elsewhere it now works) |
| `the and operator is not supported in goenvoy` | — (works) |
| `IN operator cannot be used with empty arrays` | same, verbatim |
| `IN operator cannot be used with arrays larger than 999 items` | same, verbatim |
| — | `Nesting depth too deep (max: 10)`, `Too many operators (max: 100)`, `LIKE pattern too long: … (max: 1024)`, `Compiled filter too long: … (max: 102400)` |

Those last four are limits `sqlfilter` does not have at all. They are a
denial-of-service backstop on a filter that arrives over the wire; a legitimate
task filter is nowhere near any of them.

---

## Two bugs in `sqlfilter` that `sqlb` does not have

Worth knowing because a test currently **enshrines** one of them.

**A numbered dialect emits a literal `?` for a JSON path.**
`sqlfilter.go:243-245` hardcodes the marker:

```go
JSONExtract: func(column, path string) string {
    return fmt.Sprintf("JSON_VALUE(%s, ?)", column)
},
```

while every other marker in the same statement is `@p1`, `@p2`
(`sqlfilter.go:41`). `sqlfilter/mssql_test.go:80` asserts exactly this:

```go
if !strings.Contains(clause, "JSON_VALUE([config], ?)") {
```

That query does not bind. `sqlb` renders `JSON_VALUE([config], @p1)`. **The test
must be rewritten, not ported** — porting it as-is would carry the bug across.

**The path is bound but never referenced positionally.** `sqlfilter.go:118-122`
appends the path to `b.params` directly rather than through `b.Bind`, so the
placeholder counter and the parameter list disagree from that point on for any
`?`-style dialect with a JSON path plus another operator.

`sqlb` has a *related* defect and keeps it deliberately: the reference binds a
JSON path once while rendering `json_extract(col, ?)` into every clause, so a
multi-operator path filter emits four markers and three values (`S-01`,
`D-0108`). It is reproduced because Truto is in production on it. It is
**positional-only** — `@p1` and `$1` repeated across clauses refer to the same
bound value, so the numbered dialects are correct by construction. See
`TestJSONPathMisalignmentIsPositionalOnly`.

---

## What `sqlb` still cannot do

| | Why, and what to do |
|---|---|
| **JSON paths on Postgres or MySQL** | Both are wired unsupported, matching `sqlfilter` (`Postgres.JSONExtract` and `MySQL.JSONExtract` are nil, `sqlfilter.go:40-49`). **MySQL is nearly free**: `JSON_EXTRACT(col, '$.a.b')` takes the same `$.` syntax SQLite does, so it is one field on `sqlb.MySQL`. **Postgres is not**: `#>>` wants `'{a,b}'`, which is a path *translation*, not a spelling, and it returns `text` so numeric comparisons need a cast. Left open only because closing either changes what Envoy does today. |
| **`sqlfilter.Columns(row)`** (`sqlfilter.go:220`) | Returns a row's own key order for a generated INSERT/UPDATE. Nothing to do with filters. Keep it in Envoy, or move it next to `ordered`. |
| **`query.go`** — `OrderBy`, `LimitOffset`, `AssertReadonlySelectSQL` | Same: not filter compilation. `sqlb` has no opinion on ORDER BY or read-only enforcement and should not grow one as part of this. Note `OrderBy` takes a `quote func(string) string`; `sqlb.Dialect` deliberately does not expose one, so pass `func(s string) string { return d.Ident(s).Text() }` — which also gets you validation for free, and will start rejecting the same identifiers §2 describes. |
| **Composing a numbered clause into a larger query** | `CompileWhere` returns a `Clause`, which has `Text()` and `Values()` and no way into `sqlb.Q`. `$1` carries its absolute position, so splicing two independently compiled clauses would need renumbering, and renumbering that goes wrong binds every value to the wrong column while producing SQL that still runs. Envoy's connectors already assemble their own statements (`pgconn/postgres.go:148-172`), so this costs nothing. |
| **A schema-qualified dotted field** | A dot means JSON path in `sqlb`, as in `sqlfilter`. `mssqlconn` treats it as `[schema].[table]` (`filter.go:70-83`) — another reason that connector is out of scope. |
| **Value conversion for a driver** | `sqlfilter` binds whatever the value model holds; `sqliteconn.bindValue` (`sqlite.go:674`) then JSON-encodes objects and arrays with `expr.MarshalJS`, and maps both null sentinels to SQL NULL. `sqlb.Value` does the reference's conversion (dates → `YYYY-MM-DD HH:MM:SS`, `[]byte` rejected unless via `Blob`) but knows nothing about `ordered.Map` or gnata's null. **Keep Envoy's `bindValue` and run it over `clause.Values()`**, or the driver receives a struct it cannot convert and the run dies on the main write path. |

## Decoding the filter

`sqlb` does not parse JSON. It takes a `*sqlb.Filter`, and there are two doors:

```go
// The value model Envoy already has — ordered.Map, []any, float64, gnata's null.
filter, err := sqlb.FilterFromJS(args["filter"])

// Raw bytes.
filter, err := sqlb.DecodeFilter(body)
```

`FilterFromJS` expects `github.com/trutohq/jsorder`'s value model. Envoy's
`internal/ordered` is the same model — `ordered.Map` and `jsorder.Map` are both
type aliases of `gnata.OrderedMap` (`goenvoy/internal/ordered/ordered.go:57`,
`jsorder/map.go:24`) — so an `*ordered.Map` **is** a `*jsorder.Map` to the Go
type system and passes straight through. That is not a coincidence; it is
`D-0014`.

Two things to check on the Envoy side:

- **Null.** `jsorder` and `ordered` both use gnata's `JSONNull` sentinel for JSON
  null and reserve Go `nil` for `undefined`. `sqlb` is the other way round —
  `nil` is null, `sqlb.Undefined` is undefined — and `FilterFromJS` does that
  translation. If Envoy ever hands it a plain Go `nil` meaning "explicit null",
  the field will be **dropped** instead of compiling to `IS NULL`, which widens
  the result set.
- **Numbers.** Envoy's decoder produces `float64`; `FilterFromJS` narrows an
  integral one inside the safe-integer range to `int64` so SQLite sees an
  INTEGER, which is what `sqliteconn.bindValue` (`sqlite.go:676-680`) already
  does by hand.

## Tests to move, and how

**Move as-is** (they assert shared semantics, and `sqlb` already passes
equivalents — check before duplicating):

- everything asserting operator behaviour: `gt`/`gte`/`lt`/`lte`, `ne` with and
  without null, `in`/`nin` including the empty and >999 cases, `like`
- unknown-operator rejection
- `Invalid JSON path` for `a.`, `.a`, `a..b`

**Move with the expectation regenerated** — the clause text changes because of
the parenthesisation in §5:

- any golden-SQL assertion in `pgconn`, `mysqlconn`, `sqliteconn`

**Move and fix first:**

- `sqlfilter/mssql_test.go:74-90` `TestMSSQLJSONPathBindsThePathBeforeTheOperand`
  — the `JSON_VALUE([config], ?)` assertion is wrong; expect `@p1`. The rest of
  the test (path binds before the operand) is right and is worth keeping.

**Do not move** — `sqlb` covers these harder, from the reference's own suite:

- `sqlfilter/mssql_test.go:15-50` placeholder numbering →
  `TestDialectSpelling/mssql_numbers_every_placeholder_across_an_IN_list`
- `sqlfilter/mssql_test.go:52-72` bracket escaping → `sqlb` never needs to
  escape, because it rejects; `TestDialectIdentifierPolicyIsNotADialectSetting`
  and `TestAdversarial_QuotingNeedsNoEscapeInAnyDialect` are the replacements

**Write new, in Envoy, because they are Envoy's decision and not `sqlb`'s:**

- for every connector method that takes a filter, one test that an **empty**
  filter does what that method should do — and for `delete`, that it refuses.
  This one is not optional any more: since D-0032 nothing below the connector
  will stop an empty filter from deleting the table, so that test is the only
  thing standing there.

## Order of work

1. Add the dependency. `sqlb` currently needs a `replace` for the unpublished
   `github.com/trutohq/jsorder`; `goenvoy` will need the same until jsorder is
   pushed.
2. Port `sqliteconn` first — one dialect, one connector, and its private
   `compileFilter` disappears with it.
3. Then `pgconn` and `mysqlconn`, which share `sqlfilter.New`.
4. Delete `internal/sqlfilter/sqlfilter.go`, keeping `Columns` and `query.go`
   wherever they land.
5. Leave `mssqlconn/filter.go` alone.

Do not leave an adapter between the two. A converter is the duplication plus a
lossy boundary (`D-0025`).

## Where the rest of the reasoning lives

`truto/docs/go-port/wiki/`: `DECISIONS.md` (`D-0013`, `D-0023`, `D-0024`,
`D-0025`, **`D-0032`** — the empty filter, **`D-0041`** — why the filter still
decodes through a `*jsorder.Map`, `D-0108`, `D-0110`–`D-0113`),
`DIVERGENCES.md` (`S-01`…`S-21`), `NEEDS-INPUT.md` (`O-J2`), `GOTCHAS.md`.
