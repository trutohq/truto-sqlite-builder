import {
  MAX_IDENTIFIER_LENGTH,
  MAX_QUERY_LENGTH,
  QUALIFIED_IDENTIFIER_REGEX,
  SIMPLE_IDENTIFIER_REGEX,
  STACKED_QUERY_REGEX,
} from './constants'
import { createFragment, isSqlFragment } from './fragment'
import type { SqlFragment, SqlQuery, SqlValue } from './types'

/**
 * Format a date for SQLite (YYYY-MM-DD HH:MM:SS)
 */
function formatDate(date: Date): string {
  const year = date.getFullYear()
  const month = String(date.getMonth() + 1).padStart(2, '0')
  const day = String(date.getDate()).padStart(2, '0')
  const hours = String(date.getHours()).padStart(2, '0')
  const minutes = String(date.getMinutes()).padStart(2, '0')
  const seconds = String(date.getSeconds()).padStart(2, '0')

  return `${year}-${month}-${day} ${hours}:${minutes}:${seconds}`
}

/**
 * Convert a value to its SQLite representation
 */
function sqlValue(value: SqlValue): unknown {
  if (value === null || value === undefined) {
    return null
  }

  if (typeof value === 'string') {
    return value
  }

  if (typeof value === 'number' || typeof value === 'boolean') {
    return value
  }

  if (value instanceof Date) {
    return formatDate(value)
  }

  if (value instanceof Buffer || value instanceof Uint8Array) {
    throw new TypeError(
      'Buffer/Uint8Array values must be used with sql.blob() for safe BLOB handling',
    )
  }

  throw new TypeError(`Unsupported value type: ${typeof value}`)
}

/**
 * Quote a single identifier part
 */
function quoteSingleIdentifier(identifier: string): string {
  if (identifier.length > MAX_IDENTIFIER_LENGTH) {
    throw new TypeError(
      `Identifier part too long: ${identifier.length} characters (max: ${MAX_IDENTIFIER_LENGTH})`,
    )
  }
  if (!SIMPLE_IDENTIFIER_REGEX.test(identifier)) {
    throw new TypeError(
      `Invalid identifier part: ${identifier}. Must be a valid ANSI identifier.`,
    )
  }
  return `"${identifier}"`
}

/**
 * Quote a qualified identifier by splitting on dots and quoting each part
 */
function quoteQualifiedIdentifier(identifier: string): string {
  if (!QUALIFIED_IDENTIFIER_REGEX.test(identifier)) {
    throw new TypeError(
      `Invalid identifier: ${identifier}. Must be a valid identifier or qualified identifier (e.g., table.column)`,
    )
  }

  const parts = identifier.split('.')
  return parts.map(quoteSingleIdentifier).join('.')
}

/**
 * Validate and quote a SQL identifier or array of identifiers/fragments
 */
function sqlIdent(
  identifier: string | readonly (string | SqlFragment)[],
): SqlFragment {
  // Handle array of identifiers and fragments
  if (Array.isArray(identifier)) {
    if (identifier.length === 0) {
      throw new TypeError('Identifier array cannot be empty')
    }

    const fragments: SqlFragment[] = []

    for (const item of identifier) {
      // Only fragments minted by this library may pass through unquoted. This
      // prevents a forged `{ text, values }` object (e.g. from untrusted JSON)
      // from being injected as raw SQL via sql.ident().
      if (isSqlFragment(item)) {
        fragments.push(item)
      } else if (typeof item === 'string') {
        // Handle string identifiers
        if (!item) {
          throw new TypeError('All identifiers must be non-empty strings')
        }

        if (!QUALIFIED_IDENTIFIER_REGEX.test(item)) {
          throw new TypeError(
            `Invalid identifier: ${item}. Must be a valid identifier or qualified identifier (e.g., table.column)`,
          )
        }

        fragments.push(createFragment(quoteQualifiedIdentifier(item), []))
      } else {
        throw new TypeError('Array items must be strings or SQL fragments')
      }
    }

    // Join all fragments
    const text = fragments.map((f) => f.text).join(', ')
    const values = fragments.flatMap((f) => [...f.values])

    return createFragment(text, values)
  }

  // Handle single identifier (existing behavior)
  if (!identifier || typeof identifier !== 'string') {
    throw new TypeError('Identifier must be a non-empty string')
  }

  if (!QUALIFIED_IDENTIFIER_REGEX.test(identifier)) {
    throw new TypeError(
      `Invalid identifier: ${identifier}. Must be a valid identifier or qualified identifier (e.g., table.column)`,
    )
  }

  return createFragment(quoteQualifiedIdentifier(identifier), [])
}

/**
 * Create SQL IN clause from array
 */
function sqlIn(array: readonly unknown[]): SqlFragment {
  if (!Array.isArray(array)) {
    throw new TypeError('sql.in() requires an array')
  }

  if (array.length === 0) {
    throw new TypeError('sql.in() cannot be used with empty arrays')
  }

  // Soft warning for large arrays
  if (array.length > 1000) {
    console.warn(
      `sql.in(): Large array with ${array.length} items. Consider using temporary tables for better performance.`,
    )
  }

  const placeholders = array.map(() => '?').join(',')
  const values = array.map(sqlValue)

  return createFragment(`(${placeholders})`, values)
}

/**
 * Create raw SQL fragment (DANGEROUS - must not contain user input).
 *
 * This is the library's single, explicit trust boundary: whatever string is
 * passed here becomes SQL verbatim. Only ever pass developer-authored,
 * constant SQL. Never pass user input. For dynamic WHERE clauses use
 * compileFilter(); for identifiers use sql.ident().
 */
function sqlRaw(rawSql: string): SqlFragment {
  if (typeof rawSql !== 'string') {
    throw new TypeError('sql.raw() requires a string')
  }

  return createFragment(rawSql, [])
}

/**
 * Create SQL fragment for BLOB data (for validated binary data)
 */
function sqlBlob(data: Buffer | Uint8Array): SqlFragment {
  if (!(data instanceof Buffer) && !(data instanceof Uint8Array)) {
    throw new TypeError('sql.blob() requires a Buffer or Uint8Array')
  }

  return createFragment('?', [data])
}

/**
 * Tokens that allow breaking out of a SQL expression context. A join separator
 * is structural SQL (a connector such as `, `, ` AND `, ` OR `). To make
 * arbitrary separators safe regardless of their origin, we forbid the
 * primitives that would let a separator escape the connector role: string and
 * identifier literal delimiters, statement terminators, comment markers, NUL,
 * and backslash escapes.
 */
const SEPARATOR_FORBIDDEN_TOKENS = [
  "'",
  '"',
  '`',
  '[',
  ']',
  ';',
  '\\',
  '\0',
  '--',
  '/*',
  '*/',
] as const

/**
 * Validate a string separator for sql.join(). Allows any structural connector
 * while rejecting the primitives used to inject literals, comments, or extra
 * statements. Parentheses must be balanced so a separator cannot escape the
 * grouping it sits within.
 */
function assertSafeSeparator(separator: string): void {
  for (const token of SEPARATOR_FORBIDDEN_TOKENS) {
    if (separator.includes(token)) {
      throw new TypeError(
        `Unsafe sql.join() separator: contains forbidden token ${JSON.stringify(
          token,
        )}. Pass a SqlFragment (e.g. sql.raw) if you need parameterized separators.`,
      )
    }
  }

  let depth = 0
  for (const char of separator) {
    if (char === '(') {
      depth++
    } else if (char === ')') {
      depth--
      if (depth < 0) {
        throw new TypeError(
          'Unsafe sql.join() separator: unbalanced parentheses',
        )
      }
    }
  }
  if (depth !== 0) {
    throw new TypeError('Unsafe sql.join() separator: unbalanced parentheses')
  }
}

/**
 * Join SQL fragments with a separator.
 *
 * Fragments must be library-minted (branded) fragments. The separator may be:
 *  - a string: treated as a structural connector and validated by
 *    assertSafeSeparator() so any connector is supported safely; or
 *  - a SqlFragment: its text becomes the connector and its values are
 *    interleaved between fragments, allowing fully parameterized separators.
 */
function sqlJoin(
  fragments: readonly SqlFragment[],
  separator: string | SqlFragment = ', ',
): SqlFragment {
  if (!Array.isArray(fragments)) {
    throw new TypeError('sql.join() requires an array of fragments')
  }

  for (const fragment of fragments) {
    if (!isSqlFragment(fragment)) {
      throw new TypeError(
        'sql.join() requires SQL fragments created by the sql tag or its helpers',
      )
    }
  }

  if (fragments.length === 0) {
    return createFragment('', [])
  }

  let separatorText: string
  let separatorValues: readonly unknown[] = []

  if (isSqlFragment(separator)) {
    separatorText = separator.text
    separatorValues = separator.values
  } else if (typeof separator === 'string') {
    assertSafeSeparator(separator)
    separatorText = separator
  } else {
    throw new TypeError(
      'sql.join() separator must be a string or a SQL fragment',
    )
  }

  let text = ''
  const values: unknown[] = []

  fragments.forEach((fragment, index) => {
    if (index > 0) {
      text += separatorText
      values.push(...separatorValues)
    }
    text += fragment.text
    values.push(...fragment.values)
  })

  return createFragment(text, values)
}

/**
 * Scan assembled SQL once to (a) count true placeholders and (b) produce a
 * "code-only" view with string/identifier literals and comments removed.
 *
 * Placeholders inside literals/comments are not counted, and semicolons inside
 * literals/comments are not treated as statement separators.
 */
function scanSql(text: string): { placeholderCount: number; code: string } {
  let placeholderCount = 0
  let code = ''
  let i = 0
  const length = text.length

  while (i < length) {
    const char = text[i]
    const next = text[i + 1]

    // Line comment: -- ... <newline>
    if (char === '-' && next === '-') {
      i += 2
      while (i < length && text[i] !== '\n') {
        i++
      }
      continue
    }

    // Block comment: /* ... */
    if (char === '/' && next === '*') {
      i += 2
      while (i < length && !(text[i] === '*' && text[i + 1] === '/')) {
        i++
      }
      i += 2
      continue
    }

    // Quoted string ('...') or quoted identifier ("...", `...`), with the SQL
    // convention that the quote char is escaped by doubling it.
    if (char === "'" || char === '"' || char === '`') {
      const quote = char
      i++
      while (i < length) {
        if (text[i] === quote) {
          if (text[i + 1] === quote) {
            i += 2
            continue
          }
          i++
          break
        }
        i++
      }
      continue
    }

    // Bracket-quoted identifier: [ ... ]
    if (char === '[') {
      i++
      while (i < length && text[i] !== ']') {
        i++
      }
      i++
      continue
    }

    if (char === '?') {
      placeholderCount++
    }
    code += char
    i++
  }

  return { placeholderCount, code }
}

/**
 * Main SQL tagged template function
 */
function sql(strings: TemplateStringsArray, ...values: unknown[]): SqlQuery {
  // Build the query text and collect values
  let text = strings[0] || ''
  const queryValues: unknown[] = []

  for (let i = 0; i < values.length; i++) {
    const value = values[i]

    // Only library-minted fragments contribute raw text; everything else is
    // parameterized. This blocks forged `{ text, values }` objects.
    if (isSqlFragment(value)) {
      text += value.text
      queryValues.push(...value.values)
    } else {
      // Regular value - add placeholder and collect value
      text += '?'
      queryValues.push(sqlValue(value as SqlValue))
    }

    text += strings[i + 1] || ''
  }

  // Security checks
  if (text.length > MAX_QUERY_LENGTH) {
    throw new Error(
      `Query too long: ${text.length} bytes (max: ${MAX_QUERY_LENGTH})`,
    )
  }

  const { placeholderCount, code } = scanSql(text)

  // Integrity: every placeholder must have exactly one bound value and vice
  // versa. Catches raw fragments that smuggle a stray `?` (or, conversely,
  // raw SQL that forgot to carry its values), keeping text and values aligned.
  if (placeholderCount !== queryValues.length) {
    throw new Error(
      `Placeholder count (${placeholderCount}) does not match bound value count (${queryValues.length}). ` +
        'Did a raw fragment contain a "?" without supplying its value?',
    )
  }

  if (STACKED_QUERY_REGEX.test(code)) {
    throw new Error('Stacked queries are not allowed')
  }

  // Return frozen, branded result so it can be safely composed into other
  // queries (e.g. via sql.join) without being mistaken for a forgery.
  return createFragment(text, queryValues) as SqlQuery
}

// Attach helper functions to sql
sql.value = sqlValue
sql.ident = sqlIdent
sql.in = sqlIn
sql.raw = sqlRaw
sql.blob = sqlBlob
sql.join = sqlJoin

export { sql }
