# sqlb — the Go port of `@truto/sqlite-builder`

A safe SQL builder for SQLite, Postgres, MySQL and SQL Server. Same trust model
as the TypeScript in `../src`: the **text** of a query is code and must come from
the developer, and every interpolated **value** is data and is always bound.

```go
import "github.com/trutohq/truto-sqlite-builder/go/sqlb"
```

Behavioural parity with the TypeScript is not a goal that was declared and
hoped for — it is checked, by running both engines over a generated corpus on
every test run. See [Verification](#verification).

---

## The API differs from the TypeScript, and this is the only place it had to

The TypeScript entry point is a tagged template:

```ts
sql`SELECT * FROM ${sql.ident(table)} WHERE id = ${id}`
```

Go has no tagged templates, so the surface could not be faithful. The Go entry
point is `Q`, whose **first argument must be a compile-time constant** and whose
interpolation slots are written as `?`:

```go
q, err := sqlb.Q("SELECT * FROM ? WHERE id = ?", sqlb.Ident(table), id)
// q.Text()   == `SELECT * FROM "users" WHERE id = ?`
// q.Values() == []any{id}
```

A slot filled with a `Fragment` splices that fragment's text and its values; a
slot filled with anything else is converted and bound. That is exactly what
`${}` does in the TypeScript.

**The constant-only rule is enforced by the compiler, not by convention.** `Q`'s
first parameter has an unexported named string type. Go assigns an untyped
string constant to it happily, but a `string` variable needs an explicit
conversion, and the type cannot be named from outside the package — so this does
not build:

```go
userInput := r.URL.Query().Get("q")
sqlb.Q(userInput)                    // compile error
sqlb.Q("SELECT * FROM " + userInput) // compile error
```

That is a stronger guarantee than the TypeScript has. `Raw` remains the single
explicit escape hatch, exactly as `sql.raw()` is in the reference.

### Everything else, side by side

| TypeScript | Go | Note |
|---|---|---|
| ``sql`…` `` | `Q(const, args…)` | `?` marks a slot |
| `sql.value(v)` | `Value(v)` | |
| `sql.ident("a.b")` | `Ident("a.b")` | |
| `sql.ident([...])` | `Idents(items…)` | items are `string` or `Fragment` |
| `sql.in([...])` | `In(values…)` / `InSlice(slice)` | `InSlice` is generic, so no `[]any` conversion |
| `sql.raw(s)` | `Raw(s)` | the trust boundary |
| `sql.blob(b)` | `Blob(b)` | |
| `sql.join(f, sep)` | `Join(frags, sep)` | no default separator; pass `", "` |
| `sql.join(f, frag)` | `JoinFragment(frags, sep)` | separator with its own values |
| `compileFilter(obj)` | `CompileFilter(*Filter)` | SQLite; `Dialect.CompileWhere` for the rest |
| `JSON.parse` + `compileFilter` | `DecodeFilter([]byte)` then `CompileFilter` | decoding is `jsorder`'s, not this package's |

### Errors are sticky on fragments

The reference throws from `sql.ident()` and friends immediately. Doing that in
Go would mean `(Fragment, error)` from every constructor, which makes them
impossible to nest inside a `Q` call. Instead a failed `Fragment` carries its
error, has **no text and no values**, and every consumer propagates it:

```go
q, err := sqlb.Q("SELECT ? FROM t", sqlb.Ident("bad-name"))
// err: Invalid identifier: bad-name. …
```

Check `Fragment.Err()` if you build one and do not immediately use it. Error
messages are the reference's, verbatim, so log greps and alert regexes keep
matching.

### Filters are ordered, because order is the contract

`Filter` and `Ops` are ordered key/value lists, not maps. Key order decides
clause order, which decides the order of the bound values; Go map iteration is
randomised, so a map cannot express this.

```go
where, err := sqlb.CompileFilter(
    sqlb.NewFilter().
        Eq("status", "ACTIVE").
        Where("age", sqlb.NewOps().Gte(18).Lt(65)).
        Or(
            sqlb.NewFilter().Where("priority", sqlb.NewOps().Gte(8)),
            sqlb.NewFilter().Eq("category", "urgent"),
        ),
)
q, err := sqlb.Q("SELECT * FROM users WHERE ? LIMIT ?", where, 100)
```

For a filter that arrives as JSON — the usual case for a service —
`DecodeFilter` parses it order-preservingly into a `*Filter`:

```go
filter, err := sqlb.DecodeFilter(body)   // github.com/trutohq/jsorder underneath
where, err := sqlb.CompileFilter(filter)
```

`DecodeFilter` is about twenty lines of glue over `jsorder.DecodeFloat64`, and
that is the point: this package used to carry its own order-preserving JSON
decoder and no longer does, because two of those in one organisation is one
duplicated *decision* (D-0023). If the body has been parsed already, hand the
decoded value to `FilterFromJS` instead of parsing it twice.

Three things the glue has to get right, all pinned by tests whose expectations
came from executing the reference:

- **JSON null is `nil` here and a sentinel in `jsorder`.** The two packages spend
  Go's `nil` differently — `jsorder` reserves it for `undefined`. Getting it
  backwards turns `{"deleted_at": null}` from `IS NULL` into a dropped
  condition, which widens a `DELETE`.
- **Numbers go through `JSON.parse`'s rounding.** `DecodeFloat64`, not `Decode`:
  `{"id": 9007199254740993}` binds `9007199254740992`, because that is what the
  reference binds. An integral value inside the safe-integer range then narrows
  to `int64` so SQLite sees an INTEGER.
- **Key order is JavaScript's, not the document's.** `{"10":1,"2":1}` enumerates
  `2` before `10`. The old decoder did not do this and the differential corpus
  had to exclude integer keys to hide it; the exclusion is gone.

Three JavaScript-shaped details survive into the Go API because dropping them
would change behaviour:

- **`Undefined`.** JavaScript distinguishes `null` from `undefined` and the
  reference treats them differently in a filter: `{a: null}` compiles to
  `IS NULL`, `{a: undefined}` makes the field disappear. Go has one `nil`, so
  `sqlb.Undefined` carries the other.
- **`Ops.Set(name, value)`.** The typed methods (`Gt`, `In`, …) cover every
  valid operator. `Set` exists for decoded input and is how an unknown operator
  can still reach — and be rejected by — the validator.
- **`Exists` is truthiness, not a boolean.** `{exists: 0}` means `IS NULL` in
  the reference. `Ops.Exists(bool)` is the typed door; `Set("exists", …)` is the
  faithful one.

---

## Dialects

The default is SQLite and it is the **zero value**, so nothing about the API
above changes and no existing output moves by a byte:

```go
where, err := sqlb.CompileFilter(filter)        // SQLite, as before
```

For another database, compile through a `Dialect`:

```go
clause, err := sqlb.Postgres.CompileWhere(filter)
// clause.Text()   == `(("status" = $1) AND ("age" >= $2))`
// clause.Values() == []any{"ACTIVE", int64(18)}

query := `SELECT * FROM "users" WHERE ` + clause.Text()
rows, err := pool.Query(ctx, query, clause.Values()...)
```

Four dialects, and a Dialect decides exactly three things:

| | bind marker | identifier | `ilike` | `regex` | JSON path |
|---|---|---|---|---|---|
| `SQLite` (default) | `?` | `"x"` | `LIKE ? COLLATE NOCASE` | `REGEXP ?` | `json_extract(x, ?)` |
| `Postgres` | `$1` | `"x"` | `ILIKE $1` | `~ $1` | **error** |
| `MySQL` | `?` | `` `x` `` | `LIKE ?` | `REGEXP ?` | **error** |
| `MSSQL` | `@p1` | `[x]` | `LOWER(x) LIKE LOWER(@p1)` | **error** | `JSON_VALUE(x, @p1)` |

Everything that decides which **rows** match is the reference's and is the same
in all four: operator semantics, clause order, the truthiness of `exists`, what
an empty filter means, and the identifier charset.

**Three things a Dialect deliberately is not.**

*It is not a struct of functions.* `goenvoy/internal/sqlfilter` models a dialect
as three exported `func` fields, which is the natural Go design and the wrong
one here: it reopens the boundary this package exists to close. `Dialect`'s
fields are unexported and the only values are the four above, so a caller cannot
supply its own `QuoteIdent` and put attacker-controlled text into a query with
no `Raw` call to grep for.

*It is not a way to choose an identifier policy.* Every dialect **rejects**
anything outside `[A-Za-z_][A-Za-z0-9_]*`; none escapes. `sqlfilter` escapes by
doubling the closing delimiter, which is a different security posture, and
offering both per database would let a caller pick the weaker one.

*It is not where "empty filter means everything" lives.* See `ErrEmptyFilter`.

### `Clause` is not a `Fragment`, and that is the point

`CompileWhere` returns a `Clause`, which has `Text()` and `Values()` and no way
into `Q`, `Join` or `Idents`. A `$1` or `@p1` carries its **absolute position**
in its own text, so splicing two independently compiled clauses would need
renumbering — and renumbering that goes wrong binds every value to the wrong
column while producing SQL that still runs. Making that a type error rather than
a run-time check is the same move as `Q`'s constant-only format.

The SQLite path keeps the composable `Fragment`: `CompileFilter` and
`SQLite.CompileWhere` are the same compiler, and a test asserts they agree.

### The JSON-path defect is positional-only

The reference binds a JSON path **once** while rendering `json_extract(col, ?)`
into every clause, so a multi-operator path filter emits more markers than it
binds (`S-01`, reproduced deliberately). That is a defect of *positional*
placeholders. The numbered dialects do not have it, and not because anything was
fixed — `@p1` repeated in three clauses refers to the same bound value:

```
SQLite    ((json_extract("a", ?) > ? AND json_extract("a", ?) < ?))   3 values, 4 markers
MSSQL     ((JSON_VALUE([a], @p1) > @p2 AND JSON_VALUE([a], @p1) < @p3))  3 values, 3 markers
```

`TestJSONPathMisalignmentIsPositionalOnly` pins both halves.

### Empty filters change rows, not syntax

`CompileFilter` on a filter with no conditions returns `ErrEmptyFilter`.
`sqlfilter.go:83` returns `""` for the same input, which its connectors turn
into a statement with **no WHERE clause** — "match everything". Swapping one for
the other silently is a `DELETE` that removes every row in the table, or none.

It is not a Dialect setting and not an option. Decide it at your call site,
where it is visible:

```go
if filter == nil || filter.Len() == 0 {
    // no WHERE clause, or refuse the request — your call, written down
}
```

`errors.Is(err, sqlb.ErrEmptyFilter)` also works, but it fires for a *nested*
empty filter too, so it cannot tell "no filter was sent" from `{"and":[{}]}`.

---

## Verification

```sh
export PATH="$HOME/.local/go/bin:$PATH"   # Go 1.27.0 is not on the default PATH

go test ./...                 # unit + ported suites + differential
go test -race -count=1 ./...
go vet ./... && gofmt -l .
```

### Parity with the reference suite

**224 of 224 reference cases have a Go counterpart and pass. 0 skipped.**

The Go subtest names match the reference's `it` names so the two lists can be
diffed. **526 test cases pass in total, 0 skipped**: 279 in the three ported
suites, 105 adversarial, 92 fuzz seed-corpus cases, 23 for the `jsorder` glue,
26 for the dialects, and the differential run.

No test in the three ported suites was edited for either the `jsorder` swap or
the dialects. The SQLite path is byte-identical.

Nine of the 224 assert a **compile-time** guarantee instead of a runtime throw,
because the input the reference rejects cannot be written in Go at all. They are
marked `TYPE-ENFORCED` in the source:

| Reference case | Why it cannot be written |
|---|---|
| `sql.ident(123)` rejects non-strings | `Ident` takes a `string` |
| `sql.raw(123)` rejects non-strings | `Raw` takes a `string` |
| `sql.in('not an array')` rejects non-arrays | `In` is variadic |
| `sql.join('not an array')` rejects non-arrays | `Join` takes `[]Fragment` |
| `sql.join(frags, 123)` rejects a non-string separator | `Join` takes a `string` |
| `sql.join([{text,values}])` rejects non-fragment items | `Join` takes `[]Fragment` |
| `compileFilter('string')` rejects a non-object | `CompileFilter` takes `*Filter` |
| two × "should return frozen objects" | `Fragment` is a value type with no exported fields |

### Differential testing against the running TypeScript

`go test -run TestDifferential` generates a deterministic corpus of filters and
queries, runs it through `bun` against `../src`, and diffs text, bound values
and error messages. It skips when `bun` is absent.

**4,000 cases per run by default. The final implementation was diffed over
190,000 generated cases across seven seeds, and more than half a million across
the port as a whole, with zero mismatches.** Tune it:

```sh
SQLB_DIFF_CASES=25000 SQLB_DIFF_SEED=7 go test -run TestDifferential ./sqlb
```

The corpus is deliberately hostile: invalid identifiers, oversized patterns,
unterminated literals, empty and 1000-element `IN` lists, alias blocks with
injection attempts, and nesting past the depth limit. Roughly 59% of cases
error on both sides, and the error **messages** are compared too, not just the
fact of failing.

Two things were **added** to the corpus when decoding moved to `jsorder`, both
of which the old decoder could not have passed:

- **Pure integer field names** (`"0"`, `"7"`, `"10"`, `"2"`). JavaScript emits
  array-index keys first in ascending numeric order, so these change which error
  the reference reports first. They used to be excluded to hide that.
- **Number literals `JSON.parse` cannot represent** — `9007199254740993`,
  `1234567890123456789`, `1e21`, `-0`. The bound value has to match what the
  reference binds *after* its rounding.

180,000 cases across eight seeds, zero mismatches, with both additions in.

This is what found the two `jsontext` divergences recorded in
`GOTCHAS.md` — within a minute of the JSON decoder being swapped, and neither
was visible by reading either implementation.

### Fuzzing

Five targets, run ~19M executions clean during the port:

| Target | Invariant |
|---|---|
| `FuzzCompileFilterJSON` | arbitrary JSON in; either an error, or a clause drawn from a **closed alphabet** with no quote, backslash, semicolon or comment marker in it |
| `FuzzCompileWhereAllDialects` | the same, for all four dialects at once, with every delimiter balanced — a dialect writes new literal text into the query, which is exactly how a code channel reopens |
| `FuzzIdent` | a successful `Ident` is quoted ASCII identifier parts and nothing else; a failed one has no text |
| `FuzzScanSQL` | the fused scanner agrees with an unfused transliteration of the reference |
| `FuzzQValuesNeverReachText` | for a fixed constant format, no value can change the SQL |
| `FuzzJoinSeparator` | an accepted separator cannot introduce a literal, a comment or a second statement |

```sh
go test -run=XXX -fuzz=FuzzCompileFilterJSON -fuzztime=60s ./sqlb
```

---

## Performance

Measured on a 4-core Intel Xeon E-2386G at 3.50GHz, Go 1.27.0, median of six
runs at `-benchtime=500ms`.

```sh
go test -run=XXX -bench=. -benchmem -count=6 ./sqlb
```

| Benchmark | ns/op | B/op | allocs/op |
|---|---:|---:|---:|
| `QSimple` — one bound value | 119.5 | 95 | 3 |
| `QComplex` — 4 values incl. a date | 314.0 | 224 | 7 |
| `QWithIn10` | 516.2 | 712 | 8 |
| `Ident` | 70.6 | 75 | 2 |
| `IdentQualified` — `a.b.c` | 116.6 | 96 | 2 |
| `Idents10` — an 8+ column list | 393.5 | 176 | 2 |
| `Join10` | 299.5 | 352 | 3 |
| `ScanSQLLarge` — 200-column insert, 897 MB/s | 2093.0 | **0** | **0** |
| `CompileFilterSmall` | 358.2 | 648 | 6 |
| `CompileFilterRealistic` — 3 levels, 30 values | 3184.5 | 2576 | 37 |
| `CompileFilterJSONRealistic` — decode + compile | see below | 23069 | 445 |
| `FilterDecodeJSONOnly` | see below | 20204 | 395 |
| `QParallel` | 65.2 | 112 | 3 |

Three things a naive transliteration would have got wrong, and what fixing them
was worth:

| Change | Before | After |
|---|---|---|
| Compile the clause into one buffer instead of returning-and-joining strings per node | 9150 ns, **166 allocs** | 3184 ns, **37 allocs** |
| Emit quoted identifiers straight into the caller's buffer | `Idents10` 716 ns, **13 allocs** | 393 ns, **2 allocs** |
| Skip the builder entirely when no argument splices text — the assembled query *is* the format | `QSimple` 228 ns, **4 allocs** | 119 ns, **3 allocs** |
| Fuse the stacked-query scan into the placeholder scan, dropping the "code-only" copy | 1 alloc per query, proportional to length | **0 allocs** |

Everything above is unchanged by the dialect work: a `Dialect` adds one field to
the compile context and one branch per bind marker, and the SQLite path emits
the same bytes with the same allocations it always did.

### What dropping the JSON decoder cost, honestly

The two decode rows above got **worse**, and that is the price of D-0023, not a
regression to hunt. `jsorder` is a general JSON decoder producing an ordered-map
value model; the decoder this package used to carry went straight from tokens
into `Filter` structs carved out of block allocators, which is faster precisely
because it was special-purpose.

Measured on the same box in the same session — `bench/compare.sh`'s workload,
`GOMAXPROCS=1`, 40,000 iterations, min and median of twelve runs each, `port/go`
against this branch:

| | before (own decoder) | after (`jsorder`) | |
|---|---:|---:|---|
| ns per operation, min | 14,709 | 23,821 | **1.62× slower** |
| ns per operation, median | 15,350 | 24,647 | 1.61× slower |
| decode allocations | 119 | 395 | **3.3×** |
| decode bytes | 12,017 | 20,204 | 1.7× |
| peak RSS | 8.2 MB | 9.5 MB | 1.16× |

Where the allocations go, from `go tool pprof -sample_index=alloc_objects`:

| | share |
|---|---:|
| `sqlb.fromJS` — rebuilding `*jsorder.Map` into `*Filter` | 37% |
| `gnata.NewOrderedMapWithCapacity` — the map `fromJS` then discards | 22% |
| `gnata.(*OrderedMap).Set` | 16% |
| `jsorder`'s own tokenizer walk | 14% |

So **three quarters of the added cost is building an ordered map that is thrown
away one function later.** That is the shape of the fix, and it is not a fix in
this repository: `jsorder` would need a way to drive a caller's own
materialisation from its tokenizer — one decoder, two value models — after which
`fromJS` disappears. Filed as a lead, not attempted here, because `jsorder`'s
public API is another agent's to change.

Note that a filter is decoded once per request and compiled once, against a
database round trip of milliseconds. 9 µs on a query that will spend 2 ms in
SQLite is not the thing to optimise first; it is recorded because a decision
that costs something should show the number.

### Against the JavaScript

```sh
go/bench/compare.sh [iterations]
```

One realistic workload, identical on both sides: parse a 645-byte JSON filter,
compile it to a WHERE clause, quote an eight-column select list and three
identifiers, and assemble a paginated list query. 200,000 iterations after a
2,000-iteration warm-up. Both sides print a checksum, and **the checksums
match** — so this is the same work, not a benchmark of two different programs.

| | bun 1.3.14 | Go 1.27.0 | |
|---|---:|---:|---|
| per operation | 52.6 µs | 23.8 µs | **2.2× faster** |
| peak RSS (kernel `VmHWM`) | 86.5 MB | 9.5 MB | **9.1× less** |

Re-measured after the `jsorder` swap, min of five (bun) and twelve (Go) runs at
40,000 iterations, `GOMAXPROCS=1`. The margin was 3.6× on the same box before
the swap; a `jsorder` that stopped materialising a discarded ordered map would
get most of that back.

**Where the remaining Go time goes, honestly.** 74% of it is decoding the JSON
filter, not compiling it — 10.1 µs of the 13.7. That is the cost of building an
ordered, dynamically-typed tree in a language where every value crosses an
interface boundary, against a JavaScript runtime whose `JSON.parse` is
hand-written C++ producing objects it was designed for. The compile step alone
is 3.2 µs. A caller that decodes once and compiles many times — which is what a
prepared-statement cache would do — sees the ratio widen considerably.

Moving the decoder from `encoding/json`'s `Decoder.Token` to Go 1.27's
`encoding/json/jsontext` cut its allocations by a fifth on its own; `Token`
returns `any` and boxes every string, number and delimiter. This is the same
finding recorded as P-1 in the port wiki's tracker, and the same remedy applies
there.

### Concurrency: none, deliberately, with the threshold measured

**Nothing in this package starts a goroutine.** There is no shared mutable state
either, so a caller can run it from as many goroutines as it likes —
`BenchmarkQParallel` is the evidence, and `-race` is clean.

The one place parallelism could plausibly pay is compiling a *batch* of filters,
so that was measured rather than assumed. Eight workers, results written to
pre-allocated indices so order is preserved:

| Filters in the batch | sequential | 8 workers | |
|---:|---:|---:|---|
| 1 | 3.15 µs | 6.58 µs | 2.1× **slower** |
| 8 | 25.5 µs | 30.6 µs | 1.2× **slower** |
| 64 | 202 µs | 102 µs | 2.0× faster |
| 512 | 1.80 ms | 1.64 ms | 1.1× faster |
| 4096 | 14.7 ms | 6.64 ms | 2.2× faster |

The crossover is around **32–64 filters**, and the win never approaches the
4× the core count allows because the work is allocation-bound, not CPU-bound.
Compiling one filter takes three microseconds; a goroutine costs more than that
to start and join.

So the library exposes a batch API to nobody and hides goroutines from nobody.
A caller that genuinely has thousands of filters to compile at once knows its own
`n` and can run its own bounded pool over `CompileFilter`, which is safe. See
`D-0104` in the port wiki for the reasoning.

---

## Layout

```
go/
├── go.mod                       module github.com/trutohq/truto-sqlite-builder/go
├── sqlb/
│   ├── doc.go                   package doc: the API difference, in one place
│   ├── api.go                   types, limits, builder methods
│   ├── sql.go                   Q, In, Raw, Blob, Join, separator validation
│   ├── ident.go                 identifier validation and quoting
│   ├── value.go                 value conversion and SQLite date formatting
│   ├── scan.go                  the literal/comment-aware scanner
│   ├── compile.go               the filter → WHERE clause emitter
│   ├── filter.go                Filter/Ops, and JavaScript value semantics
│   ├── dialect.go               SQLite/Postgres/MySQL/MSSQL spelling
│   ├── jsonfilter.go            the jsorder glue (~20 lines that matter)
│   ├── sql_test.go              port of src/sql.test.ts
│   ├── filter_test.go           port of src/filter.test.ts
│   ├── security_test.go         port of src/security.test.ts
│   ├── dialect_test.go
│   ├── jsonfilter_test.go
│   ├── security_adversarial_test.go
│   ├── differential_test.go     generates the corpus, diffs against bun
│   ├── fuzz_test.go
│   └── bench_test.go
├── differential/run.ts          the reference side of the differential harness
└── bench/
    ├── compare.sh               JS vs Go: wall time and peak RSS
    ├── workload.ts              JavaScript side
    └── workload/main.go         Go side
```

## Dependencies

One: **`github.com/trutohq/jsorder`**, for order-preserving JSON decoding.

This package was zero-dependency and no longer is, and that was spent
deliberately (D-0023). It had its own decoder; `jsorder` is the organisation's,
and carrying both meant carrying two answers to "how do we preserve JavaScript
key order in Go" that nothing tested against each other. Duplicated *decisions*
drift silently — they surface as a signature that will not verify, not as an
error — so one of them had to go, and the one with a single consumer went.

`jsorder` pulls `recolabs/gnata` (its ordered map is a type alias, see jsorder
D-0015) and gnata pulls `tidwall/gjson`.

**`jsorder` is not published yet.** `go.mod` carries

```
replace github.com/trutohq/jsorder => ../../jsorder
```

which expects `~/work/jsorder` beside this checkout, on branch
`feat/jsorder-initial`. Delete the directive and pin a version once it is
pushed; nothing else about the dependency changes.

The cost is measured, not asserted — see [Performance](#performance).

## Known divergences

The full register is in `docs/go-port/wiki/DIVERGENCES.md` in the `truto`
repository. The three worth knowing before you write a line against this:

1. **A reference bug is reproduced on purpose.** A JSON-path field with two or
   more comparison operators emits more placeholders than it binds
   (`src/filter.ts:169-176`). `CompileFilter` reproduces it; composing the result
   into `Q` then fails closed with a placeholder-count error rather than binding
   the wrong columns. Do not "fix" it here — see `S-01` in the divergence
   register.
2. **`time.Time` is formatted in its own location**, not forced to the process's
   local zone the way the reference's `getFullYear()` is. Workers run in UTC, so
   pass UTC times to reproduce what production stores.
3. **An interpolation cannot sit inside a string literal or a comment.** The
   tagged template can express it; `?`-slots cannot, because a `?` in a literal
   is not in code position. Both engines reject such a query, with different
   messages.

And two that matter only if you are coming from `goenvoy/internal/sqlfilter`,
both of which change **rows** rather than syntax:

4. **An empty filter is an error here and "match everything" there.**
5. **An unvalidated identifier is rejected here and escaped there.**

Read [`ENVOY-MIGRATION.md`](ENVOY-MIGRATION.md) before swapping one for the
other. It is written for someone with no context on either.
