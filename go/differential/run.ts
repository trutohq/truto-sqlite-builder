/**
 * Reference side of the differential harness.
 *
 * Reads the case file the Go test generates, runs each case through the
 * TypeScript in ../../src, and writes the results to stdout as JSON. The Go
 * side diffs them. See go/sqlb/differential_test.go.
 *
 *   bun go/differential/run.ts cases.json > js.json
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
    default:
      throw new Error(`unknown arg tag ${a.t}`)
  }
}

function run(c: DiffCase): DiffResult {
  try {
    if (c.kind === 'filter') {
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      const r = compileFilter(c.filter as any)
      return { ok: true, text: r.text, values: [...r.values] }
    }
    const values = c.args.map(buildArg)
    const r = tag(c.parts, values)
    return { ok: true, text: r.text, values: [...r.values] }
  } catch (e) {
    return { ok: false, error: e instanceof Error ? e.message : String(e) }
  }
}

const path = process.argv[2]
if (!path) {
  console.error('usage: bun run.ts <cases.json>')
  process.exit(2)
}

// sql.in() warns on large arrays and the corpus contains some on purpose;
// silence it so the JSON on stdout stays parseable.
console.warn = () => {}

const cases: DiffCase[] = JSON.parse(await Bun.file(path).text())
process.stdout.write(JSON.stringify(cases.map(run)))
