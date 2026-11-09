/**
 * Reference side of the differential harness.
 *
 * Reads the case file the Go test generates, runs each case through the
 * TypeScript in ../../src, and writes the results to stdout. The Go side diffs
 * them. See go/sqlb/differential_test.go.
 *
 *   bun go/differential/run.ts cases.jsonl > js.jsonl
 *
 * The wire format is JSONL — one case per line in, one result per line out, in
 * order. That is what github.com/trutohq/differential speaks, and it is the
 * better framing: a single JSON array made a 150,000-case corpus one parse and
 * one live object on each side, a malformed case took the whole file with it,
 * and neither side could start before the other finished. No JSON-encoded
 * value can contain a raw newline, so the framing is safe by construction.
 */
import { compileFilter } from '../../src/filter'
import { sql } from '../../src/sql'

type DiffArg = { t: string; v?: unknown }

type DiffCase =
  | { kind: 'filter'; filter: unknown }
  | { kind: 'sql'; parts: string[]; args: DiffArg[] }

type DiffResult = {
  ok: boolean
  text?: string
  values?: unknown[]
  /**
   * vtypes is what SQLite would STORE for each bound value, one tag per entry
   * of values.
   *
   * It exists because JSON erases the distinction the Go side has to get right.
   * `JSON.stringify(1)` is `"1"` whether the value is 1 or 1.0, and
   * `json.Marshal` on the Go side flattens `int64(1)` and `float64(1)` to the
   * same `1` — so a port that bound every integer as a REAL passed 25,000 cases
   * with identical bucket counts. In SQLite that is not cosmetic: `typeof()`
   * and `quote()` differ, and a value read back re-serialises as `1.0`.
   *
   * `int` is the contract `jsNumber` (go/sqlb/jsonfilter.go) implements: an
   * integral number inside the exactly-representable range binds as INTEGER,
   * everything else as REAL. Past 2^53 a JSON literal is already inexact by the
   * time JavaScript sees it, so REAL is the truthful answer on both sides.
   */
  vtypes?: string[]
  error?: string
}

// The sql tag only ever reads the array by index, so a plain array stands in
// for a TemplateStringsArray.
function tag(parts: string[], values: unknown[]) {
  return sql(parts as unknown as TemplateStringsArray, ...values)
}

function buildArg(a: DiffArg): unknown {
  switch (a.t) {
    case 'str':
    case 'num':
    case 'bool':
      return a.v
    case 'null':
      return null
    case 'undefined':
      return undefined
    case 'ident':
      return sql.ident(a.v as string)
    case 'idents':
      return sql.ident(a.v as string[])
    case 'raw':
      return sql.raw(a.v as string)
    case 'in':
      return sql.in(a.v as unknown[])
    case 'date':
      // Epoch milliseconds, so the two sides construct the same instant. Both
      // formatDate implementations then read it through LOCAL-time getters,
      // which is why the harness pins TZ=UTC (Truto's H-18).
      return new Date(a.v as number)
    case 'blob':
      // An absent value is passed straight through, so sql.blob() throws its
      // own "requires a Buffer or Uint8Array". `== null` on purpose: the case
      // file omits an empty v, so this arrives as undefined rather than null,
      // and `new Uint8Array(undefined)` is a silently EMPTY blob — which made
      // the reference succeed where the Go side refused.
      return a.v == null
        ? sql.blob(a.v as unknown as Uint8Array)
        : sql.blob(new Uint8Array(a.v as number[]))
    case 'rawblob':
      // A Uint8Array as a plain value, NOT through sql.blob(): the reference
      // refuses it, and that message is one the corpus never reached.
      return new Uint8Array(a.v as number[])
    case 'unsupported':
      // Reaches `Unsupported value type: ${typeof value}` — the one message
      // sameError is deliberately loose about, because JavaScript reports
      // typeof and Go reports its own type name.
      return {}
    default:
      throw new Error(`unknown arg tag ${a.t}`)
  }
}

function valueTag(v: unknown): string {
  if (v === null || v === undefined) return 'null'
  switch (typeof v) {
    case 'string':
      return 'text'
    case 'boolean':
      return 'bool'
    case 'number':
      // Number.MAX_SAFE_INTEGER + 1 is 2^53, which is jsNumber's inclusive
      // bound. Both are exact, so this comparison is exact.
      return Number.isInteger(v) && Math.abs(v) <= Number.MAX_SAFE_INTEGER + 1
        ? 'int'
        : 'real'
  }
  if (v instanceof Uint8Array) return 'blob'
  return typeof v
}

// A BLOB crosses the wire as base64, which is what Go's json.Marshal does for
// []byte. JSON.stringify of a Uint8Array is `{"0":1,"1":2}`, which would
// disagree with the Go side for reasons that are about JSON and not about SQL.
function wireValue(v: unknown): unknown {
  if (v instanceof Uint8Array) return Buffer.from(v).toString('base64')
  return v
}

function result(values: readonly unknown[], text: string): DiffResult {
  return {
    ok: true,
    text,
    values: values.map(wireValue),
    vtypes: values.map(valueTag),
  }
}

function run(c: DiffCase): DiffResult {
  try {
    if (c.kind === 'filter') {
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      const r = compileFilter(c.filter as any)
      return result(r.values, r.text)
    }
    const values = c.args.map(buildArg)
    const r = tag(c.parts, values)
    return result(r.values, r.text)
  } catch (e) {
    return { ok: false, error: e instanceof Error ? e.message : String(e) }
  }
}

const path = process.argv[2]
if (!path) {
  console.error('usage: bun run.ts <cases.jsonl>')
  process.exit(2)
}

// sql.in() warns on large arrays and the corpus contains some on purpose;
// silence it so the JSONL on stdout stays parseable.
console.warn = () => {}

const text = await Bun.file(path).text()
// Batched rather than one write per case: at 150,000 cases the syscalls are a
// measurable share of the run, and the Go side reads the stream as it arrives
// either way.
const out: string[] = []
for (const line of text.split('\n')) {
  if (line.length === 0) continue
  out.push(JSON.stringify(run(JSON.parse(line) as DiffCase)))
  if (out.length >= 1000) {
    process.stdout.write(out.join('\n') + '\n')
    out.length = 0
  }
}
if (out.length > 0) process.stdout.write(out.join('\n') + '\n')
