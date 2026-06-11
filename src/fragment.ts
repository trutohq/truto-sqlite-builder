import type { SqlFragment } from './types'

/**
 * Module-private registry of every SQL fragment minted by this library.
 *
 * Trust model: the text of a query is "code" (must originate from the
 * developer) and interpolated values are "data" (always parameterized). The
 * only way to contribute raw text is through a fragment created by one of the
 * library helpers (sql, sql.raw, sql.ident, sql.in, sql.blob, sql.join,
 * compileFilter). Membership in this WeakSet is the unforgeable proof that an
 * object is such a fragment.
 *
 * A plain object that merely looks like `{ text, values }` (e.g. from
 * JSON.parse or an untrusted request body) is NOT in this set, so it can never
 * be treated as raw SQL. This closes the structural duck-typing bypass.
 */
const fragmentRegistry = new WeakSet<object>()

/**
 * Create and register an immutable, branded SQL fragment.
 *
 * The returned object is frozen (including its values array) so callers cannot
 * mutate a fragment after the integrity checks that produced it.
 */
export function createFragment(
  text: string,
  values: readonly unknown[],
): SqlFragment {
  const fragment: SqlFragment = Object.freeze({
    text,
    values: Object.freeze([...values]),
  })

  fragmentRegistry.add(fragment)

  return fragment
}

/**
 * Type guard: was this value minted by the library (and therefore trusted to
 * contribute raw SQL text)?
 */
export function isSqlFragment(value: unknown): value is SqlFragment {
  return (
    typeof value === 'object' &&
    value !== null &&
    fragmentRegistry.has(value as object)
  )
}
