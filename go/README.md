# sqlb — the Go port of `@truto/sqlite-builder`

A safe, zero-dependency SQL builder for SQLite. Same trust model as the
TypeScript in `../src`: the **text** of a query is code and must come from the
developer, and every interpolated **value** is data and is always bound.

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
| `compileFilter(obj)` | `CompileFilter(*Filter)` | plus `CompileFilterJSON([]byte)` |

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

For a filter that arrives as JSON — which is the usual case for a service —
`CompileFilterJSON` decodes it order-preservingly and compiles in one step.

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
diffed. **466 test cases pass in total, 0 skipped**: 279 in the three ported
suites, 105 adversarial, 81 fuzz seed-corpus cases, and the differential run.

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
injection attempts, and nesting past the depth limit. Roughly 58% of cases
error on both sides, and the error **messages** are compared too, not just the
fact of failing.

This is what found the two `jsontext` divergences recorded in
`GOTCHAS.md` — within a minute of the JSON decoder being swapped, and neither
was visible by reading either implementation.

### Fuzzing

Five targets, run ~19M executions clean during the port:

| Target | Invariant |
|---|---|
| `FuzzCompileFilterJSON` | arbitrary JSON in; either an error, or a clause drawn from a **closed alphabet** with no quote, backslash, semicolon or comment marker in it |
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
| `CompileFilterJSONRealistic` — decode + compile | 11482.0 | 14856 | 168 |
| `FilterDecodeJSONOnly` | 10131.5 | 12017 | 119 |
| `QParallel` | 65.2 | 112 | 3 |

Three things a naive transliteration would have got wrong, and what fixing them
was worth:

| Change | Before | After |
|---|---|---|
| Compile the clause into one buffer instead of returning-and-joining strings per node | 9150 ns, **166 allocs** | 3184 ns, **37 allocs** |
| Emit quoted identifiers straight into the caller's buffer | `Idents10` 716 ns, **13 allocs** | 393 ns, **2 allocs** |
| Skip the builder entirely when no argument splices text — the assembled query *is* the format | `QSimple` 228 ns, **4 allocs** | 119 ns, **3 allocs** |
| Fuse the stacked-query scan into the placeholder scan, dropping the "code-only" copy | 1 alloc per query, proportional to length | **0 allocs** |

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
| wall time | 10.86 s | 2.74 s | **3.97× faster** |
| per operation | 54.3 µs | 13.7 µs | |
| peak RSS (kernel `VmHWM`) | 109.4 MB | 11.5 MB | **9.5× less** |

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
│   ├── filterjson.go            order-preserving JSON decoding
│   ├── sql_test.go              port of src/sql.test.ts
│   ├── filter_test.go           port of src/filter.test.ts
│   ├── security_test.go         port of src/security.test.ts
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

**None.** Standard library only, matching the reference's "zero-dependency by
design". `encoding/json/jsontext` is standard library in Go 1.27.

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
