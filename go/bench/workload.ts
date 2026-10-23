/**
 * JavaScript side of the JS-versus-Go comparison.
 *
 * One realistic workload, run identically on both sides so the numbers mean
 * something: build a paginated list query the way a service does — quote a
 * column list, compile a JSON filter into a WHERE clause, add ORDER BY and
 * LIMIT — and repeat it.
 *
 * The reference's own bench/benchmark.ts measures the tag against string
 * concatenation, which answers a different question ("what does safety cost?").
 * This one answers "what does the same work cost in each language?".
 *
 *   bun go/bench/workload.ts <iterations>
 *
 * Prints one JSON object: wall time in ms and peak RSS in bytes.
 */
import { compileFilter } from '../../src/filter'
import { sql } from '../../src/sql'

const ITERATIONS = Number(process.argv[2] ?? '200000')

const COLUMNS = [
  'id',
  'name',
  'email',
  'status',
  'created_at',
  'updated_at',
  'u.role',
  'p.title',
]

const FILTER = {
  and: [
    { available: true },
    { price: { gte: 10, lte: 500 } },
    {
      or: [
        {
          and: [
            { category: 'electronics' },
            {
              or: [
                { brand: { in: ['Apple', 'Samsung', 'Google'] } },
                {
                  and: [
                    { brand: 'Other' },
                    { rating: { gte: 4.5 } },
                    { reviews_count: { gte: 100 } },
                  ],
                },
              ],
            },
          ],
        },
        {
          and: [
            { category: 'clothing' },
            {
              or: [
                {
                  and: [
                    { 'attributes.gender': 'unisex' },
                    { 'attributes.size': { in: ['S', 'M', 'L'] } },
                  ],
                },
                {
                  and: [
                    { 'attributes.style': { like: '%casual%' } },
                    { discount_percentage: { gte: 20 } },
                  ],
                },
              ],
            },
          ],
        },
        {
          and: [
            { category: 'books' },
            {
              or: [
                { genre: { in: ['fiction', 'mystery', 'sci-fi'] } },
                {
                  and: [
                    { genre: 'non-fiction' },
                    { 'metadata.bestseller': true },
                  ],
                },
              ],
            },
            { language: { in: ['english', 'spanish'] } },
          ],
        },
      ],
    },
  ],
}

// The filter arrives as JSON in real life, so parsing is part of the workload.
const FILTER_JSON = JSON.stringify(FILTER)

function once(i: number) {
  const parsed = JSON.parse(FILTER_JSON)
  const where = compileFilter(parsed)
  const query = sql`
    SELECT ${sql.ident(COLUMNS)}
    FROM ${sql.ident('products')} p
    JOIN ${sql.ident('users')} u ON u.id = p.owner_id
    WHERE ${where}
    ORDER BY ${sql.ident('p.created_at')} DESC
    LIMIT ${100} OFFSET ${i % 1000}
  `
  return query.text.length + query.values.length
}

// Warm the JIT so the measurement is of steady state, not of compilation.
let checksum = 0
for (let i = 0; i < 2000; i++) {
  checksum += once(i)
}

const start = performance.now()
for (let i = 0; i < ITERATIONS; i++) {
  checksum += once(i)
}
const elapsedMs = performance.now() - start

// Bun and Node both expose RSS through process.memoryUsage().
const peakRSS = process.memoryUsage().rss

process.stdout.write(
  JSON.stringify({
    runtime: typeof Bun !== 'undefined' ? `bun ${Bun.version}` : 'node',
    iterations: ITERATIONS,
    elapsed_ms: Number(elapsedMs.toFixed(2)),
    ns_per_op: Number(((elapsedMs * 1e6) / ITERATIONS).toFixed(1)),
    peak_rss_bytes: peakRSS,
    checksum,
  }) + '\n',
)
